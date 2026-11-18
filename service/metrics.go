package service

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics interface {
	ServiceUp(service string)
	ServiceDown(service string)
	ServiceAnnounced(service string, announced bool)

	MeasureCall(ctx context.Context, service, check string, fn func(ctx context.Context) error) error
}

type metrics struct {
	registerer           prometheus.Registerer
	appUpGauge           *prometheus.GaugeVec
	upGauge              *prometheus.GaugeVec
	announcedGauge       *prometheus.GaugeVec
	checkDurationSeconds *prometheus.GaugeVec
}

// MetricsOption configures a Metrics built by NewMetrics. Options are applied
// after the defaults, so a later option wins.
type MetricsOption func(*metrics)

// WithRegisterer sets the prometheus.Registerer the collectors are registered
// to. A nil registerer (explicit or the default) resolves to
// prometheus.DefaultRegisterer, matching the legacy behaviour.
func WithRegisterer(reg prometheus.Registerer) MetricsOption {
	return func(m *metrics) {
		if reg == nil {
			reg = prometheus.DefaultRegisterer
		}
		m.registerer = reg
	}
}

func NewMetrics(appVersion string, opts ...MetricsOption) (Metrics, error) {
	m := &metrics{registerer: prometheus.DefaultRegisterer}

	for _, opt := range opts {
		opt(m)
	}

	appUpGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "up",
			Help:      "Application liveness status (must always be 1)",
		},
		[]string{"version"},
	)

	upGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "service_up",
			Help:      "Service liveness status based on checks",
		},
		[]string{"service"},
	)

	announcedGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "service_announced",
			Help:      "Effective BGP announce state (1 if locally announced AND at least one peer session is ESTABLISHED, 0 otherwise)",
		},
		[]string{"service"},
	)

	checkDurationSeconds := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "check_duration_seconds",
			Help:      "Duration of check execution in seconds",
		},
		[]string{"service", "check"},
	)

	for _, mc := range []prometheus.Collector{appUpGauge, upGauge, announcedGauge, checkDurationSeconds} {
		if err := m.registerer.Register(mc); err != nil {
			return nil, err
		}
	}

	appUpGauge.WithLabelValues(appVersion).Set(1)

	m.appUpGauge = appUpGauge
	m.upGauge = upGauge
	m.announcedGauge = announcedGauge
	m.checkDurationSeconds = checkDurationSeconds

	return m, nil
}

func (m *metrics) ServiceUp(service string) {
	m.upGauge.WithLabelValues(service).Set(1.0)
}

func (m *metrics) ServiceDown(service string) {
	m.upGauge.WithLabelValues(service).Set(0.0)
}

func (m *metrics) ServiceAnnounced(service string, announced bool) {
	if announced {
		m.announcedGauge.WithLabelValues(service).Set(1.0)
	} else {
		m.announcedGauge.WithLabelValues(service).Set(0.0)
	}
}

func (m *metrics) MeasureCall(ctx context.Context, service, check string, fn func(ctx context.Context) error) error {
	start := time.Now()

	err := fn(ctx)

	m.checkDurationSeconds.WithLabelValues(service, check).Set(time.Since(start).Seconds())

	return err
}
