package ntpq

import (
	"context"
	"encoding/json"
	"time"

	"github.com/beevik/ntp"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/runityru/anycastd/checkers"
)

var (
	_ checkers.Checker = (*ntpq)(nil)

	ntpOffset = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "check_last_ntp_offset_seconds",
			Help:      "The estimated offset of the local system clock relative to the server's clock",
		},
		[]string{"check", "host", "src_addr"},
	)

	ntpRtt = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "check_last_ntp_rtt_seconds",
			Help:      "An estimate of the round-trip-time delay between the client and the server",
		},
		[]string{"check", "host", "src_addr"},
	)

	ntpPacketsSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "anycastd",
			Name:      "check_ntp_packets_sent_total",
			Help:      "Total amount of ntp packets sent",
		},
		[]string{"check", "host", "src_addr"},
	)

	ErrOffset = errors.New("offset is too big")
)

type ntpq struct {
	server          string
	srcAddr         string
	tries           uint8
	offsetThreshold time.Duration
	interval        time.Duration
	timeout         time.Duration

	queryFn func(string, ntp.QueryOptions) (*ntp.Response, error)
}

const checkName = "ntpq"

func init() {
	checkers.MustRegister(checkName, NewFromSpec)

	prometheus.MustRegister(ntpOffset)
	prometheus.MustRegister(ntpPacketsSent)
	prometheus.MustRegister(ntpRtt)
}

func New(s spec) (checkers.Checker, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	return &ntpq{
		server:          s.Server,
		srcAddr:         s.SrcAddr,
		tries:           s.Tries,
		offsetThreshold: s.OffsetThreshold.TimeDuration(),
		interval:        s.Interval.TimeDuration(),
		timeout:         s.Timeout.TimeDuration(),
		queryFn:         ntp.QueryWithOptions,
	}, nil
}

func NewFromSpec(in json.RawMessage) (checkers.Checker, error) {
	s := spec{}
	if err := json.Unmarshal(in, &s); err != nil {
		return nil, err
	}

	return New(s)
}

func (d *ntpq) Kind() string {
	return checkName
}

func (d *ntpq) Check(ctx context.Context) error {
	var lastErr error
	for i := 0; i < int(d.tries); i++ {
		log.WithFields(log.Fields{
			"check":   checkName,
			"attempt": i + 1,
		}).Tracef("running check")

		if err := d.check(ctx); err != nil {
			lastErr = err
			log.WithFields(log.Fields{
				"check":   checkName,
				"attempt": i + 1,
			}).Infof("error received: %s", err)
		} else {
			return nil
		}

		time.Sleep(d.interval)
	}

	if lastErr != nil {
		return errors.Errorf(
			"check failed: %d tries with %s interval; last error: `%s`",
			d.tries, d.interval, lastErr.Error(),
		)
	}
	return nil
}

func (d *ntpq) check(_ context.Context) error {
	options := ntp.QueryOptions{LocalAddress: d.srcAddr, Timeout: d.timeout}
	response, err := d.queryFn(d.server, options)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"check": checkName,
	}).Tracef("Offset: %s, RTT: %s, RefID: %s", response.ClockOffset, response.RTT, response.ReferenceString())
	ntpPacketsSent.WithLabelValues(checkName, d.server, d.srcAddr).Inc()
	ntpOffset.WithLabelValues(checkName, d.server, d.srcAddr).Set(response.ClockOffset.Seconds())
	ntpRtt.WithLabelValues(checkName, d.server, d.srcAddr).Set(response.RTT.Seconds())

	if response.ClockOffset.Abs() > d.offsetThreshold {
		return errors.Wrapf(ErrOffset, "%s exceeds %s", response.ClockOffset, d.offsetThreshold)
	}

	return nil
}
