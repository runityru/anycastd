package http_2xx

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
				URL:      "http://127.0.0.1:8080/",
				Method:   "GET",
				Tries:    3,
				Interval: th.Duration(1 * time.Second),
				Timeout:  th.Duration(5 * time.Second),
			},
		},
		{
			name: "empty spec",
			in:   spec{},
			expError: errors.New(
				"interval: cannot be blank; method: cannot be blank; timeout: cannot be blank; tries: cannot be blank; url: cannot be blank.",
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
