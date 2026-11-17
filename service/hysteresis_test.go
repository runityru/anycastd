package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHysteresisDecide exercises the transition logic of the Hysteresis value
// object. Each case replays a sequence of raw per-tick results (true = service
// down) and asserts the exact transition decide returns on every tick. The
// table is deliberately non-tautological: it pins down the threshold boundary,
// streak reset on sign flip, and the fact that a crossed threshold keeps
// yielding a transition (which underpins announce/denounce retry after a
// failed side effect).
func TestHysteresisDecide(t *testing.T) {
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
			h := NewHysteresis(tc.n, tc.m)
			for i, down := range tc.sequence {
				require.Equalf(t, tc.expected[i], h.decide(down), "tick %d (down=%v)", i, down)
			}
		})
	}
}
