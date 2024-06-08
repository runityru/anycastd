package icmp_ping

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/teran/anycastd/config"
)

func TestCheck(t *testing.T) {
	r := require.New(t)

	c, err := newWithPinger(
		spec{
			Host:     "127.0.0.1",
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
