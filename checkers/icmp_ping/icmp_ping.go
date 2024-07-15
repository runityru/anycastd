package icmp_ping

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-ping/ping"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/runityru/anycastd/checkers"
)

var (
	_ checkers.Checker = (*icmp_ping)(nil)

	maxRttSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "check_max_rtt_seconds",
			Help:      "Max RTT of ICMP checks",
		},
		[]string{"check", "host"},
	)
	minRttSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "check_min_rtt_seconds",
			Help:      "Min RTT of ICMP checks",
		},
		[]string{"check", "host"},
	)
	avgRttSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "check_avg_rtt_seconds",
			Help:      "Avg RTT of ICMP checks",
		},
		[]string{"check", "host"},
	)
	stdDevRtt = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "check_std_dev_rtt_seconds",
			Help:      "Standard deviation RTT of ICMP checks",
		},
		[]string{"check", "host"},
	)

	packetsSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "anycastd",
			Name:      "check_packets_sent_total",
			Help:      "Total amount of packets sent",
		},
		[]string{"check", "host"},
	)
	packetsReceived = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "anycastd",
			Name:      "check_packets_received_total",
			Help:      "Total amount of packets received",
		},
		[]string{"check", "host"},
	)
	packetsReceivedDuplicates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "anycastd",
			Name:      "check_packets_received_duplicates_total",
			Help:      "Total amount of duplicate packets received",
		},
		[]string{"check", "host"},
	)
	packetsLoss = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "check_loss_percent",
			Help:      "Percent of packet loss",
		}, []string{"check", "host"})
)

type pingStats struct {
	PacketsRecv           int
	PacketsSent           int
	PacketsRecvDuplicates int
	PacketLoss            float64
	MinRTT                time.Duration
	MaxRTT                time.Duration
	AvgRTT                time.Duration
	StdDevRTT             time.Duration
}

const checkName = "icmp_ping"

func init() {
	checkers.Register(checkName, NewFromSpec)

	prometheus.MustRegister(maxRttSeconds)
	prometheus.MustRegister(minRttSeconds)
	prometheus.MustRegister(avgRttSeconds)
	prometheus.MustRegister(stdDevRtt)
	prometheus.MustRegister(packetsSent)
	prometheus.MustRegister(packetsReceived)
	prometheus.MustRegister(packetsReceivedDuplicates)
	prometheus.MustRegister(packetsLoss)
}

type icmp_ping struct {
	host     string
	tries    uint8
	interval time.Duration
	timeout  time.Duration

	pingerFn func(host string, tries uint8, interval, timeout time.Duration) (*pingStats, error)
}

func New(s spec) (checkers.Checker, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	return newWithPinger(s, runPing)
}

func newWithPinger(
	s spec,
	pingerFn func(host string, tries uint8, interval, timeout time.Duration) (*pingStats, error),
) (checkers.Checker, error) {
	return &icmp_ping{
		host:     s.Static.Host,
		tries:    s.Tries,
		interval: s.Interval.TimeDuration(),
		timeout:  s.Timeout.TimeDuration(),

		pingerFn: pingerFn,
	}, nil
}

func NewFromSpec(in json.RawMessage) (checkers.Checker, error) {
	s := spec{}
	if err := json.Unmarshal(in, &s); err != nil {
		return nil, err
	}

	return New(s)
}

func (p *icmp_ping) Kind() string {
	return checkName
}

func (p *icmp_ping) Check(ctx context.Context) error {
	stats, err := runPing(p.host, p.tries, p.interval, p.timeout)
	if err != nil {
		return err
	}

	packetsSent.WithLabelValues(checkName, p.host).Add(float64(stats.PacketsSent))
	packetsReceived.WithLabelValues(checkName, p.host).Add(float64(stats.PacketsRecv))
	packetsReceivedDuplicates.WithLabelValues(checkName, p.host).Add(float64(stats.PacketsRecvDuplicates))
	packetsLoss.WithLabelValues(checkName, p.host).Set(stats.PacketLoss)
	maxRttSeconds.WithLabelValues(checkName, p.host).Set(stats.MaxRTT.Seconds())
	minRttSeconds.WithLabelValues(checkName, p.host).Set(stats.MinRTT.Seconds())
	avgRttSeconds.WithLabelValues(checkName, p.host).Set(stats.AvgRTT.Seconds())
	stdDevRtt.WithLabelValues(checkName, p.host).Set(stats.StdDevRTT.Seconds())

	return nil
}

func runPing(host string, tries uint8, interval, timeout time.Duration) (*pingStats, error) {
	pinger, err := ping.NewPinger(host)
	if err != nil {
		return nil, errors.Wrap(err, "error initializing pinger")
	}

	pinger.Count = int(tries)
	pinger.Interval = interval
	pinger.Timeout = timeout

	if err := pinger.Run(); err != nil {
		return nil, errors.Wrap(err, "error running ICMP ping")
	}

	stats := pinger.Statistics()

	return &pingStats{
		PacketsRecv:           stats.PacketsRecv,
		PacketsSent:           stats.PacketsSent,
		PacketsRecvDuplicates: stats.PacketsRecvDuplicates,
		PacketLoss:            stats.PacketLoss,
		MinRTT:                stats.MinRtt,
		MaxRTT:                stats.MaxRtt,
		AvgRTT:                stats.AvgRtt,
		StdDevRTT:             stats.StdDevRtt,
	}, nil
}
