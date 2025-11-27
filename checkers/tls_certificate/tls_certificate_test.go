package tls_certificate

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	ptr "github.com/teran/go-ptr"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestSpec(t *testing.T) {
	r := require.New(t)

	data, err := os.ReadFile("testdata/spec.json")
	r.NoError(err)

	c, err := NewFromSpec(json.RawMessage(data))
	r.NoError(err)

	tc := c.(*tls_certificate)
	r.Equal("/etc/ssl/cert.pem", tc.path)
	r.Equal(ptr.String("test cert"), tc.commonName)
	r.Equal([]string{
		"test-host.example.org",
		"test-host-2.example.org",
	}, tc.dnsNames)
	r.Equal([]string{"127.0.0.34"}, tc.ipAddresses)
	r.Equal(ptr.String("test issuer"), tc.issuer)
}

func TestTLSCertificateLocal(t *testing.T) {
	type testCase struct {
		name     string
		in       spec
		expError error
	}

	tcs := []testCase{
		{
			name: "simple valid certificate",
			in: spec{
				Local: &Local{Path: "testdata/test_cert.pem"},
			},
		},
		{
			name: "simple valid certificate w/ CN check",
			in: spec{
				Local:      &Local{Path: "testdata/test_cert.pem"},
				CommonName: ptr.String("Test certificate"),
			},
		},
		{
			name: "simple valid certificate w/ DNS check",
			in: spec{
				Local:    &Local{Path: "testdata/test_cert.pem"},
				DNSNames: []string{"test.example.org"},
			},
		},
		{
			name: "simple valid certificate w/ SAN check",
			in: spec{
				Local:       &Local{Path: "testdata/test_cert.pem"},
				IPAddresses: []string{"127.0.0.1"},
			},
		},
		{
			name: "simple valid certificate w/ issuer check",
			in: spec{
				Local:  &Local{Path: "testdata/test_cert.pem"},
				Issuer: ptr.String("Test CA"),
			},
		},
		{
			name: "expired certificate",
			in: spec{
				Local: &Local{Path: "testdata/expired_cert.pem"},
			},
			expError: errors.Errorf(
				"Certificate is expired %d seconds ago",
				int64(time.Since(time.Date(2025, 11, 28, 16, 43, 7, 0, time.UTC)).Seconds()),
			),
		},
	}

	ctx := context.Background()
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			c, err := New(tc.in)
			r.NoError(err)

			err = c.Check(ctx)
			if tc.expError == nil {
				r.NoError(err)
			} else {
				r.Error(err)
				r.Equal(tc.expError.Error(), err.Error())
			}
		})
	}
}
