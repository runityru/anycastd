package ntpq

import (
	"context"
	"encoding/json"
	"net"
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

	ntpUnverified = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "anycastd",
			Name:      "check_ntp_unverified_total",
			Help:      "Total amount of checks that could not verify the local clock",
		},
		[]string{"check", "host", "src_addr"},
	)

	ErrOffset = errors.New("offset is too big")

	errUnverified = errors.New("unverified reply")
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
	prometheus.MustRegister(ntpUnverified)
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
	var lastErr, offsetErr error
	for i := 0; i < int(d.tries); i++ {
		log.WithFields(log.Fields{
			"check":   checkName,
			"attempt": i + 1,
		}).Tracef("running check")

		err := d.check(ctx)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrOffset) {
			offsetErr = err
		}
		lastErr = err
		log.WithFields(log.Fields{
			"check":   checkName,
			"attempt": i + 1,
		}).Infof("error received: %s", err)
		if errors.Is(err, errUnverified) {
			break
		}

		time.Sleep(d.interval)
	}

	if offsetErr != nil {
		return errors.Errorf(
			"check failed: %d tries with %s interval; offset error: `%s`",
			d.tries, d.interval, offsetErr.Error(),
		)
	}

	// No reply or an unverified reply leaves the route announced; only a verified offset failure withdraws it.
	ntpUnverified.WithLabelValues(checkName, d.server, d.srcAddr).Inc()
	ntpOffset.DeleteLabelValues(checkName, d.server, d.srcAddr)
	ntpRtt.DeleteLabelValues(checkName, d.server, d.srcAddr)
	log.WithFields(log.Fields{
		"check": checkName,
	}).Warnf("reference %s cannot verify local clock: %s", d.server, lastErr)
	return nil
}

func (d *ntpq) check(_ context.Context) error {
	options := ntp.QueryOptions{LocalAddress: d.srcAddr, Timeout: d.timeout, Dialer: dial}
	response, err := d.queryFn(d.server, options)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"check": checkName,
	}).Tracef("Offset: %s, RTT: %s, RefID: %s", response.ClockOffset, response.RTT, response.ReferenceString())
	ntpPacketsSent.WithLabelValues(checkName, d.server, d.srcAddr).Inc()

	if err := response.Validate(); err != nil {
		return errors.Wrapf(errUnverified, "stratum %d, refid %s: %s", response.Stratum, response.ReferenceString(), err)
	}

	ntpOffset.WithLabelValues(checkName, d.server, d.srcAddr).Set(response.ClockOffset.Seconds())
	ntpRtt.WithLabelValues(checkName, d.server, d.srcAddr).Set(response.RTT.Seconds())

	if response.ClockOffset.Abs() > d.offsetThreshold {
		return errors.Wrapf(ErrOffset, "%s exceeds %s", response.ClockOffset, d.offsetThreshold)
	}

	return nil
}

// dial binds the source address; net.Dialer then dials only remote addresses of its family.
func dial(localAddress, remoteAddress string) (net.Conn, error) {
	d := net.Dialer{LocalAddr: &net.UDPAddr{IP: net.ParseIP(localAddress)}}
	return d.Dial("udp", remoteAddress)
}
