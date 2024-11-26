package http_2xx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/runityru/anycastd/checkers"
)

var _ checkers.Checker = (*http_2xx)(nil)

const checkName = "http_2xx"

func init() {
	checkers.Register(checkName, NewFromSpec)
}

type http_2xx struct {
	client   *http.Client
	url      string
	method   string
	headers  map[string]string
	payload  []byte
	tries    uint8
	interval time.Duration
}

func New(s spec) (checkers.Checker, error) {
	client := &http.Client{
		Timeout: s.Timeout.TimeDuration(),
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}

	var payload []byte = nil
	if s.Payload != nil {
		payload = []byte(*s.Payload)
	}

	return &http_2xx{
		client:   client,
		url:      s.URL,
		method:   s.Method,
		headers:  s.Headers,
		payload:  payload,
		tries:    s.Tries,
		interval: s.Interval.TimeDuration(),
	}, nil
}

func NewFromSpec(in json.RawMessage) (checkers.Checker, error) {
	s := spec{}
	if err := json.Unmarshal(in, &s); err != nil {
		return nil, err
	}

	return New(s)
}

func (h *http_2xx) Kind() string {
	return checkName
}

func (h *http_2xx) Check(ctx context.Context) error {
	var lastErr error
	for i := 0; i < int(h.tries); i++ {
		log.WithFields(log.Fields{
			"check":   checkName,
			"attempt": i + 1,
		}).Tracef("running check")

		if err := h.check(ctx); err != nil {
			lastErr = err
			log.WithFields(log.Fields{
				"check":   checkName,
				"attempt": i + 1,
			}).Infof("error received: %s", err)
		} else {
			return nil
		}

		time.Sleep(h.interval)
	}

	if lastErr != nil {
		return errors.Errorf(
			"check failed: %d tries with %s interval; last error: `%s`",
			h.tries, h.interval, lastErr.Error(),
		)
	}
	return nil
}

func (h *http_2xx) check(ctx context.Context) error {
	var payloadReader io.Reader = nil
	if h.payload != nil {
		payloadReader = bytes.NewReader(h.payload)
	}

	req, err := http.NewRequestWithContext(ctx, h.method, h.url, payloadReader)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "anycastd/1.0 (http_2xx checker)")

	for k, v := range h.headers {
		req.Header.Set(k, v)
	}

	if v := req.Header.Get("Host"); v != "" {
		req.Host = v
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errors.Errorf("Unexpected code received: %d", resp.StatusCode)
	}

	return nil
}
