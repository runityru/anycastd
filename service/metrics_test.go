package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/runityru/anycastd/announcer"
)

// newTestMetrics builds a real *metrics via NewMetrics on a fresh, hermetic
// registry so tests never touch the global default registerer (which is shared
// with the checkers registered in init()).
func newTestMetrics(t *testing.T, appVersion string) (*metrics, *prometheus.Registry) {
	t.Helper()

	reg := prometheus.NewRegistry()
	m, err := NewMetrics(appVersion, WithRegisterer(reg))
	require.NoError(t, err)

	mm, ok := m.(*metrics)
	require.True(t, ok, "NewMetrics should return *metrics")

	return mm, reg
}

// TestMetricsNewMetricsSetsAppUp kills any mutation that drops the initial
// Set(1) on the anycastd_up{version} gauge: after construction the gauge must
// read exactly 1.
func TestMetricsNewMetricsSetsAppUp(t *testing.T) {
	m, _ := newTestMetrics(t, "1.2.3")

	require.Equal(t, 1.0, testutil.ToFloat64(m.appUpGauge.WithLabelValues("1.2.3")))
}

// TestMetricsServiceUpDown kills swapped Set calls: ServiceUp must set 1.0 and
// ServiceDown must set 0.0 on the service_up{service} gauge.
func TestMetricsServiceUpDown(t *testing.T) {
	m, _ := newTestMetrics(t, "test")

	m.ServiceUp("x")
	require.Equal(t, 1.0, testutil.ToFloat64(m.upGauge.WithLabelValues("x")))

	m.ServiceDown("x")
	require.Equal(t, 0.0, testutil.ToFloat64(m.upGauge.WithLabelValues("x")))
}

// TestMetricsServiceUpDownToggle is the Issue #120 regression test: driving the
// real metrics through run() must flip service_up 1<->0 as the check toggles.
func TestMetricsServiceUpDownToggle(t *testing.T) {
	r := require.New(t)

	m, _ := newTestMetrics(t, "test")
	tc := &toggleChecker{kind: "icmp_ping"}
	a := &countingAnnouncer{}

	strategy, err := GetStrategyNoOptions("")
	r.NoError(err)

	svc := New("svc", a, []Checker{{tc, ""}}, 10*time.Millisecond, m, strategy).(*service)

	// Healthy -> service_up = 1.
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.upGauge.WithLabelValues("svc")))

	// Flip to failing -> service_up = 0.
	tc.setFail(true)
	r.NoError(svc.run(t.Context()))
	r.Equal(0.0, testutil.ToFloat64(m.upGauge.WithLabelValues("svc")))

	// Back to healthy -> service_up = 1 again.
	tc.setFail(false)
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.upGauge.WithLabelValues("svc")))
}

// TestMetricsMeasureCallDuration asserts that a slow check populates a
// positive check_duration_seconds value.
func TestMetricsMeasureCallDuration(t *testing.T) {
	m, _ := newTestMetrics(t, "test")

	err := m.MeasureCall(context.Background(), "svc", "check", func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, testutil.ToFloat64(m.checkDurationSeconds.WithLabelValues("svc", "check")), 0.0)
}

// TestMetricsMeasureCallError kills a mutation that drops the returned error:
// the original error must be returned AND the duration still recorded.
func TestMetricsMeasureCallError(t *testing.T) {
	m, _ := newTestMetrics(t, "test")

	sentinel := errors.New("check boom")
	err := m.MeasureCall(context.Background(), "svc", "check", func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	require.Greater(t, testutil.ToFloat64(m.checkDurationSeconds.WithLabelValues("svc", "check")), 0.0)
}

// TestMetricsNewMetricsDuplicateRegistration asserts that a second NewMetrics on
// the same registry fails with an AlreadyRegisteredError. This only applies to
// a fresh, isolated registry — never the global default registerer.
func TestMetricsNewMetricsDuplicateRegistration(t *testing.T) {
	reg := prometheus.NewRegistry()

	_, err := NewMetrics("v1", WithRegisterer(reg))
	require.NoError(t, err)

	_, err = NewMetrics("v1", WithRegisterer(reg))
	require.Error(t, err)

	var already prometheus.AlreadyRegisteredError
	require.ErrorAs(t, err, &already)
}

// TestMetricsWithRegistererNilDefaultsToDefaultRegisterer asserts that passing
// WithRegisterer(nil) behaves exactly like the default: the option resolves a
// nil registerer to prometheus.DefaultRegisterer. The option closure is
// exercised directly so the (global) default registerer is never written to.
func TestMetricsWithRegistererNilDefaultsToDefaultRegisterer(t *testing.T) {
	m := &metrics{}
	WithRegisterer(nil)(m)
	require.Equal(t, prometheus.DefaultRegisterer, m.registerer)
}

// TestMetricsWithRegistererAppliesToProvidedRegistry kills a mutation where
// WithRegisterer is ignored (e.g. registration leaking onto the global default
// registerer): the supplied hermetic registry must actually receive the
// collectors.
func TestMetricsWithRegistererAppliesToProvidedRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()

	_, err := NewMetrics("v1", WithRegisterer(reg))
	require.NoError(t, err)

	families, err := reg.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, families, "collectors must be registered on the provided registry, not the default one")
}

// TestMetricsCollectsExpectedFamilies kills a mutation that drops the
// registration of a collector (the object-valued gauge tests still pass even if
// a collector is never registered). Gathering the registry proves anycastd_up
// is actually exported.
func TestMetricsCollectsExpectedFamilies(t *testing.T) {
	_, reg := newTestMetrics(t, "1.2.3")

	expected := `
# HELP anycastd_up Application liveness status (must always be 1)
# TYPE anycastd_up gauge
anycastd_up{version="1.2.3"} 1
`
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(expected), "anycastd_up"))
}

// TestMetricsServiceAnnounced drives the real gauge directly: announced=true
// sets 1.0, announced=false sets 0.0.
func TestMetricsServiceAnnounced(t *testing.T) {
	m, _ := newTestMetrics(t, "test")

	m.ServiceAnnounced("x", true)
	require.Equal(t, 1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("x")))

	m.ServiceAnnounced("x", false)
	require.Equal(t, 0.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("x")))

	// Re-announce to prove the gauge is overwritten, not sticky.
	m.ServiceAnnounced("x", true)
	require.Equal(t, 1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("x")))
}

// TestMetricsServiceAnnouncedToggle drives the real metrics through run() and
// asserts that service_announced{service} mirrors the actual BGP announce
// state: 1 after a successful announce, 0 after a successful denounce.
func TestMetricsServiceAnnouncedToggle(t *testing.T) {
	r := require.New(t)

	m, _ := newTestMetrics(t, "test")
	tc := &toggleChecker{kind: "icmp_ping"}
	a := &countingAnnouncer{}

	strategy, err := GetStrategyNoOptions("")
	r.NoError(err)

	svc := New("svc", a, []Checker{{tc, ""}}, 10*time.Millisecond, m, strategy).(*service)

	// Healthy -> announce -> announced = 1.
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Flip to failing -> denounce -> announced = 0.
	tc.setFail(true)
	r.NoError(svc.run(t.Context()))
	r.Equal(0.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Back to healthy -> announce -> announced = 1.
	tc.setFail(false)
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))
}

// failFirstAnnouncer is an announcer.Announcer fake that fails its first
// Announce, letting a test drive the announce-retry path.
type failFirstAnnouncer struct {
	announces int
}

func (f *failFirstAnnouncer) Announce(context.Context) error {
	f.announces++
	if f.announces == 1 {
		return errors.New("announce error")
	}
	return nil
}

func (f *failFirstAnnouncer) Denounce(context.Context) error { return nil }

// TestMetricsServiceAnnouncedReflectsAnnounceRetry asserts that a failed
// Announce leaves the announced gauge at 0 (the service is not actually
// announced), and that a retried successful Announce flips it to 1.
func TestMetricsServiceAnnouncedReflectsAnnounceRetry(t *testing.T) {
	r := require.New(t)

	m, _ := newTestMetrics(t, "test")
	tc := &toggleChecker{kind: "icmp_ping"}
	a := &failFirstAnnouncer{}

	strategy, err := GetStrategyNoOptions("")
	r.NoError(err)

	svc := New("svc", a, []Checker{{tc, ""}}, 10*time.Millisecond, m, strategy).(*service)

	// Run 1: healthy, but Announce fails -> announced stays false -> gauge 0.
	r.NoError(svc.run(t.Context()))
	r.Equal(0.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Run 2: healthy, Announce retried and succeeds -> announced true -> gauge 1.
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))
}

// TestMetricsServiceAnnouncedWithAttemptTrackerWindow asserts that within an
// anti-flap window (below the announce/denounce threshold) the announced gauge
// keeps its previous value, only changing when a transition actually fires.
func TestMetricsServiceAnnouncedWithAttemptTrackerWindow(t *testing.T) {
	r := require.New(t)

	m, _ := newTestMetrics(t, "test")
	tc := &toggleChecker{kind: "icmp_ping"}
	tc.setFail(true) // start down to reset the cold-start success streak
	a := &countingAnnouncer{}

	strategy, err := GetStrategyNoOptions("")
	r.NoError(err)

	svc := New("svc", a, []Checker{{tc, ""}}, 10*time.Millisecond, m, strategy, WithAttemptTracker(3, 2)).(*service)

	// Two failing ticks below the M=2 denounce threshold: never announced -> 0.
	r.NoError(svc.run(t.Context()))
	r.Equal(0.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))
	r.NoError(svc.run(t.Context()))
	r.Equal(0.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Healthy. First two up ticks stay below N=3 (no announce) -> gauge stays 0.
	tc.setFail(false)
	r.NoError(svc.run(t.Context()))
	r.Equal(0.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))
	r.NoError(svc.run(t.Context()))
	r.Equal(0.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Third up tick crosses N=3 -> announce -> gauge flips to 1.
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))
}

// TestMetricsCollectsServiceAnnouncedFamily kills a mutation that drops the
// registration of the service_announced collector. With no series set yet, the
// family must still be exported (headers present, no data lines).
func TestMetricsCollectsServiceAnnouncedFamily(t *testing.T) {
	_, reg := newTestMetrics(t, "1.2.3")

	expected := `
# HELP anycastd_service_announced Effective BGP announce state (1 if locally announced AND at least one peer session is ESTABLISHED, 0 otherwise)
# TYPE anycastd_service_announced gauge
`
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(expected), "anycastd_service_announced"))
}

// failFirstDenouncer is an announcer.Announcer fake that succeeds at Announce and
// fails its first Denounce, letting a test drive the denounce-retry path.
type failFirstDenouncer struct {
	denounces int
}

func (f *failFirstDenouncer) Announce(context.Context) error { return nil }

func (f *failFirstDenouncer) Denounce(context.Context) error {
	f.denounces++
	if f.denounces == 1 {
		return errors.New("denounce error")
	}
	return nil
}

// TestMetricsServiceAnnouncedReflectsDenounceRetry asserts that a failed Denounce
// leaves the announced gauge at 1 (we still believe we're announced), and that a
// retried successful Denounce flips it to 0.
func TestMetricsServiceAnnouncedReflectsDenounceRetry(t *testing.T) {
	r := require.New(t)

	m, _ := newTestMetrics(t, "test")
	tc := &toggleChecker{kind: "icmp_ping"}
	a := &failFirstDenouncer{}

	strategy, err := GetStrategyNoOptions("")
	r.NoError(err)

	svc := New("svc", a, []Checker{{tc, ""}}, 10*time.Millisecond, m, strategy).(*service)

	// Run 1: healthy -> announce -> gauge 1.
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Run 2: down, denounce fails -> announced stays true -> gauge stays 1.
	tc.setFail(true)
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Run 3: down, denounce retried and succeeds -> gauge 0.
	r.NoError(svc.run(t.Context()))
	r.Equal(0.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))
}

// TestMetricsServiceAnnouncedDenounceWindow asserts that within the anti-flap
// denounce window (M consecutive failures) the gauge keeps its previous value,
// only dropping to 0 once the M-th consecutive failure actually fires the
// Denounce transition.
func TestMetricsServiceAnnouncedDenounceWindow(t *testing.T) {
	r := require.New(t)

	m, _ := newTestMetrics(t, "test")
	tc := &toggleChecker{kind: "icmp_ping"}
	a := &countingAnnouncer{}

	strategy, err := GetStrategyNoOptions("")
	r.NoError(err)

	svc := New("svc", a, []Checker{{tc, ""}}, 10*time.Millisecond, m, strategy, WithAttemptTracker(1, 3)).(*service)

	// Healthy -> announce immediately (N=1) -> gauge 1.
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Two failing ticks below M=3: no denounce yet -> gauge stays 1.
	tc.setFail(true)
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Third failing tick crosses M=3 -> denounce -> gauge 0.
	r.NoError(svc.run(t.Context()))
	r.Equal(0.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))
}

// TestMetricsServiceAnnouncedReflectsPeerState drives the real gauge through
// run() with a peer-state source and asserts service_announced reflects the
// effective state: 1 when announced AND at least one peer is ESTABLISHED, 0 when
// announced but no peer is established.
func TestMetricsServiceAnnouncedReflectsPeerState(t *testing.T) {
	r := require.New(t)

	m, _ := newTestMetrics(t, "test")
	tc := &toggleChecker{kind: "icmp_ping"}
	a := &countingAnnouncer{}
	src := &fakePeerStateSource{states: []announcer.PeerState{{SessionState: announcer.PeerSessionEstablished}}}

	strategy, err := GetStrategyNoOptions("")
	r.NoError(err)

	svc := New("svc", a, []Checker{{tc, ""}}, 10*time.Millisecond, m, strategy,
		WithPeerStateSource(src)).(*service)

	// Healthy + established peer -> gauge 1.
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Peer session drops -> effective announced = 0 even though locally announced.
	src.setStates([]announcer.PeerState{{SessionState: 4}})
	r.NoError(svc.run(t.Context()))
	r.Equal(0.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))

	// Peer back up -> effective announced = 1 again (no re-announce side effect).
	src.setStates([]announcer.PeerState{{SessionState: announcer.PeerSessionEstablished}})
	r.NoError(svc.run(t.Context()))
	r.Equal(1.0, testutil.ToFloat64(m.announcedGauge.WithLabelValues("svc")))
}
