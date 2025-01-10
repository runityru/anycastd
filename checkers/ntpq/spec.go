package ntpq

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	th "github.com/teran/go-time"
)

type spec struct {
	Server          string      `json:"server"`
	SrcAddr         string      `json:"src_addr"`
	Tries           uint8       `json:"tries"`
	OffsetThreshold th.Duration `json:"offset_threshold"`
	Interval        th.Duration `json:"interval"`
	Timeout         th.Duration `json:"timeout"`
}

func (s spec) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.Server, validation.Required, is.Host),
		validation.Field(&s.SrcAddr, validation.Required, is.IPv4),
		validation.Field(&s.Tries, validation.Required),
		validation.Field(&s.OffsetThreshold, validation.Required),
		validation.Field(&s.Interval, validation.Required),
		validation.Field(&s.Timeout, validation.Required),
	)
}
