package service

import (
	"github.com/prometheus/client_golang/prometheus"
)

var up = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "anycastd",
		Name:      "up",
		Help:      "Service liveness status based on checks",
	},
	[]string{"service"},
)

func init() {
	prometheus.MustRegister(up)
}
