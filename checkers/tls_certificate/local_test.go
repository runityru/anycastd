package tls_certificate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetLocalCertificateSingle(t *testing.T) {
	r := require.New(t)

	fn := getLocalCertificate(Local{
		Path: "testdata/test_cert.pem",
	})
	r.NotNil(fn)

	crts, err := fn()
	r.NoError(err)
	r.Len(crts, 1)
	r.Equal(crts[0].Subject.CommonName, "Test certificate")
}

func TestGetLocalCertificateMultiple(t *testing.T) {
	r := require.New(t)

	fn := getLocalCertificate(Local{
		Path: "testdata/test_cert_with_ca.pem",
	})
	r.NotNil(fn)

	crts, err := fn()
	r.NoError(err)
	r.Len(crts, 2)
	r.Equal("Test certificate", crts[0].Subject.CommonName)
	r.Equal("Test CA", crts[1].Subject.CommonName)
}
