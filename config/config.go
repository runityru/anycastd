package config

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/pkg/errors"
	th "github.com/teran/go-time"
	yaml "gopkg.in/yaml.v3"
)

// GoBGP mishandles these session addresses: link-local needs a zone, a zone
// yields a `::` next hop, and an IPv4-mapped IPv6 address counts as IPv4.
var isSessionAddress = validation.By(func(v any) error {
	s := v.(string)
	if s == "" {
		return nil
	}
	addr, err := netip.ParseAddr(s)
	switch {
	case err != nil:
		return errors.New("must be a valid IP address")
	case addr.Is6() && addr.IsLinkLocalUnicast():
		return errors.New("must not be a link-local address")
	case addr.Zone() != "":
		return errors.New("must not have a zone")
	case addr.Is4In6():
		return errors.New("must not be an IPv4-mapped IPv6 address")
	}
	return nil
})

// Linux connects an unspecified remote address to localhost.
var notUnspecified = validation.By(func(v any) error {
	addr, err := netip.ParseAddr(v.(string))
	if err == nil && addr.IsUnspecified() {
		return errors.New("must not be an unspecified address")
	}
	return nil
})

var (
	_ validation.Validatable = (*Config)(nil)
	_ validation.Validatable = (*Announcer)(nil)
	_ validation.Validatable = (*Service)(nil)
	_ validation.Validatable = (*Metrics)(nil)
	_ validation.Validatable = (*Check)(nil)
	_ validation.Validatable = (*Peer)(nil)
)

type Announcer struct {
	RouterID      string   `json:"router_id"`
	LocalAddress  string   `json:"local_address"`
	LocalAddress6 string   `json:"local_address6"`
	LocalASN      uint32   `json:"local_asn"`
	Routes        []string `json:"routes"`
	Peers         []Peer   `json:"peers"`
}

func (a Announcer) Validate() error {
	// Each address family is announced only over sessions of the same family.
	hasPeer6 := slices.ContainsFunc(a.Peers, Peer.isIPv6)
	hasPeer4 := slices.ContainsFunc(a.Peers, func(p Peer) bool { return !p.isIPv6() })

	routeHasPeer := validation.By(func(v any) error {
		p, err := netip.ParsePrefix(v.(string))
		switch {
		case err != nil:
			return errors.New("must be a valid CIDR")
		case p.Addr().Is4() && !hasPeer4:
			return errors.New("requires an IPv4 peer")
		case !p.Addr().Is4() && !hasPeer6:
			return errors.New("requires an IPv6 peer")
		}
		return nil
	})

	// ozzo runs Peer.Validate only after this rule passes, so invalid peers are left to it.
	multihopHasLocalAddress := validation.By(func(v any) error {
		p := v.(Peer)
		switch {
		case !p.EnableMultihop || p.Validate() != nil || a.LocalAddressFor(p) != "":
			return nil
		case p.isIPv6():
			return errors.New("enable_multihop requires local_address6")
		}
		return errors.New("enable_multihop requires local_address")
	})

	return validation.ValidateStruct(&a,
		validation.Field(&a.RouterID, validation.Required, is.IPv4),
		validation.Field(&a.LocalAddress, isSessionAddress, is.IPv4),
		validation.Field(&a.LocalAddress6, isSessionAddress, is.IPv6),
		validation.Field(&a.LocalASN, validation.Required),
		validation.Field(&a.Routes, validation.Required, validation.Each(routeHasPeer)),
		validation.Field(&a.Peers, validation.Required, validation.Each(multihopHasLocalAddress)),
	)
}

// LocalAddressFor returns the local address of the peer's address family, or ""
// when it is unset or unspecified: GoBGP takes any other spelling of `::`, such
// as `::0`, as set and sends `::` as the next hop.
func (a Announcer) LocalAddressFor(p Peer) string {
	s := a.LocalAddress
	if p.isIPv6() {
		s = a.LocalAddress6
	}
	if addr, err := netip.ParseAddr(s); err == nil && addr.IsUnspecified() {
		return ""
	}
	return s
}

type Check struct {
	Kind  string          `json:"kind"`
	Spec  json.RawMessage `json:"spec"`
	Group string          `json:"group"`
}

func (c Check) Validate() error {
	return validation.ValidateStruct(&c,
		validation.Field(&c.Kind, validation.Required),
		validation.Field(&c.Spec, validation.Required),
	)
}

type Peer struct {
	Name           string `json:"name"`
	RemoteAddress  string `json:"remote_address"`
	RemoteASN      uint32 `json:"remote_asn"`
	EnableMultihop bool   `json:"enable_multihop"`
	MultihopTTL    uint32 `json:"multihop_ttl"`
}

func (p Peer) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Name, validation.Required),
		validation.Field(&p.RemoteAddress, validation.Required, isSessionAddress, notUnspecified),
		validation.Field(&p.RemoteASN, validation.Required),
		validation.Field(&p.MultihopTTL, validation.When(p.EnableMultihop, validation.Min(uint32(1)))),
	)
}

func (p Peer) isIPv6() bool {
	addr, err := netip.ParseAddr(p.RemoteAddress)
	return err == nil && addr.Is6()
}

type Service struct {
	Name            string          `json:"name"`
	CheckInterval   th.Duration     `json:"check_interval"`
	Strategy        string          `json:"strategy"`
	StrategyOptions json.RawMessage `json:"strategy_options"`
	Checks          []Check         `json:"checks"`
}

func (s Service) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.Name, validation.Required),
		validation.Field(&s.CheckInterval, validation.Required),
		validation.Field(&s.Checks, validation.Required),
	)
}

type Metrics struct {
	Enabled bool   `json:"enabled"`
	Address string `json:"address"`
}

func (m Metrics) Validate() error {
	return validation.ValidateStruct(&m,
		validation.Field(&m.Enabled, validation.Required),
		validation.Field(&m.Address, validation.Required),
	)
}

type Config struct {
	Announcer Announcer `json:"announcer"`
	Services  []Service `json:"services"`
	Metrics   Metrics   `json:"metrics"`
}

func (c *Config) Validate() error {
	return validation.ValidateStruct(c,
		validation.Field(&c.Announcer),
		validation.Field(&c.Services, validation.Required),
		validation.Field(&c.Metrics),
	)
}

func NewFromFile(filename string) (*Config, error) {
	cfg := &Config{}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrap(err, "error reading configuration file")
	}

	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".yml", ".yaml":
		type intermediate struct {
			Announcer map[string]any `yaml:"announcer"`
			Services  []any          `yaml:"services"`
			Metrics   map[string]any `yaml:"metrics"`
		}

		d := intermediate{}

		err := yaml.Unmarshal(data, &d)
		if err != nil {
			return nil, errors.Wrap(err, "error unmarshaling intermediate configuration")
		}

		data, err = json.Marshal(d)
		if err != nil {
			return nil, errors.Wrap(err, "error marshaling intermediate configuration")
		}
	case ".json":
		// no additional action needed
	default:
		return nil, errors.Errorf("unexpected file format: `%s`", ext)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Wrap(err, "error unmarshaling config")
	}

	return cfg, cfg.Validate()
}
