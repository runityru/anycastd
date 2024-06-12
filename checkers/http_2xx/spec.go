package http_2xx

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	th "github.com/teran/go-time"
)

type spec struct {
	URL      string      `json:"url"`
	Method   string      `json:"method"`
	Tries    uint8       `json:"tries"`
	Interval th.Duration `json:"interval"`
	Timeout  th.Duration `json:"timeout"`
}

func (s spec) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.URL, validation.Required, is.URL),
		validation.Field(&s.Method, validation.Required),
		validation.Field(&s.Tries, validation.Required),
		validation.Field(&s.Interval, validation.Required),
		validation.Field(&s.Timeout, validation.Required),
	)
}
