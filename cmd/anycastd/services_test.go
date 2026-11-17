package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	th "github.com/teran/go-time"

	"github.com/runityru/anycastd/announcer"
	"github.com/runityru/anycastd/checkers"
	"github.com/runityru/anycastd/config"
)

// stubMetrics is a no-op service.Metrics used to build services in tests
// without touching the global Prometheus registry. It is a test-only fake at
// the metrics boundary.
type stubMetrics struct{}

func (stubMetrics) ServiceUp(string)   {}
func (stubMetrics) ServiceDown(string) {}
func (stubMetrics) MeasureCall(context.Context, string, string, func(context.Context) error) error {
	return nil
}

func TestBuildServices(t *testing.T) {
	r := require.New(t)

	driver := announcer.NewDriverMock()

	// Record every checker kind the factory is asked to construct so we can
	// assert the correct number of checkers was wired per service.
	var gotKinds []string
	newChecker := func(kind string, _ json.RawMessage) (checkers.Checker, error) {
		gotKinds = append(gotKinds, kind)
		return &checkers.Mock{}, nil
	}

	svcs := []config.Service{
		{
			Name:          "svc-a",
			CheckInterval: th.Duration(10 * time.Millisecond),
			Strategy:      "at_least_one",
			Checks: []config.Check{
				{Kind: "icmp_ping", Group: "g1"},
				{Kind: "dns_lookup", Group: "g1"},
			},
		},
		{
			Name:          "svc-b",
			CheckInterval: th.Duration(20 * time.Millisecond),
			Strategy:      "all",
			Checks: []config.Check{
				{Kind: "http_2xx", Group: "g2"},
			},
		},
	}

	services, err := buildServices(svcs, []string{"10.0.0.0/24", "2001:db8::/32"}, driver, stubMetrics{}, newChecker)
	r.NoError(err)
	r.Len(services, 2)

	// One checker per configured check, in declaration order.
	r.Equal([]string{"icmp_ping", "dns_lookup", "http_2xx"}, gotKinds)
}

func TestBuildServicesRunsAnnouncerWithPrefixes(t *testing.T) {
	r := require.New(t)

	driver := announcer.NewDriverMock()

	// Record every announced prefix behind a mutex: the service goroutine
	// appends to it, the test goroutine reads it. This avoids a data race on
	// testify's internal Calls slice while polling.
	var mu sync.Mutex
	announced := map[string]bool{}
	driver.On("AddPath", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			mu.Lock()
			defer mu.Unlock()
			announced[args.Get(1).(string)] = true
		}).Return(nil)

	// A passing checker lets the service reach the "up" state, which drives the
	// announcer to announce every configured prefix over the injected driver.
	// service.run reads Check.Kind() on every tick, so the mock checker needs
	// a Kind expectation even though MeasureCall is a no-op stub.
	newChecker := func(_ string, _ json.RawMessage) (checkers.Checker, error) {
		c := &checkers.Mock{}
		c.On("Kind").Return("icmp_ping")
		return c, nil
	}

	svcs := []config.Service{
		{
			Name:          "svc-a",
			CheckInterval: th.Duration(5 * time.Millisecond),
			Strategy:      "at_least_one",
			Checks:        []config.Check{{Kind: "icmp_ping"}},
		},
	}

	services, err := buildServices(svcs, []string{"10.0.0.0/24", "2001:db8::/32"}, driver, stubMetrics{}, newChecker)
	r.NoError(err)
	r.Len(services, 1)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = services[0].Run(ctx)
	}()

	// The loop ticks on CheckInterval; wait until both prefixes are announced.
	r.Eventually(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return announced["10.0.0.0/24"] && announced["2001:db8::/32"]
	}, time.Second, 10*time.Millisecond)

	// Stop the loop and wait for the goroutine to exit before asserting
	// expectations, so no concurrent append can race with the assertion.
	cancel()
	<-done

	driver.AssertExpectations(t)
}

func TestBuildServicesInvalidCheckerKind(t *testing.T) {
	r := require.New(t)

	// Use the real registry factory: an unregistered kind must fail closed.
	svcs := []config.Service{
		{
			Name:          "svc-a",
			CheckInterval: th.Duration(10 * time.Millisecond),
			Strategy:      "at_least_one",
			Checks:        []config.Check{{Kind: "does_not_exist"}},
		},
	}

	_, err := buildServices(svcs, nil, announcer.NewDriverMock(), stubMetrics{}, checkers.NewCheckerByKind)
	r.Error(err)
	r.Contains(err.Error(), "does_not_exist")
	r.Contains(err.Error(), "svc-a")
}

func TestBuildServicesInvalidStrategy(t *testing.T) {
	r := require.New(t)

	newChecker := func(_ string, _ json.RawMessage) (checkers.Checker, error) {
		return &checkers.Mock{}, nil
	}

	svcs := []config.Service{
		{
			Name:          "svc-a",
			CheckInterval: th.Duration(10 * time.Millisecond),
			Strategy:      "no_such_strategy",
			Checks:        []config.Check{{Kind: "icmp_ping"}},
		},
	}

	_, err := buildServices(svcs, nil, announcer.NewDriverMock(), stubMetrics{}, newChecker)
	r.Error(err)
	r.Contains(err.Error(), "no_such_strategy")
}
