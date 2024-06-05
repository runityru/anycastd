package dns_lookup

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/teran/anycastd/checkers"
)

var (
	_ checkers.Checker = (*dns_lookup)(nil)

	ErrNoRecords = errors.New("NXDOMAIN")
)

type resolver interface {
	LookupHost(ctx context.Context, host string) (addrs []string, err error)
}

type dns_lookup struct {
	query    string
	resolver string
	tries    uint8
	interval time.Duration
	timeout  time.Duration

	resolverMaker func(resolver string, timeout time.Duration) resolver
}

const checkName = "dns_lookup"

func init() {
	checkers.Register(checkName, NewFromSpec)
}

func New(s spec) (checkers.Checker, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	return &dns_lookup{
		query:    s.Query,
		resolver: s.Resolver,
		tries:    s.Tries,
		interval: time.Duration(s.Interval),
		timeout:  time.Duration(s.Timeout),

		resolverMaker: mkResolver,
	}, nil
}

func NewFromSpec(in json.RawMessage) (checkers.Checker, error) {
	s := spec{}
	if err := json.Unmarshal(in, &s); err != nil {
		return nil, err
	}

	return New(s)
}

func (h *dns_lookup) Kind() string {
	return checkName
}

func (d *dns_lookup) Check(ctx context.Context) error {
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

func (d *dns_lookup) check(ctx context.Context) error {
	r := d.resolverMaker(d.resolver, d.timeout)

	addrs, err := r.LookupHost(ctx, d.query)
	if err != nil {
		return err
	}

	if len(addrs) == 0 {
		return ErrNoRecords
	}

	return nil
}

func mkResolver(resolver string, timeout time.Duration) resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{
				Timeout: timeout,
			}
			return dialer.DialContext(ctx, network, resolver)
		},
	}
}
