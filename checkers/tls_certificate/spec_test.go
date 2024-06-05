package tls_certificate

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
			name: "minimal spec",
			in: spec{
				Local: Local{
					Path: "filename",
				},
			},
		},
		{
			name: "valid full spec",
			in: spec{
				Local: Local{
					Path: "filename",
				},
				CommonName:  ptr.String("Some subject common name"),
				DNSNames:    []string{"domain1.example.com", "domain2.example.com"},
				IPAddresses: []string{"127.0.0.1"},
				Issuer:      ptr.String("Some issuer"),
			},
		},
		{
			name: "invalid DNS name",
			in: spec{
				Local: Local{
					Path: "filename",
				},
				DNSNames: []string{"blah!"},
			},
			expError: errors.New(
				"dns_names: (0: must be a valid DNS name.).",
			),
		},
		{
			name: "invalid IP address",
			in: spec{
				Local: Local{
					Path: "filename",
				},
				IPAddresses: []string{"blah"},
			},
			expError: errors.New(
				"ip_addresses: (0: must be a valid IP address.).",
			),
		},
		{
			name: "empty spec",
			in:   spec{},
			expError: errors.New(
				"local: (path: cannot be blank.).",
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
