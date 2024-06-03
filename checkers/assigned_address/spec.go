package assigned_address

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
)

type spec struct {
	Interface *string `json:"interface"`
	IPv4      string  `json:"ipv4"`
}

func (s *spec) Validate() error {
	return validation.ValidateStruct(s,
		validation.Field(&s.IPv4, validation.Required, is.IPv4),
	)
}
