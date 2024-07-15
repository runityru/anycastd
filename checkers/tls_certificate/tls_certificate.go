package tls_certificate

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"github.com/runityru/anycastd/checkers"
)

var (
	_ checkers.Checker = (*tls_certificate)(nil)

	certificateExpiresInSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "anycastd",
			Name:      "certificate_expires_in_seconds",
			Help:      "Time the certificate expires in (in seconds)",
		},
		[]string{"check", "path"},
	)
)

const checkName = "tls_certificate"

func init() {
	checkers.Register(checkName, NewFromSpec)
	prometheus.MustRegister(certificateExpiresInSeconds)
}

type tls_certificate struct {
	path string

	retrieveCertificates func() ([]*x509.Certificate, error)

	commonName  *string
	dnsNames    []string
	ipAddresses []string
	issuer      *string
}

func New(s spec) (checkers.Checker, error) {
	var fn func() ([]*x509.Certificate, error)
	if s.Local != nil {
		fn = getLocalCertificate(*s.Local)
	} else if s.Remote != nil {
		fn = getRemoteCertificate(*s.Remote)
	} else {
		return nil, errors.New("either local or remote configuration must be defined")
	}

	return newWithCertificateRetriever(s, fn)
}

func newWithCertificateRetriever(s spec, fn func() ([]*x509.Certificate, error)) (checkers.Checker, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	tc := &tls_certificate{
		retrieveCertificates: fn,

		commonName:  s.CommonName,
		dnsNames:    s.DNSNames,
		ipAddresses: s.IPAddresses,
		issuer:      s.Issuer,
	}

	if s.Local != nil {
		tc.path = s.Local.Path
	}

	return tc, nil
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
	log.WithFields(log.Fields{
		"check": checkName,
	}).Tracef("running check")

	certs, err := s.retrieveCertificates()
	if err != nil {
		return errors.Wrap(err, "error retrieving certificates")
	}

	if len(certs) < 1 {
		return errors.New("empty certificate list received")
	}
	crt := certs[len(certs)-1]

	ttl := int(time.Since(crt.NotAfter).Seconds())

	log.WithFields(log.Fields{
		"check":        checkName,
		"path":         s.path,
		"ttl":          ttl,
		"common_name":  crt.Subject.CommonName,
		"issuer":       crt.Issuer.CommonName,
		"dns_names":    crt.DNSNames,
		"ip_addresses": crt.IPAddresses,
	}).Tracef("certificate parsed")

	certificateExpiresInSeconds.WithLabelValues(checkName, s.path).Set(float64(ttl))

	if ttl > 0 {
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
