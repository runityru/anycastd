package http_2xx

import (
	"github.com/teran/anycastd/config"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

type spec struct {
	Address  string          `json:"address"`
	Path     string          `json:"path"`
	Method   string          `json:"method"`
	Tries    uint8           `json:"tries"`
	Interval config.Duration `json:"interval"`
	Timeout  config.Duration `json:"timeout"`
}

func (s *spec) Validate() error {
	return validation.ValidateStruct(s,
		validation.Field(&s.Address, validation.Required),
		validation.Field(&s.Path, validation.Required),
		validation.Field(&s.Method, validation.Required),
		validation.Field(&s.Tries, validation.Required),
		validation.Field(&s.Interval, validation.Required),
		validation.Field(&s.Timeout, validation.Required),
	)
}
