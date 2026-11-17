package main

import (
	"context"
	"encoding/json"
	"errors"
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

// spyMetrics is a service.Metrics fake that actually runs the checker fn and
// records the last known up/down state. Unlike stubMetrics (which never calls
// the fn), spyMetrics lets behavioural strategy tests model failing checks.
type spyMetrics struct {
	mu  sync.Mutex
	up  bool
	set bool
}

func (s *spyMetrics) ServiceUp(string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.up = true
	s.set = true
}

func (s *spyMetrics) ServiceDown(string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.up = false
	s.set = true
}

func (s *spyMetrics) MeasureCall(ctx context.Context, _, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}

func (s *spyMetrics) isUp() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.up
}

// toggleChecker is a hand-written checkers.Checker whose result can be flipped
// at runtime, so a single behavioural test can drive a service up then down.
type toggleChecker struct {
	mu   sync.Mutex
	fail bool
	kind string
}

func (c *toggleChecker) Kind() string { return c.kind }

func (c *toggleChecker) Check(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return errors.New("check failed")
	}
	return nil
}

func (c *toggleChecker) setFail(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fail = v
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

	services, _, err := buildServices(svcs, []string{"10.0.0.0/24", "2001:db8::/32"}, driver, stubMetrics{}, newChecker)
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

	services, _, err := buildServices(svcs, []string{"10.0.0.0/24", "2001:db8::/32"}, driver, stubMetrics{}, newChecker)
	r.NoError(err)
	r.Len(services, 1)

	ctx, cancel := context.WithCancel(t.Context())

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

	_, _, err := buildServices(svcs, nil, announcer.NewDriverMock(), stubMetrics{}, checkers.NewCheckerByKind)
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

	_, _, err := buildServices(svcs, nil, announcer.NewDriverMock(), stubMetrics{}, newChecker)
	r.Error(err)
	r.Contains(err.Error(), "no_such_strategy")
	r.Contains(err.Error(), "svc-a")
}

// TestBuildServicesEmptyServicesList asserts that an empty (or nil) services
// list yields an empty, non-nil slice and no error.
func TestBuildServicesEmptyServicesList(t *testing.T) {
	r := require.New(t)

	services, _, err := buildServices(nil, nil, announcer.NewDriverMock(), stubMetrics{}, func(_ string, _ json.RawMessage) (checkers.Checker, error) {
		return &checkers.Mock{}, nil
	})

	r.NoError(err)
	r.NotNil(services)
	r.Len(services, 0)
}

// TestBuildServicesPropagatesCheckGroups asserts that the Group of each
// configured check is carried through to the built service. It uses the
// all_in_group strategy over two checks in different groups: g1 fails, g2
// passes. Because g1 is fully failed, the service must be Down. If buildServices
// dropped the Group (both checks landing in the empty "" group), the strategy
// would see a single partially-failed group and report Up — the test fails.
func TestBuildServicesPropagatesCheckGroups(t *testing.T) {
	r := require.New(t)

	driver := announcer.NewDriverMock()
	sm := &spyMetrics{}

	newChecker := func(kind string, _ json.RawMessage) (checkers.Checker, error) {
		c := &checkers.Mock{}
		c.On("Kind").Return(kind)
		if kind == "failing" {
			c.On("Check").Return(errors.New("boom"))
		} else {
			c.On("Check").Return(nil)
		}
		return c, nil
	}

	svcs := []config.Service{
		{
			Name:          "svc-a",
			CheckInterval: th.Duration(5 * time.Millisecond),
			Strategy:      "all_in_group",
			Checks: []config.Check{
				{Kind: "failing", Group: "g1"},
				{Kind: "passing", Group: "g2"},
			},
		},
	}

	services, _, err := buildServices(svcs, []string{"10.0.0.0/24"}, driver, sm, newChecker)
	r.NoError(err)
	r.Len(services, 1)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = services[0].Run(ctx)
	}()

	r.Eventually(func() bool { return sm.isUp() == false }, time.Second, 10*time.Millisecond)

	cancel()
	<-done
}

// TestBuildServicesStrategyAllRequiresEveryCheckUp asserts that the "all"
// strategy announces only when every check passes. With one of two checks
// failing, a correct "all" reports Up (AddPath). A mutation that always used
// at_least_one would instead report Down here, and the test would fail because
// no prefix is ever announced.
func TestBuildServicesStrategyAllRequiresEveryCheckUp(t *testing.T) {
	r := require.New(t)

	driver := announcer.NewDriverMock()

	var mu sync.Mutex
	announced := map[string]bool{}
	driver.On("AddPath", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			mu.Lock()
			defer mu.Unlock()
			announced[args.Get(1).(string)] = true
		}).Return(nil)

	newChecker := func(kind string, _ json.RawMessage) (checkers.Checker, error) {
		c := &checkers.Mock{}
		c.On("Kind").Return(kind)
		if kind == "failing" {
			c.On("Check").Return(errors.New("boom"))
		} else {
			c.On("Check").Return(nil)
		}
		return c, nil
	}

	svcs := []config.Service{
		{
			Name:          "svc-a",
			CheckInterval: th.Duration(5 * time.Millisecond),
			Strategy:      "all",
			Checks: []config.Check{
				{Kind: "failing"},
				{Kind: "passing"},
			},
		},
	}

	services, _, err := buildServices(svcs, []string{"10.0.0.0/24"}, driver, stubMetrics{}, newChecker)
	r.NoError(err)
	r.Len(services, 1)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = services[0].Run(ctx)
	}()

	r.Eventually(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return announced["10.0.0.0/24"]
	}, time.Second, 10*time.Millisecond)

	cancel()
	<-done
}

// TestBuildServicesStrategyAllDenouncesOnDown asserts the full up→down cycle
// for the "all" strategy: a passing check announces, then flipping the check
// to failing withdraws (Denounce → DeletePath).
func TestBuildServicesStrategyAllDenouncesOnDown(t *testing.T) {
	r := require.New(t)

	driver := announcer.NewDriverMock()

	var mu sync.Mutex
	announced := map[string]bool{}
	driver.On("AddPath", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			mu.Lock()
			defer mu.Unlock()
			announced[args.Get(1).(string)] = true
		}).Return(nil)
	driver.On("DeletePath", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			mu.Lock()
			defer mu.Unlock()
			delete(announced, args.Get(1).(string))
		}).Return(nil)

	tc := &toggleChecker{kind: "icmp_ping"}
	newChecker := func(_ string, _ json.RawMessage) (checkers.Checker, error) {
		return tc, nil
	}

	svcs := []config.Service{
		{
			Name:          "svc-a",
			CheckInterval: th.Duration(5 * time.Millisecond),
			Strategy:      "all",
			Checks:        []config.Check{{Kind: "icmp_ping"}},
		},
	}

	services, _, err := buildServices(svcs, []string{"10.0.0.0/24"}, driver, &spyMetrics{}, newChecker)
	r.NoError(err)
	r.Len(services, 1)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = services[0].Run(ctx)
	}()

	// Healthy → announced.
	r.Eventually(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return announced["10.0.0.0/24"]
	}, time.Second, 10*time.Millisecond)

	// Flip to failing → service Down → withdrawn.
	tc.setFail(true)
	r.Eventually(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return !announced["10.0.0.0/24"]
	}, time.Second, 10*time.Millisecond)

	cancel()
	<-done
}

// TestBuildServicesReturnsWithdrawableAnnouncers asserts that buildServices
// returns one announcer per service, and that each announcer withdraws every
// configured prefix via DeletePath when Denounced. This underpins graceful
// shutdown, which withdraws prefixes before closing BGP sessions.
func TestBuildServicesReturnsWithdrawableAnnouncers(t *testing.T) {
	r := require.New(t)

	driver := announcer.NewDriverMock()

	var mu sync.Mutex
	deleted := map[string]bool{}
	driver.On("DeletePath", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			mu.Lock()
			defer mu.Unlock()
			deleted[args.Get(1).(string)] = true
		}).Return(nil)

	newChecker := func(_ string, _ json.RawMessage) (checkers.Checker, error) {
		return &checkers.Mock{}, nil
	}

	svcs := []config.Service{
		{
			Name:          "svc-a",
			CheckInterval: th.Duration(10 * time.Millisecond),
			Strategy:      "at_least_one",
			Checks:        []config.Check{{Kind: "icmp_ping"}},
		},
		{
			Name:          "svc-b",
			CheckInterval: th.Duration(10 * time.Millisecond),
			Strategy:      "at_least_one",
			Checks:        []config.Check{{Kind: "http_2xx"}},
		},
	}
	routes := []string{"10.0.0.0/24", "2001:db8::/32"}

	_, announcers, err := buildServices(svcs, routes, driver, stubMetrics{}, newChecker)
	r.NoError(err)
	r.Len(announcers, 2)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	for _, a := range announcers {
		r.NoError(a.Denounce(ctx))
	}

	mu.Lock()
	defer mu.Unlock()
	r.Len(deleted, 2)
	r.True(deleted["10.0.0.0/24"])
	r.True(deleted["2001:db8::/32"])
}
