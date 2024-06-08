package icmp_ping

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/teran/anycastd/config"
)

func TestSpec(t *testing.T) {
	r := require.New(t)

	data, err := os.ReadFile("testdata/spec.json")
	r.NoError(err)

	c, err := NewFromSpec(json.RawMessage(data))
	r.NoError(err)

	p := c.(*icmp_ping)
	r.Equal("127.0.0.33", p.host)
	r.Equal(uint8(5), p.tries)
	r.Equal(100*time.Millisecond, p.interval)
	r.Equal(5*time.Second, p.timeout)
}

func TestCheck(t *testing.T) {
	r := require.New(t)

	c, err := newWithPinger(
		spec{
			Static:   Static{Host: "127.0.0.1"},
			Tries:    10,
			Interval: config.Duration(10 * time.Second),
			Timeout:  config.Duration(30 * time.Second),
		},
		func(host string, tries uint8, interval, timeout time.Duration) (*pingStats, error) {
			r.Equal("127.0.0.1", host)
			r.Equal(uint8(10), tries)
			r.Equal(10*time.Second, interval)
			r.Equal(30*time.Second, timeout)

			return &pingStats{}, nil
		},
	)
	r.NoError(err)

	err = c.Check(context.Background())
	r.NoError(err)
}
