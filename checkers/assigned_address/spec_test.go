package assigned_address

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	ptr "github.com/teran/go-ptr"
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
				Interface: ptr.String("test00"),
				IPv4:      "127.0.0.33",
			},
		},
		{
			name: "valid spec w/o interface",
			in: spec{
				IPv4: "127.0.0.33",
			},
		},
		{
			name: "invalid ipv4 address format",
			in: spec{
				IPv4: "blah",
			},
			expError: errors.New("ipv4: must be a valid IPv4 address."),
		},
		{
			name: "invalid ipv4 address value",
			in: spec{
				IPv4: "127.0.0.333",
			},
			expError: errors.New("ipv4: must be a valid IPv4 address."),
		},
		{
			name:     "empty spec",
			in:       spec{},
			expError: errors.New("ipv4: cannot be blank."),
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
