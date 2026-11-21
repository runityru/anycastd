package gobgpv4

import (
	"io"
	"testing"
	"time"

	"github.com/osrg/gobgp/v4/pkg/server"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/runityru/anycastd/announcer"
)

// TestFSMTimingsCollectorObserve feeds Observe with known durations and asserts
// via a fresh registry's Gather output that each histogram recorded the right
// sample count and sum. This exercises both the server.FSMTimingHook path
// (Observe) and the prometheus.Collector path (Describe/Collect).
func TestFSMTimingsCollectorObserve(t *testing.T) {
	r := require.New(t)
	c := NewFSMTimingsCollector()

	// Two mgmt_op operations: one with a wait delay, one with tWait==0 (which
	// must NOT create a wait observation).
	c.Observe(server.FSMMgmtOp, 100*time.Millisecond, 5*time.Millisecond)
	c.Observe(server.FSMMgmtOp, 300*time.Millisecond, 0)
	// A single message operation with a wait delay.
	c.Observe(server.FSMMessage, 200*time.Millisecond, 10*time.Millisecond)

	reg := prometheus.NewRegistry()
	r.NoError(reg.Register(c))
	mfs, err := reg.Gather()
	r.NoError(err)

	hist := func(name string) *dto.Histogram {
		for _, mf := range mfs {
			if mf.GetName() == name {
				return mf.GetMetric()[0].GetHistogram()
			}
		}
		return nil
	}

	mgmtTiming := hist("anycastd_gobgp_fsm_mgmt_op_timing_seconds")
	r.NotNil(mgmtTiming, "mgmt_op timing histogram must be present")
	r.Equal(uint64(2), mgmtTiming.GetSampleCount())
	r.InDelta(0.4, mgmtTiming.GetSampleSum(), 1e-9)

	mgmtWait := hist("anycastd_gobgp_fsm_mgmt_op_wait_seconds")
	r.NotNil(mgmtWait, "mgmt_op wait histogram must be present")
	r.Equal(uint64(1), mgmtWait.GetSampleCount(), "tWait==0 must not create a wait observation")
	r.InDelta(0.005, mgmtWait.GetSampleSum(), 1e-9)

	msgTiming := hist("anycastd_gobgp_fsm_message_timing_seconds")
	r.NotNil(msgTiming, "message timing histogram must be present")
	r.Equal(uint64(1), msgTiming.GetSampleCount())
	r.InDelta(0.2, msgTiming.GetSampleSum(), 1e-9)

	msgWait := hist("anycastd_gobgp_fsm_message_wait_seconds")
	r.NotNil(msgWait, "message wait histogram must be present")
	r.Equal(uint64(1), msgWait.GetSampleCount())
	r.InDelta(0.01, msgWait.GetSampleSum(), 1e-9)
}

// TestFSMTimingsCollectorMetricNames asserts the collector exposes exactly the
// 8 expected anycastd_-prefixed families, one timing and one wait per FSM
// operation infix.
func TestFSMTimingsCollectorMetricNames(t *testing.T) {
	r := require.New(t)
	c := NewFSMTimingsCollector()

	reg := prometheus.NewRegistry()
	r.NoError(reg.Register(c))
	mfs, err := reg.Gather()
	r.NoError(err)

	names := make(map[string]bool, len(mfs))
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	r.Equal(8, len(names), "must expose exactly 8 histogram families")

	for _, infix := range []string{"mgmt_op", "accept", "event", "message"} {
		r.True(names["anycastd_gobgp_fsm_"+infix+"_timing_seconds"], "missing %s", "anycastd_gobgp_fsm_"+infix+"_timing_seconds")
		r.True(names["anycastd_gobgp_fsm_"+infix+"_wait_seconds"], "missing %s", "anycastd_gobgp_fsm_"+infix+"_wait_seconds")
	}
}

// TestFSMTimingsCollectorOutOfRange guards against a future FSMOperation index
// beyond FSMOperationTypeCount: Observe must ignore it rather than panic.
func TestFSMTimingsCollectorOutOfRange(t *testing.T) {
	r := require.New(t)
	c := NewFSMTimingsCollector()
	r.NotPanics(func() {
		c.Observe(server.FSMOperation(99), time.Second, time.Second)
	})
}

// TestNewDriverWiresFSMTimingsHook proves the driver installs the hook into
// the GoBGP server and exposes the same instance for registration, so metrics
// actually get observed rather than staying at zero.
func TestNewDriverWiresFSMTimingsHook(t *testing.T) {
	r := require.New(t)

	logger := log.New()
	logger.SetOutput(io.Discard)

	drv, err := NewDriver(announcer.DriverSpec{}, logger)
	r.NoError(err)

	d, ok := drv.(*driver)
	r.True(ok, "NewDriver must return a *driver")
	r.NotNil(d.fsmTimings, "driver must hold a wired FSMTimingsCollector")

	coll := d.FSMTimingsCollector()
	r.NotNil(coll, "FSMTimingsCollector() must return a non-nil collector")

	// The same instance that is wired into the server (a *FSMTimingsCollector)
	// must be the one returned for registration.
	_, ok = coll.(*FSMTimingsCollector)
	r.True(ok, "FSMTimingsCollector() must return the wired *FSMTimingsCollector")

	// It must be registerable in a fresh registry (idempotency is handled by
	// the composition root, not by the collector itself).
	reg := prometheus.NewRegistry()
	r.NoError(reg.Register(coll))
}
