package dns_lookup

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"github.com/teran/anycastd/config"
)

type spec struct {
	Query    string          `json:"query"`
	Resolver string          `json:"resolver"`
	Tries    uint8           `json:"tries"`
	Interval config.Duration `json:"interval"`
	Timeout  config.Duration `json:"timeout"`
}

func (s spec) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.Query, validation.Required, is.DNSName),
		validation.Field(&s.Resolver, validation.Required, is.DialString),
		validation.Field(&s.Tries, validation.Required),
		validation.Field(&s.Interval, validation.Required),
		validation.Field(&s.Timeout, validation.Required),
	)
}
