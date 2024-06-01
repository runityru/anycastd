package http_2xx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/teran/anycastd/checkers"
)

var _ checkers.Checker = (*http_2xx)(nil)

type http_2xx struct {
	client   *http.Client
	address  string
	method   string
	path     string
	tries    uint8
	interval time.Duration
}

func New(s spec) (checkers.Checker, error) {
	client := &http.Client{
		Timeout: time.Duration(s.Timeout),
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}

	return &http_2xx{
		client:   client,
		address:  s.Address,
		method:   s.Method,
		path:     s.Path,
		tries:    s.Tries,
		interval: time.Duration(s.Interval),
	}, nil
}

func NewFromSpec(in json.RawMessage) (checkers.Checker, error) {
	s := spec{}
	if err := json.Unmarshal(in, &s); err != nil {
		return nil, err
	}

	return New(s)
}

func (h *http_2xx) Check(ctx context.Context) error {
	var lastErr error
	for i := 0; i < int(h.tries); i++ {
		if err := h.check(ctx); err != nil {
			lastErr = err
			log.Infof("error received: %s", err)
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
	req, err := http.NewRequestWithContext(ctx, h.method, h.address+h.path, nil)
	if err != nil {
		return err
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
