package service

// transition is the announce/denounce decision produced by AttemptTracker.decide
// for a single check tick. AttemptTracker gates only the Announce/Denounce side
// effects; the raw ServiceUp/ServiceDown metrics are recorded every tick
// regardless of any threshold.
type transition int

const (
	transitionNone transition = iota
	transitionAnnounce
	transitionDenounce
)

// AttemptTracker is a value object that gates the BGP transitions:
//
//   - Announce only after N consecutive successful (non-down) ticks;
//   - Denounce only after M consecutive failed (down) ticks.
//
// A streak resets to zero when the sign flips. On cold start a healthy
// (non-down) service announces immediately: the success streak is seeded to N
// so the first "up" tick already crosses the announce threshold. N and M are
// independent positive values (validated by config.Service.Validate); allowing
// N > M yields a fast-withdraw (fail-closed) configuration where the service
// is withdrawn quicker than it is announced.
type AttemptTracker struct {
	N int
	M int

	successStreak int
	failStreak    int
}

// NewAttemptTracker returns an AttemptTracker gating announce on N consecutive ups and
// denounce on M consecutive downs. The success streak is seeded to N so a
// service that is healthy at startup announces on its first tick (cold start).
func NewAttemptTracker(n, m int) AttemptTracker {
	return AttemptTracker{
		N:             n,
		M:             m,
		successStreak: n,
	}
}

// decide consumes the raw serviceDown result of one tick and returns the
// transition the caller must perform this tick. It mutates the streak counters
// as a side effect, so it must be called exactly once per tick.
func (t *AttemptTracker) decide(serviceDown bool) transition {
	if serviceDown {
		t.successStreak = 0
		t.failStreak++
		if t.failStreak >= t.M {
			return transitionDenounce
		}
		return transitionNone
	}

	t.failStreak = 0
	t.successStreak++
	if t.successStreak >= t.N {
		return transitionAnnounce
	}
	return transitionNone
}
