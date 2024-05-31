package config

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestDuration(t *testing.T) {
	type testCase struct {
		name     string
		in       string
		expOut   Duration
		expError error
	}
	tcs := []testCase{
		{
			name:   "number",
			in:     "10",
			expOut: Duration(time.Duration(10)),
		},
		{
			name:   "number w/ suffix",
			in:     `"10s"`,
			expOut: Duration(time.Duration(10 * time.Second)),
		},
		{
			name:     "invalid string",
			in:       `"blah"`,
			expError: errors.Errorf(`time: invalid duration "blah"`),
		},
		{
			name:     "boolean",
			in:       "true",
			expError: errors.Errorf("invalid value: `true` (bool)"),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			v := new(Duration)
			err := json.Unmarshal([]byte(tc.in), v)
			if tc.expError == nil {
				r.NoError(err)
				r.Equal(&tc.expOut, v)
			} else {
				r.Error(err)
				r.Equal(tc.expError.Error(), err.Error())
			}
		})
	}
}
