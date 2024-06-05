package tls_certificate

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
)

type Local struct {
	Path string `json:"path"`
}

func (s Local) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.Path, validation.Required),
	)
}

type spec struct {
	Local       Local    `json:"local"`
	CommonName  *string  `json:"common_name"`
	DNSNames    []string `json:"dns_names"`
	IPAddresses []string `json:"ip_addresses"`
	Issuer      *string  `json:"issuer"`
}

func (s spec) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.Local, validation.Required),
		// CommonName: no validation required
		validation.Field(&s.DNSNames, validation.Each(is.DNSName)),
		validation.Field(&s.IPAddresses, validation.Each(is.IP)),
		// Issuer: no validation required
	)
}
