package service

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics interface {
	ServiceUp(service string)
	ServiceDown(service string)

	MeasureCall(ctx context.Context, service, check string, fn func(ctx context.Context) error) error
}

type metrics struct {
	upGauge       *prometheus.GaugeVec
	checkDuration *prometheus.GaugeVec
}

func NewMetrics() (Metrics, error) {
	upGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "up",
			Help:      "Service liveness status based on checks",
		},
		[]string{"service"},
	)

	checkDuration := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "check_duration",
			Help:      "Service liveness status based on checks",
		},
		[]string{"service", "check"},
	)

	if err := prometheus.Register(upGauge); err != nil {
		return nil, err
	}

	if err := prometheus.Register(checkDuration); err != nil {
		return nil, err
	}

	return &metrics{
		upGauge: upGauge,
	}, nil
}

func (m *metrics) ServiceUp(service string) {
	m.upGauge.WithLabelValues(service).Set(1.0)
}

func (m *metrics) ServiceDown(service string) {
	m.upGauge.WithLabelValues(service).Set(0.0)
}

func (m *metrics) MeasureCall(ctx context.Context, service, check string, fn func(ctx context.Context) error) error {
	start := time.Now()

	err := fn(ctx)

	m.checkDuration.WithLabelValues(service, check).Set(float64(time.Now().Sub(start)))

	return err
}
