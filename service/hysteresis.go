package service

// transition is the announce/denounce decision produced by Hysteresis.decide
// for a single check tick. Hysteresis gates only the Announce/Denounce side
// effects; the raw ServiceUp/ServiceDown metrics are recorded every tick
// regardless of any threshold.
type transition int

const (
	transitionNone transition = iota
	transitionAnnounce
	transitionDenounce
)

// Hysteresis is a value object that gates the BGP transitions:
//
//   - Announce only after N consecutive successful (non-down) ticks;
//   - Denounce only after M consecutive failed (down) ticks.
//
// A streak resets to zero when the sign flips. On cold start a healthy
// (non-down) service announces immediately: the success streak is seeded to N
// so the first "up" tick already crosses the announce threshold. The invariant
// 0 < N <= M is enforced by configuration validation (config.Service.Validate).
type Hysteresis struct {
	N int
	M int

	successStreak int
	failStreak    int
}

// NewHysteresis returns a Hysteresis gating announce on N consecutive ups and
// denounce on M consecutive downs. The success streak is seeded to N so a
// service that is healthy at startup announces on its first tick (cold start).
func NewHysteresis(n, m int) Hysteresis {
	return Hysteresis{
		N:             n,
		M:             m,
		successStreak: n,
	}
}

// decide consumes the raw serviceDown result of one tick and returns the
// transition the caller must perform this tick. It mutates the streak counters
// as a side effect, so it must be called exactly once per tick.
func (h *Hysteresis) decide(serviceDown bool) transition {
	if serviceDown {
		h.successStreak = 0
		h.failStreak++
		if h.failStreak >= h.M {
			return transitionDenounce
		}
		return transitionNone
	}

	h.failStreak = 0
	h.successStreak++
	if h.successStreak >= h.N {
		return transitionAnnounce
	}
	return transitionNone
}
