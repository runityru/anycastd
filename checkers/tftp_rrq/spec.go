package tftp_rrq

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"github.com/teran/anycastd/config"
)

type spec struct {
	URL            string          `json:"url"`
	ExpectedSHA256 *string         `json:"expected_sha256"`
	Tries          uint8           `json:"tries"`
	Interval       config.Duration `json:"interval"`
	Timeout        config.Duration `json:"timeout"`
}

func (s spec) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.URL, validation.Required, is.RequestURI),
		validation.Field(&s.ExpectedSHA256, validation.Length(64, 64), is.Hexadecimal),
		validation.Field(&s.Tries, validation.Required),
		validation.Field(&s.Interval, validation.Required),
		validation.Field(&s.Timeout, validation.Required),
	)
}
