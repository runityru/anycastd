package tls_certificate

import (
	"crypto/x509"
	"encoding/pem"
	"os"

	"github.com/pkg/errors"
)

func getLocalCertificate(spec Local) func() ([]*x509.Certificate, error) {
	return func() ([]*x509.Certificate, error) {
		data, err := os.ReadFile(spec.Path)
		if err != nil {
			return nil, errors.Wrap(err, "error opening certificate file")
		}

		certs := []*x509.Certificate{}
		var derBlock *pem.Block
		for {
			derBlock, data = pem.Decode(data)
			if derBlock == nil {
				break
			}

			if derBlock.Type == "CERTIFICATE" {
				crt, err := x509.ParseCertificate(derBlock.Bytes)
				if err != nil {
					return nil, errors.Wrap(err, "error parsing certificate")
				}
				certs = append(certs, crt)
			}
		}

		return certs, nil
	}
}
