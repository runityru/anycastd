package service

import (
	"context"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/teran/anycastd/announcer"
	"github.com/teran/anycastd/checkers"
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
		if err := check.Check(ctx); err != nil {
			log.Warnf("check failed: %s", err)

			s.metrics.ServiceDown(s.name)

			if err := s.announcer.Denounce(ctx); err != nil {
				log.Warnf("denounce failed: %s", err)
				return nil
			}
			return nil
		}
	}

	s.metrics.ServiceUp(s.name)

	if err := s.announcer.Announce(ctx); err != nil {
		log.Warnf("announce failed: %s", err)
	}

	return nil
}
