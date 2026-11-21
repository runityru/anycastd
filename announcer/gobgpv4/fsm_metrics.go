package gobgpv4

import (
	"time"

	"github.com/osrg/gobgp/v4/pkg/server"
	"github.com/prometheus/client_golang/prometheus"
)

// fsmOperationInfixes mirrors GoBGP's own FSM metric naming (see
// pkg/metrics/metrics.go): each server.FSMOperation index maps to a stable
// infix used in the metric name. The order is fixed and matches
// server.FSMOperationTypeCount, so the arrays below stay index-aligned with
// the FSMOperation enum (FSMMgmtOp=0, FSMAccept=1, FSMROAEvent=2, FSMMessage=3).
var fsmOperationInfixes = [server.FSMOperationTypeCount]string{
	"mgmt_op",
	"accept",
	"event",
	"message",
}

// FSMTimingsCollector observes GoBGP's FSM-loop timings via the server's
// FSMTimingHook and exposes them as Prometheus histograms under the anycastd_
// namespace (not GoBGP's built-in fsm_loop_*). It implements both
// server.FSMTimingHook and prometheus.Collector so the very same instance is
// both wired into the GoBGP server (TimingHookOption) and registered with the
// metrics registry — if a different, unwired instance were registered, its
// histograms would stay permanently zero.
//
// Units: the *_timing_seconds histograms measure how long each FSM operation
// takes; the *_wait_seconds histograms measure the channel-wait delay between
// an FSM event and its handling. All values are in seconds (Prometheus
// convention for durations). The underlying prometheus.Histograms are safe for
// concurrent use by the FSM goroutines (Observe) and the scrape goroutine
// (Collect).
type FSMTimingsCollector struct {
	timing, wait [server.FSMOperationTypeCount]prometheus.Histogram
}

// Compile-time assertions that the type satisfies both required interfaces.
var (
	_ server.FSMTimingHook = (*FSMTimingsCollector)(nil)
	_ prometheus.Collector = (*FSMTimingsCollector)(nil)
)

// NewFSMTimingsCollector builds a collector with the 8 histograms under
// namespace "anycastd" and subsystem "gobgp":
//
//	anycastd_gobgp_fsm_{mgmt_op,accept,event,message}_{timing_seconds,wait_seconds}
//
// Buckets use prometheus.DefBuckets, matching GoBGP's own collector.
func NewFSMTimingsCollector() *FSMTimingsCollector {
	c := &FSMTimingsCollector{}
	for i := range c.timing {
		infix := fsmOperationInfixes[i]
		c.timing[i] = prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "anycastd",
			Subsystem: "gobgp",
			Name:      "fsm_" + infix + "_timing_seconds",
			Help:      "Histogram of GoBGP FSM " + infix + " operation timings, in seconds",
			Buckets:   prometheus.DefBuckets,
		})
		c.wait[i] = prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "anycastd",
			Subsystem: "gobgp",
			Name:      "fsm_" + infix + "_wait_seconds",
			Help:      "Histogram of GoBGP FSM " + infix + " channel wait delays, in seconds",
			Buckets:   prometheus.DefBuckets,
		})
	}
	return c
}

// Observe implements server.FSMTimingHook. It records one FSM operation's
// duration (tOp) into the timing histogram for the operation's type and, when
// non-zero, its channel-wait delay (tWait) into the corresponding wait
// histogram. An out-of-range operation is ignored defensively.
func (c *FSMTimingsCollector) Observe(op server.FSMOperation, tOp, tWait time.Duration) {
	if int(op) < 0 || int(op) >= len(c.timing) {
		return
	}
	c.timing[op].Observe(tOp.Seconds())
	if tWait != 0 {
		c.wait[op].Observe(tWait.Seconds())
	}
}

// Describe implements prometheus.Collector.
func (c *FSMTimingsCollector) Describe(ch chan<- *prometheus.Desc) {
	for i := range c.timing {
		c.timing[i].Describe(ch)
		c.wait[i].Describe(ch)
	}
}

// Collect implements prometheus.Collector.
func (c *FSMTimingsCollector) Collect(ch chan<- prometheus.Metric) {
	for i := range c.timing {
		c.timing[i].Collect(ch)
		c.wait[i].Collect(ch)
	}
}
