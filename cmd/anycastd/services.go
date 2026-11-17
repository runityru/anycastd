package main

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/runityru/anycastd/announcer"
	"github.com/runityru/anycastd/checkers"
	"github.com/runityru/anycastd/config"
	"github.com/runityru/anycastd/service"
)

// newCheckerFactory constructs a checkers.Checker from a registered kind and
// its raw spec. It is a function type so buildServices can be exercised with a
// stub/real factory without spinning up a real BGP driver.
type newCheckerFactory func(kind string, spec json.RawMessage) (checkers.Checker, error)

// buildServices turns the configured services into ready-to-run
// service.Service instances. It is a pure builder: it performs no I/O, starts
// nothing and registers no goroutines, which makes it unit-testable without a
// live GoBGP server. All side-effecting dependencies (the BGP driver and the
// service metrics sink) are injected; the checker factory is injectable so the
// happy path can use a stub and the error path can use the real registry.
func buildServices(
	svcs []config.Service,
	routes []string,
	driver announcer.BGPServer,
	metrics service.Metrics,
	newChecker newCheckerFactory,
) ([]service.Service, []announcer.Announcer, error) {
	services := make([]service.Service, 0, len(svcs))
	announcers := make([]announcer.Announcer, 0, len(svcs))

	for _, svcCfg := range svcs {
		checks := make([]service.Checker, 0, len(svcCfg.Checks))
		for _, check := range svcCfg.Checks {
			c, err := newChecker(check.Kind, check.Spec)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "error creating checker %q for service %q", check.Kind, svcCfg.Name)
			}
			checks = append(checks, service.Checker{Check: c, Group: check.Group})
		}

		a := announcer.New(announcer.Config{
			GoBGP:    driver,
			Prefixes: routes,
		})
		announcers = append(announcers, a)

		strategy, err := service.GetStrategy(svcCfg.Strategy, svcCfg.StrategyOptions)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error getting strategy for service %q", svcCfg.Name)
		}

		// Hysteresis thresholds default to 1/1 (immediate) when unset (0).
		announceThreshold := svcCfg.AnnounceThreshold
		if announceThreshold == 0 {
			announceThreshold = 1
		}
		denounceThreshold := svcCfg.DenounceThreshold
		if denounceThreshold == 0 {
			denounceThreshold = 1
		}

		svc := service.New(
			svcCfg.Name,
			a,
			checks,
			svcCfg.CheckInterval.TimeDuration(),
			metrics,
			strategy,
			service.WithHysteresis(announceThreshold, denounceThreshold),
		)

		services = append(services, svc)
	}

	return services, announcers, nil
}
