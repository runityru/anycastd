package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAttemptTrackerDecide exercises the transition logic of the AttemptTracker
// value object. Each case replays a sequence of raw per-tick results (true =
// service down) and asserts the exact transition decide returns on every tick.
// The table is deliberately non-tautological: it pins down the threshold
// boundary, streak reset on sign flip, and the fact that a crossed threshold
// keeps yielding a transition (which underpins announce/denounce retry after a
// failed side effect).
func TestAttemptTrackerDecide(t *testing.T) {
	tests := []struct {
		name     string
		n, m     int
		sequence []bool
		expected []transition
	}{
		{
			name: "threshold 1/1 cold start announces immediately",
			n:    1, m: 1,
			sequence: []bool{false},
			expected: []transition{transitionAnnounce},
		},
		{
			name: "cold start down with m=1 decides denounce",
			n:    1, m: 1,
			sequence: []bool{true},
			expected: []transition{transitionDenounce},
		},
		{
			name: "cold start healthy announces immediately even with large N",
			n:    3, m: 3,
			sequence: []bool{false},
			expected: []transition{transitionAnnounce},
		},
		{
			name: "announce requires N consecutive ups after a failure",
			n:    3, m: 3,
			sequence: []bool{true, false, false, false},
			expected: []transition{transitionNone, transitionNone, transitionNone, transitionAnnounce},
		},
		{
			name: "holds state below announce threshold",
			n:    3, m: 3,
			sequence: []bool{true, false, false},
			expected: []transition{transitionNone, transitionNone, transitionNone},
		},
		{
			name: "denounce requires M consecutive downs",
			n:    1, m: 3,
			sequence: []bool{true, true, true},
			expected: []transition{transitionNone, transitionNone, transitionDenounce},
		},
		{
			name: "fail streak resets on an up tick",
			n:    1, m: 3,
			sequence: []bool{true, false, true, true, true},
			expected: []transition{transitionNone, transitionAnnounce, transitionNone, transitionNone, transitionDenounce},
		},
		{
			name: "success streak resets on a down tick",
			n:    3, m: 1,
			sequence: []bool{false, true, false, false},
			expected: []transition{transitionAnnounce, transitionDenounce, transitionNone, transitionNone},
		},
		{
			// N > M (announce_threshold > denounce_threshold) is now a legal
			// fail-closed configuration. The decide logic must stay correct for
			// it: denounce fires after fewer consecutive downs than announce
			// needs ups, so a failing service is withdrawn faster than it is
			// re-announced (fast withdrawal).
			name: "announce faster than denounce (N>M)",
			n:    3, m: 2,
			sequence: []bool{true, true, false, false, false},
			expected: []transition{transitionNone, transitionDenounce, transitionNone, transitionNone, transitionAnnounce},
		},
		{
			name: "crossed announce threshold keeps yielding announce (enables retry)",
			n:    2, m: 2,
			sequence: []bool{false, false, false, false},
			expected: []transition{transitionAnnounce, transitionAnnounce, transitionAnnounce, transitionAnnounce},
		},
		{
			name: "crossed denounce threshold keeps yielding denounce (enables retry)",
			n:    1, m: 2,
			sequence: []bool{false, true, true, true},
			expected: []transition{transitionAnnounce, transitionNone, transitionDenounce, transitionDenounce},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, len(tc.sequence), len(tc.expected), "malformed test case")
			tr := NewAttemptTracker(tc.n, tc.m)
			for i, down := range tc.sequence {
				require.Equalf(t, tc.expected[i], tr.decide(down), "tick %d (down=%v)", i, down)
			}
		})
	}
}

// toggleChecker is a hand-written checkers.Checker whose result can be flipped
// at runtime so a single behavioural test can drive a service up then down.
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

// countingAnnouncer is a hand-written announcer.Announcer fake that counts how
// many times Announce/Denounce were invoked. It is a boundary fake: the code
// under test is service.run, and the BGP side effect is out of scope.
type countingAnnouncer struct {
	mu        sync.Mutex
	announces int
	denounces int
}

func (c *countingAnnouncer) Announce(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.announces++
	return nil
}

func (c *countingAnnouncer) Denounce(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.denounces++
	return nil
}

func (c *countingAnnouncer) counts() (announces, denounces int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.announces, c.denounces
}

// countingMetrics records how many times ServiceUp/ServiceDown were observed,
// letting tests assert that metrics are updated on every tick regardless of
// whether an anti-flap transition fired.
type countingMetrics struct {
	mu      sync.Mutex
	upCount int
	downCnt int
}

func (m *countingMetrics) ServiceUp(string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upCount++
}

func (m *countingMetrics) ServiceDown(string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.downCnt++
}

func (m *countingMetrics) MeasureCall(_ context.Context, _, _ string, fn func(context.Context) error) error {
	return fn(context.Background())
}

func (m *countingMetrics) counts() (up, down int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upCount, m.downCnt
}

// TestAttemptTrackerAnnouncesOnceAtThreshold drives a service with N=3, M=2
// through a down-then-up sequence. It asserts that Announce fires exactly once
// when the 3rd consecutive up tick crosses the threshold — not on every tick —
// and that ServiceUp metrics are recorded on every up tick, even below the
// threshold.
func TestAttemptTrackerAnnouncesOnceAtThreshold(t *testing.T) {
	r := require.New(t)

	tc := &toggleChecker{kind: "icmp_ping"}
	tc.setFail(true) // start down to reset the cold-start success streak

	announcer := &countingAnnouncer{}
	metrics := &countingMetrics{}

	svc := New(
		"test_service",
		announcer,
		[]Checker{{Check: tc}},
		time.Second,
		metrics,
		AtLeastOne(),
		WithAttemptTracker(3, 2),
	).(*service)

	ctx := context.Background()

	// Two down ticks: streak 1,2. Neither announces; metrics go down each tick.
	for i := 0; i < 2; i++ {
		r.NoError(svc.run(ctx))
	}
	a, d := announcer.counts()
	r.Equal(0, a, "no announce while down below denounce threshold")
	r.Equal(0, d, "no denounce while never announced")
	up, down := metrics.counts()
	r.Equal(0, up)
	r.Equal(2, down)

	// Now healthy. The first two up ticks stay below N=3 (no announce), the
	// third crosses it and announces exactly once. Metrics go up each tick.
	tc.setFail(false)
	for i := 0; i < 2; i++ {
		r.NoError(svc.run(ctx))
	}
	a, _ = announcer.counts()
	r.Equal(0, a, "announce must not fire below the N threshold")
	up, _ = metrics.counts()
	r.Equal(2, up, "ServiceUp must update on every up tick below threshold")

	r.NoError(svc.run(ctx)) // 3rd up tick -> crosses threshold -> announce
	a, d = announcer.counts()
	r.Equal(1, a, "announce fires exactly once at the threshold")
	r.Equal(0, d)
	up, _ = metrics.counts()
	r.Equal(3, up)

	// A further up tick must not announce again (already announced).
	r.NoError(svc.run(ctx))
	a, _ = announcer.counts()
	r.Equal(1, a, "no re-announce after the transition")
	up, _ = metrics.counts()
	r.Equal(4, up)
}

// TestAttemptTrackerDenouncesOnceAtThreshold drives a service with N=3, M=2
// to the announced state, then flips it down. Consecutive failures inside the
// M window do not flap (no denounce); the M-th failure denounces exactly once;
// further failures do not re-denounce.
func TestAttemptTrackerDenouncesOnceAtThreshold(t *testing.T) {
	r := require.New(t)

	tc := &toggleChecker{kind: "icmp_ping"}
	tc.setFail(true) // start down to reset the cold-start success streak

	announcer := &countingAnnouncer{}
	metrics := &countingMetrics{}

	svc := New(
		"test_service",
		announcer,
		[]Checker{{Check: tc}},
		time.Second,
		metrics,
		AtLeastOne(),
		WithAttemptTracker(3, 2),
	).(*service)

	ctx := context.Background()

	// Establish the announced state: 2 down (reset streak) then 3 up (announce).
	r.NoError(svc.run(ctx))
	r.NoError(svc.run(ctx))
	tc.setFail(false)
	r.NoError(svc.run(ctx))
	r.NoError(svc.run(ctx))
	r.NoError(svc.run(ctx))
	a, _ := announcer.counts()
	r.Equal(1, a, "precondition: service is announced")

	// Flip down. First failure is below the M=2 window -> no denounce (no flap).
	tc.setFail(true)
	r.NoError(svc.run(ctx))
	a, d := announcer.counts()
	r.Equal(1, a)
	r.Equal(0, d, "first down tick inside window must not denounce")
	up, down := metrics.counts()
	r.Equal(3, up, "precondition up ticks remain counted")
	r.Equal(3, down, "down metric updates on every down tick")

	// Second consecutive failure crosses M=2 -> denounce exactly once.
	r.NoError(svc.run(ctx))
	a, d = announcer.counts()
	r.Equal(1, a)
	r.Equal(1, d, "denounce fires exactly once at the M threshold")
	_, down = metrics.counts()
	r.Equal(4, down)

	// Third consecutive failure is past the threshold but already denounced ->
	// no re-denounce.
	r.NoError(svc.run(ctx))
	a, d = announcer.counts()
	r.Equal(1, a)
	r.Equal(1, d, "no re-denounce after the transition")
	_, down = metrics.counts()
	r.Equal(5, down, "ServiceDown keeps updating every down tick")
}

// TestAttemptTrackerColdStartHealthyAnnouncesImmediately verifies the cold
// start rule: a service that is healthy on its very first tick announces at
// once, even with a non-trivial announce threshold.
func TestAttemptTrackerColdStartHealthyAnnouncesImmediately(t *testing.T) {
	r := require.New(t)

	tc := &toggleChecker{kind: "icmp_ping"} // healthy by default

	announcer := &countingAnnouncer{}
	metrics := &countingMetrics{}

	svc := New(
		"test_service",
		announcer,
		[]Checker{{Check: tc}},
		time.Second,
		metrics,
		AtLeastOne(),
		WithAttemptTracker(5, 5),
	).(*service)

	r.NoError(svc.run(context.Background()))
	a, _ := announcer.counts()
	r.Equal(1, a, "healthy cold-start service announces on its first tick")
	up, _ := metrics.counts()
	r.Equal(1, up)
}

// TestAttemptTrackerDefaultIsPassThrough verifies that New without the
// WithAttemptTracker option keeps the legacy 1/1 behaviour: every healthy tick
// announces and every down tick denounces immediately.
func TestAttemptTrackerDefaultIsPassThrough(t *testing.T) {
	r := require.New(t)

	tc := &toggleChecker{kind: "icmp_ping"}
	announcer := &countingAnnouncer{}
	metrics := &countingMetrics{}

	svc := New(
		"test_service",
		announcer,
		[]Checker{{Check: tc}},
		time.Second,
		metrics,
		AtLeastOne(),
	).(*service)

	ctx := context.Background()
	r.NoError(svc.run(ctx)) // healthy -> announce
	tc.setFail(true)
	r.NoError(svc.run(ctx)) // down -> denounce

	a, d := announcer.counts()
	r.Equal(1, a)
	r.Equal(1, d)
}
