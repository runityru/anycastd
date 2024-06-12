package tftp_rrq

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	th "github.com/teran/go-time"

	"github.com/teran/go-ptr"
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
				URL:      "tftp://127.0.0.1:69/lpxelinux.0",
				Tries:    3,
				Interval: th.Duration(1 * time.Second),
				Timeout:  th.Duration(2 * time.Second),
			},
		},
		{
			name: "valid spec w/ checksum",
			in: spec{
				URL:            "tftp://127.0.0.1:69/lpxelinux.0",
				ExpectedSHA256: ptr.String("09da9c01b6b2a8ccc5d3445c4f364243d8a063bd0bf520643737899e6ce0170f"),
				Tries:          3,
				Interval:       th.Duration(1 * time.Second),
				Timeout:        th.Duration(2 * time.Second),
			},
		},
		{
			name: "empty spec",
			in:   spec{},
			expError: errors.New(
				"interval: cannot be blank; timeout: cannot be blank; tries: cannot be blank; url: cannot be blank.",
			),
		},
		{
			name: "valid spec w/ invalid checksum (not hex)",
			in: spec{
				URL:            "tftp://127.0.0.1:69/lpxelinux.0",
				ExpectedSHA256: ptr.String("09da9c01b6b2a8ccc5d3445c4f364243d8a063bd0bf520643737899e6ce0170s"),
				Tries:          3,
				Interval:       th.Duration(1 * time.Second),
				Timeout:        th.Duration(2 * time.Second),
			},
			expError: errors.New("expected_sha256: must be a valid hexadecimal number."),
		},
		{
			name: "valid spec w/ invalid checksum (wrong length)",
			in: spec{
				URL:            "tftp://127.0.0.1:69/lpxelinux.0",
				ExpectedSHA256: ptr.String("deadbeef"),
				Tries:          3,
				Interval:       th.Duration(1 * time.Second),
				Timeout:        th.Duration(2 * time.Second),
			},
			expError: errors.New("expected_sha256: the length must be exactly 64."),
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
