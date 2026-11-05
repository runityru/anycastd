package ntpq

import (
	"net/netip"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/pkg/errors"
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
		validation.Field(&s.Server, validation.Required, is.Host, validation.By(checkServer)),
		validation.Field(&s.SrcAddr, validation.Required, is.IP, validation.By(s.checkSrcAddr)),
		validation.Field(&s.Tries, validation.Required),
		validation.Field(&s.OffsetThreshold, validation.Required),
		validation.Field(&s.Interval, validation.Required),
		validation.Field(&s.Timeout, validation.Required),
	)
}

// checkServer rejects IP literals that cannot be a remote reference.
func checkServer(v any) error {
	srv, err := netip.ParseAddr(v.(string))
	if err != nil {
		return nil
	}
	srv = srv.Unmap()
	if srv.IsUnspecified() || srv.Is6() && srv.IsLinkLocalUnicast() {
		return errors.New("must not be an unspecified or IPv6 link-local address")
	}
	return nil
}

// checkSrcAddr compares families only for an IP literal server; dial filters a hostname's addresses.
func (s spec) checkSrcAddr(any) error {
	src, _ := netip.ParseAddr(s.SrcAddr)
	src = src.Unmap()
	if src.Is6() && src.IsLinkLocalUnicast() {
		return errors.New("must not be an IPv6 link-local address")
	}
	srv, err := netip.ParseAddr(s.Server)
	if err != nil {
		return nil
	}
	if src.Is4() != srv.Unmap().Is4() {
		return errors.New("must match the address family of server")
	}
	return nil
}
