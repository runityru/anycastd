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

type Remote struct {
	Address  string `json:"host"`
	Insecure bool   `json:"insecure"`
}

func (s Remote) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.Address, validation.Required, is.DialString),
	)
}

type spec struct {
	Local       *Local   `json:"local"`
	Remote      *Remote  `json:"remote"`
	CommonName  *string  `json:"common_name"`
	DNSNames    []string `json:"dns_names"`
	IPAddresses []string `json:"ip_addresses"`
	Issuer      *string  `json:"issuer"`
}

func (s spec) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.Local, validation.When(s.Remote == nil, validation.Required.Error("either remote or local configuration must be defined"))),
		validation.Field(&s.Remote, validation.When(s.Local == nil, validation.Required.Error("either remote or local configuration must be defined"))),
		// CommonName: no validation required
		validation.Field(&s.DNSNames, validation.Each(is.DNSName)),
		validation.Field(&s.IPAddresses, validation.Each(is.IP)),
		// Issuer: no validation required
	)
}
