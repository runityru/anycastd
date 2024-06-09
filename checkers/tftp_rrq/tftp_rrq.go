package tftp_rrq

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/pin/tftp"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/teran/anycastd/checkers"
)

var _ checkers.Checker = (*tftp_rrq)(nil)

const checkName = "http_2xx"

type tftp_rrq struct {
	url       string
	expSHA256 *string
	tries     uint8
	interval  time.Duration
	timeout   time.Duration
}

func New(s spec) (checkers.Checker, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	return &tftp_rrq{
		url:       s.URL,
		expSHA256: s.ExpectedSHA256,
		tries:     s.Tries,
		interval:  time.Duration(s.Interval),
		timeout:   time.Duration(s.Timeout),
	}, nil
}

func NewFromSpec(in json.RawMessage) (checkers.Checker, error) {
	s := spec{}
	if err := json.Unmarshal(in, &s); err != nil {
		return nil, err
	}

	return New(s)
}

func (t *tftp_rrq) Kind() string {
	return checkName
}

func (t *tftp_rrq) Check(ctx context.Context) error {
	var lastErr error
	for i := 0; i < int(t.tries); i++ {
		log.WithFields(log.Fields{
			"check":   checkName,
			"attempt": i + 1,
		}).Tracef("running check")

		if err := t.check(ctx); err != nil {
			lastErr = err
			log.WithFields(log.Fields{
				"check":   checkName,
				"attempt": i + 1,
			}).Infof("error received: %s", err)
		} else {
			return nil
		}

		time.Sleep(t.interval)
	}

	if lastErr != nil {
		return errors.Errorf(
			"check failed: %d tries with %s interval; last error: `%s`",
			t.tries, t.interval, lastErr.Error(),
		)
	}

	return nil
}

func (t *tftp_rrq) check(context.Context) error {
	url, err := url.Parse(t.url)
	if err != nil {
		return errors.Wrap(err, "error parsing URL")
	}

	host := url.Host
	path := url.RequestURI()

	c, err := tftp.NewClient(host)
	if err != nil {
		return errors.Wrap(err, "error creating TFTP client")
	}

	wt, err := c.Receive(path, "octet")
	if err != nil {
		return errors.Wrap(err, "error receiving file")
	}

	buf := &bytes.Buffer{}
	n, err := wt.WriteTo(buf)
	if err != nil {
		return errors.Wrap(err, "error filling buffer")
	}

	log.WithFields(log.Fields{
		"check": checkName,
	}).Tracef("bytes received: %d", n)

	if t.expSHA256 != nil {
		exp := *t.expSHA256
		act := fmt.Sprintf("%x", sha256.Sum256(buf.Bytes()))
		if exp != act {
			return errors.Errorf(
				"checksum mismatch: expected `%s` != actual `%s`",
				exp, act,
			)
		}
	}

	return nil
}
