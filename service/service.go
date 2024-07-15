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

type service struct {
	name      string
	announcer announcer.Announcer
	checks    []checkers.Checker
	interval  time.Duration
	metrics   Metrics

	announced *atomic.Bool
}

func New(
	name string,
	a announcer.Announcer,
	checks []checkers.Checker,
	interval time.Duration,
	metrics Metrics,
) Service {
	return &service{
		name:      name,
		announcer: a,
		checks:    checks,
		interval:  interval,
		metrics:   metrics,
		announced: &atomic.Bool{},
	}
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
	for _, check := range s.checks {
		if err := s.metrics.MeasureCall(ctx, s.name, check.Kind(), check.Check); err != nil {
			log.Warnf("check failed: %s", err)

			s.metrics.ServiceDown(s.name)

			if s.announced.Load() {
				if err := s.announcer.Denounce(ctx); err != nil {
					log.Warnf("denounce failed: %s", err)
					return nil
				}
				s.announced.Store(false)
			}
			return nil
		}
	}

	s.metrics.ServiceUp(s.name)

	if !s.announced.Load() {
		if err := s.announcer.Announce(ctx); err != nil {
			log.Warnf("announce failed: %s", err)
			return nil
		}
	}

	s.announced.Store(true)

	return nil
}
