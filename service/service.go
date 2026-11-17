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
	name       string
	announcer  announcer.Announcer
	checks     []Checker
	interval   time.Duration
	metrics    Metrics
	strategy   Strategy
	hysteresis Hysteresis

	announced *atomic.Bool
}

// Option configures a Service built by New. Options are applied after the
// defaults, so a later option wins.
type Option func(*service)

// WithHysteresis returns an Option that gates Announce/Denounce transitions on
// N consecutive successes and M consecutive failures. N must be <= M; a zero
// N/M is not valid here — callers are expected to pass the validated,
// defaulted values from configuration. Without this option New applies the
// default 1/1 (announce/denounce immediately, legacy behaviour).
func WithHysteresis(n, m int) Option {
	return func(s *service) {
		s.hysteresis = NewHysteresis(n, m)
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
		name:       name,
		announcer:  a,
		checks:     checks,
		interval:   interval,
		metrics:    metrics,
		strategy:   strategy,
		hysteresis: NewHysteresis(1, 1),
		announced:  &atomic.Bool{},
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

	// Hysteresis decides whether to announce/denounce this tick. The
	// announced flag guards the side effect so it fires exactly once per
	// transition; the decide call itself stays idempotent per tick.
	switch s.hysteresis.decide(serviceDown) {
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

	return nil
}
