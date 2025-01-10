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
			expError: errors.New("server: must be a valid host."),
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
			expError: errors.New("threshold must be set"),
		},
		{
			name: "empty spec",
			in:   spec{},
			expError: errors.New(
				"server: cannot be blank; src_addr: cannot be blank; offseth_threshold: cannot be blank; timeout: cannot be blank; tries: cannot be blank.",
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
