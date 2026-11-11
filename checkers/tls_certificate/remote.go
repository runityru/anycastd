package tls_certificate

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/pkg/errors"
)

func getRemoteCertificate(spec Remote) func() ([]*x509.Certificate, error) {
	return func() ([]*x509.Certificate, error) {
		conf := &tls.Config{
			// nosec G402: spec.Insecure intentionally disables certificate
			// verification for the tls_certificate checker (documented in
			// SPEC.md §8 as a security-relevant, opt-in option).
			InsecureSkipVerify: spec.Insecure, // #nosec G402
		}

		conn, err := tls.Dial("tcp", spec.Address, conf)
		if err != nil {
			return nil, errors.Wrapf(err, "error connecting to %s", spec.Address)
		}
		defer func() { _ = conn.Close() }()

		return conn.ConnectionState().PeerCertificates, nil
	}
}
