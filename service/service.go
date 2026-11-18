package service

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/runityru/anycastd/announcer"
	"github.com/runityru/anycastd/checkers"
)

type Service interface {
	Run(ctx context.Context) error
}

type Checker struct {
	Check checkers.Checker
	Group string
}

type service struct {
	name           string
	announcer      announcer.Announcer
	checks         []Checker
	interval       time.Duration
	metrics        Metrics
	strategy       Strategy
	attemptTracker AttemptTracker
	peers          announcer.PeerStateSource

	announced *atomic.Bool
}

// Option configures a Service built by New. Options are applied after the
// defaults, so a later option wins.
type Option func(*service)

// WithAttemptTracker returns an Option that gates Announce/Denounce transitions on
// N consecutive successes and M consecutive failures. N and M are independent
// positive values (validated by config.Service.Validate); a zero N/M behaves
// like 1/1 (immediate). Callers are expected to pass the validated, defaulted
// values from configuration. Without this option New applies the default 1/1
// (announce/denounce immediately, legacy behaviour).
func WithAttemptTracker(n, m int) Option {
	return func(s *service) {
		s.attemptTracker = NewAttemptTracker(n, m)
	}
}

// WithPeerStateSource returns an Option that makes the reported announce state
// reflect peer sessions: a service is only reported announced when it is locally
// confirmed announced AND at least one BGP peer session is ESTABLISHED. A nil
// source (the default) is treated as "all peers established", so without this
// option the gauge reflects the locally-confirmed announce state (legacy
// behaviour). A transient ListPeers error is tolerated: the service falls back
// to its locally-confirmed state rather than flapping.
func WithPeerStateSource(src announcer.PeerStateSource) Option {
	return func(s *service) {
		s.peers = src
	}
}

func New(
	name string,
	a announcer.Announcer,
	checks []Checker,
	interval time.Duration,
	metrics Metrics,
	strategy Strategy,
	opts ...Option,
) Service {
	s := &service{
		name:           name,
		announcer:      a,
		checks:         checks,
		interval:       interval,
		metrics:        metrics,
		strategy:       strategy,
		attemptTracker: NewAttemptTracker(1, 1),
		announced:      &atomic.Bool{},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *service) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.interval):
			if err := s.run(ctx); err != nil {
				return errors.Wrap(err, "service error")
			}
		}
	}
}

func (s *service) run(ctx context.Context) error {
	checkResults := []CheckResult{}
	for _, check := range s.checks {
		result := true
		if err := s.metrics.MeasureCall(ctx, s.name, check.Check.Kind(), check.Check.Check); err != nil {
			log.Warnf("check failed: %s", err)
			result = false
		}
		checkResults = append(checkResults, CheckResult{check, result})
	}

	serviceDown, err := s.strategy(checkResults)
	if err != nil {
		return err
	}

	if serviceDown {
		s.metrics.ServiceDown(s.name)
	} else {
		s.metrics.ServiceUp(s.name)
	}

	// AttemptTracker decides whether to announce/denounce this tick. The
	// announced flag guards the side effect so it fires exactly once per
	// transition; the decide call itself stays idempotent per tick.
	switch s.attemptTracker.decide(serviceDown) {
	case transitionAnnounce:
		if !s.announced.Load() {
			if err := s.announcer.Announce(ctx); err != nil {
				log.Warnf("announce failed: %s", err)
			} else {
				s.announced.Store(true)
			}
		}
	case transitionDenounce:
		if s.announced.Load() {
			if err := s.announcer.Denounce(ctx); err != nil {
				log.Warnf("denounce failed: %s", err)
			} else {
				s.announced.Store(false)
			}
		}
	}

	// Record the effective announced state every tick: the prefix is only truly
	// on the wire when we have locally confirmed the announce AND at least one
	// BGP peer session is ESTABLISHED. The gauge therefore drops to 0 if the
	// prefix is not actually being advertised because no peer is up, even when
	// s.announced is still true. The internal s.announced flag is left untouched
	// (it only gates the announce/denounce side effects); the `&&` short-circuits
	// the peer check when not announced.
	effective := s.announced.Load() && s.anyPeerEstablished(ctx)
	s.metrics.ServiceAnnounced(s.name, effective)

	return nil
}

// anyPeerEstablished reports whether at least one configured BGP peer session is
// ESTABLISHED. With no source injected it reports true (legacy behaviour: the
// effective announced state equals the locally-confirmed one). A ListPeers error
// is logged and treated as "established" so a transient failure never flaps the
// gauge to 0; the caller still trusts the locally-confirmed announce state.
func (s *service) anyPeerEstablished(ctx context.Context) bool {
	if s.peers == nil {
		return true
	}

	states, err := s.peers.ListPeers(ctx)
	if err != nil {
		log.Warnf("failed to list BGP peers: %s", err)
		return true
	}

	for _, p := range states {
		if p.SessionState == announcer.PeerSessionEstablished {
			return true
		}
	}

	return false
}
