package dns_lookup

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
			name: "valid spec",
			in: spec{
				Query:    "example.com",
				Resolver: "127.0.0.1:53",
				Tries:    3,
				Interval: config.Duration(1 * time.Second),
				Timeout:  config.Duration(5 * time.Second),
			},
		},
		{
			name: "invalid query",
			in: spec{
				Query:    "blah!",
				Resolver: "127.0.0.1:53",
				Tries:    3,
				Interval: config.Duration(1 * time.Second),
				Timeout:  config.Duration(5 * time.Second),
			},
			expError: errors.New("query: must be a valid DNS name."),
		},
		{
			name: "invalid resolver",
			in: spec{
				Query:    "example.com",
				Resolver: "!!!",
				Tries:    3,
				Interval: config.Duration(1 * time.Second),
				Timeout:  config.Duration(5 * time.Second),
			},
			expError: errors.New("resolver: must be a valid dial string."),
		},
		{
			name: "empty spec",
			in:   spec{},
			expError: errors.New(
				"interval: cannot be blank; query: cannot be blank; resolver: cannot be blank; timeout: cannot be blank; tries: cannot be blank.",
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
