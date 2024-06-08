package icmp_ping

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"github.com/teran/anycastd/config"
)

type spec struct {
	Host     string          `json:"host"`
	Tries    uint8           `json:"tries"`
	Interval config.Duration `json:"interval"`
	Timeout  config.Duration `json:"timeout"`
}

func (s spec) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.Host, validation.Required, is.Host),
		validation.Field(&s.Tries, validation.Required),
		validation.Field(&s.Interval, validation.Required),
		validation.Field(&s.Timeout, validation.Required),
	)
}
