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
	announcer announcer.Announcer
	checks    []checkers.Checker
	interval  time.Duration
}

func New(
	a announcer.Announcer,
	checks []checkers.Checker,
	interval time.Duration,
) Service {
	return &service{
		announcer: a,
		checks:    checks,
		interval:  interval,
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
	successCount := 0
	for _, check := range s.checks {
		if err := check.Check(ctx); err != nil {
			log.Warnf("check failed: %s", err)
		}
		successCount++
	}

	if successCount == len(s.checks) {
		if err := s.announcer.Announce(ctx); err != nil {
			log.Warnf("announce error: %s", err)
		}
	} else {
		if err := s.announcer.Denounce(ctx); err != nil {
			log.Warnf("denounce error: %s", err)
		}
	}

	return nil
}
