package ntpq

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	th "github.com/teran/go-time"
)

func TestSpecValidation(t *testing.T) {
	type testCase struct {
		name     string
		in       spec
		expError error
	}

	tcs := []testCase{
		{
			name: "valid spec",
			in: spec{
				Server:          "pool.ntp.org",
				SrcAddr:         "192.168.0.1",
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Tries:           3,
				Interval:        th.Duration(1 * time.Second),
				Timeout:         th.Duration(5 * time.Second),
			},
		},
		{
			name: "invalid server",
			in: spec{
				Server:          "$4!7",
				SrcAddr:         "192.168.0.1",
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Tries:           3,
				Interval:        th.Duration(1 * time.Second),
				Timeout:         th.Duration(5 * time.Second),
			},
			expError: errors.New("server: must be a valid IP address or DNS name."),
		},
		{
			name: "ipv6 src_addr",
			in: spec{
				Server:          "2001:db8::123",
				SrcAddr:         "2001:db8::1",
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Tries:           3,
				Interval:        th.Duration(1 * time.Second),
				Timeout:         th.Duration(5 * time.Second),
			},
		},
		{
			name: "ipv6 src_addr with hostname server",
			in: spec{
				Server:          "pool.ntp.org",
				SrcAddr:         "2001:db8::1",
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Tries:           3,
				Interval:        th.Duration(1 * time.Second),
				Timeout:         th.Duration(5 * time.Second),
			},
		},
		{
			name: "invalid src_addr",
			in: spec{
				Server:          "pool.ntp.org",
				SrcAddr:         "not-an-ip",
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Tries:           3,
				Interval:        th.Duration(1 * time.Second),
				Timeout:         th.Duration(5 * time.Second),
			},
			expError: errors.New("src_addr: must be a valid IP address."),
		},
		{
			name: "src_addr family mismatch",
			in: spec{
				Server:          "2001:db8::123",
				SrcAddr:         "192.168.0.1",
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Tries:           3,
				Interval:        th.Duration(1 * time.Second),
				Timeout:         th.Duration(5 * time.Second),
			},
			expError: errors.New("src_addr: must match the address family of server."),
		},
		{
			name: "v4-mapped src_addr family mismatch",
			in: spec{
				Server:          "2001:db8::123",
				SrcAddr:         "::ffff:192.168.0.1",
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Tries:           3,
				Interval:        th.Duration(1 * time.Second),
				Timeout:         th.Duration(5 * time.Second),
			},
			expError: errors.New("src_addr: must match the address family of server."),
		},
		{
			name: "ipv6 link-local src_addr",
			in: spec{
				Server:          "pool.ntp.org",
				SrcAddr:         "fe80::1",
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Tries:           3,
				Interval:        th.Duration(1 * time.Second),
				Timeout:         th.Duration(5 * time.Second),
			},
			expError: errors.New("src_addr: must not be an IPv6 link-local address."),
		},
		{
			name: "unspecified server",
			in: spec{
				Server:          "::",
				SrcAddr:         "2001:db8::1",
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Tries:           3,
				Interval:        th.Duration(1 * time.Second),
				Timeout:         th.Duration(5 * time.Second),
			},
			expError: errors.New("server: must not be an unspecified or IPv6 link-local address."),
		},
		{
			name: "ipv6 link-local server",
			in: spec{
				Server:          "fe80::1",
				SrcAddr:         "2001:db8::1",
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Tries:           3,
				Interval:        th.Duration(1 * time.Second),
				Timeout:         th.Duration(5 * time.Second),
			},
			expError: errors.New("server: must not be an unspecified or IPv6 link-local address."),
		},
		{
			name: "invalid threshold",
			in: spec{
				Server:   "pool.ntp.org",
				SrcAddr:  "192.168.0.1",
				Tries:    3,
				Interval: th.Duration(1 * time.Second),
				Timeout:  th.Duration(5 * time.Second),
			},
			expError: errors.New("offset_threshold: cannot be blank."),
		},
		{
			name: "empty spec",
			in:   spec{},
			expError: errors.New(
				"interval: cannot be blank; offset_threshold: cannot be blank; server: cannot be blank; src_addr: cannot be blank; timeout: cannot be blank; tries: cannot be blank.",
			),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			err := tc.in.Validate()
			if tc.expError == nil {
				r.NoError(err)
			} else {
				r.Error(err)
				r.Equal(tc.expError.Error(), err.Error())
			}
		})
	}
}
