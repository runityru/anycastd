package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// newTestMetrics builds a real *metrics via NewMetrics on a fresh, hermetic
// registry so tests never touch the global default registerer (which is shared
// with the checkers registered in init()).
func newTestMetrics(t *testing.T, appVersion string) (*metrics, *prometheus.Registry) {
	t.Helper()

	reg := prometheus.NewRegistry()
	m, err := NewMetrics(reg, appVersion)
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

	_, err := NewMetrics(reg, "v1")
	require.NoError(t, err)

	_, err = NewMetrics(reg, "v1")
	require.Error(t, err)

	var already prometheus.AlreadyRegisteredError
	require.ErrorAs(t, err, &already)
}
