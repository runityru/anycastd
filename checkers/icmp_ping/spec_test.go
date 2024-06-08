package icmp_ping

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/teran/anycastd/config"
)

func TestSpecValidation(t *testing.T) {
	type testCase struct {
		name     string
		in       spec
		expError error
	}

	tcs := []testCase{
		{
			name: "valid spec w/ domain name",
			in: spec{
				Host:     "test.example.org",
				Tries:    10,
				Interval: config.Duration(1 * time.Second),
				Timeout:  config.Duration(2 * time.Second),
			},
		},
		{
			name: "valid spec w/ IP address",
			in: spec{
				Host:     "127.0.0.1",
				Tries:    10,
				Interval: config.Duration(1 * time.Second),
				Timeout:  config.Duration(2 * time.Second),
			},
		},
		{
			name: "empty spec",
			in:   spec{},
			expError: errors.New(
				"host: cannot be blank; interval: cannot be blank; timeout: cannot be blank; tries: cannot be blank.",
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
