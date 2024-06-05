package tls_certificate

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"time"

	"github.com/pkg/errors"

	"github.com/teran/anycastd/checkers"
)

var _ checkers.Checker = (*tls_certificate)(nil)

const checkName = "tls_certificate"

func init() {
	checkers.Register(checkName, NewFromSpec)
}

type tls_certificate struct {
	path        string
	commonName  *string
	dnsNames    []string
	ipAddresses []string
	issuer      *string
}

func New(s spec) (checkers.Checker, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	return &tls_certificate{
		path:        s.Local.Path,
		commonName:  s.CommonName,
		dnsNames:    s.DNSNames,
		ipAddresses: s.IPAddresses,
		issuer:      s.Issuer,
	}, nil
}

func NewFromSpec(in json.RawMessage) (checkers.Checker, error) {
	s := spec{}
	if err := json.Unmarshal(in, &s); err != nil {
		return nil, err
	}

	return New(s)
}

func (s *tls_certificate) Kind() string {
	return checkName
}

func (s *tls_certificate) Check(ctx context.Context) error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return errors.Wrap(err, "error opening certificate file")
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return errors.New("empty data block received from PEM container")
	}

	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return errors.Wrap(err, "error parsing certificate")
	}

	if ttl := int(time.Since(crt.NotAfter).Seconds()); ttl > 0 {
		return errors.Errorf("Certificate is expired %d seconds ago", ttl)
	}

	if s.commonName != nil && *s.commonName != crt.Subject.CommonName {
		return errors.Errorf("CommonName mismatch: %s != %s", *s.commonName, crt.Subject.CommonName)
	}

	if s.dnsNames != nil {
		actualDomains := map[string]struct{}{}
		for _, actual := range crt.DNSNames {
			actualDomains[actual] = struct{}{}
		}

		for _, desired := range s.dnsNames {
			if _, ok := actualDomains[desired]; !ok {
				return errors.Errorf("DNS name %s is not found in certificate DNS names", desired)
			}
		}
	}

	if s.ipAddresses != nil {
		actualIPAddresses := map[string]struct{}{}
		for _, actual := range crt.IPAddresses {
			actualIPAddresses[actual.String()] = struct{}{}
		}

		for _, desired := range s.ipAddresses {
			if _, ok := actualIPAddresses[desired]; !ok {
				return errors.Errorf("IP address %s is not found in certificate IP addresses", desired)
			}
		}
	}

	if s.issuer != nil && *s.issuer != crt.Issuer.CommonName {
		return errors.Errorf("Issuer mismatch: %s != %s", *s.issuer, crt.Issuer.CommonName)
	}

	return nil
}
