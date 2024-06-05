package service

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics interface {
	ServiceUp(service string)
	ServiceDown(service string)
}

type metrics struct {
	upGauge *prometheus.GaugeVec
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

	if err := prometheus.Register(upGauge); err != nil {
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
