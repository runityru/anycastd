package config

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	th "github.com/teran/go-time"
)

func TestConfig(t *testing.T) {
	type testCase struct {
		name       string
		samplePath string
		expOut     Config
		expError   error
	}

	sampleConfig := Config{
		Announcer: Announcer{
			Engine:       "gobgpv3",
			RouterID:     "10.3.3.3",
			LocalAddress: "10.0.0.1",
			LocalASN:     65999,
			Routes:       []string{"10.0.0.128/32"},
			Peers: []Peer{
				{
					Name:          "some_router_1",
					RemoteAddress: "10.0.0.252",
					RemoteASN:     65000,
				},
				{
					Name:           "some_router_2",
					RemoteAddress:  "10.0.0.253",
					RemoteASN:      65000,
					EnableMultihop: true,
					MultihopTTL:    2,
				},
			},
		},
		Services: []Service{
			{
				Name:          "http",
				CheckInterval: th.Duration(10 * time.Second),
				Checks: []Check{
					{
						Kind: "dns_lookup",
						Spec: json.RawMessage(`{"interval":"100ms","query":"example.com","resolver":"127.0.0.1","tries":3}`),
					},
					{
						Kind: "http_2xx",
						Spec: json.RawMessage(`{"address":"127.0.0.1:8080","interval":"100ms","path":"/","timeout":"2s","tries":3}`),
					},
					{
						Kind: "assigned_address",
						Spec: json.RawMessage(`{"interface":"dummy0","ipv4":"10.0.0.128"}`),
					},
				},
			},
		},
		Metrics: Metrics{
			Enabled: true,
			Address: "127.0.0.1:9090",
		},
	}

	tcs := []testCase{
		{
			name:       "YAML configuration",
			samplePath: "testdata/sample.yaml",
			expOut:     sampleConfig,
		},
		{
			name:       "JSON configuration",
			samplePath: "testdata/sample.json",
			expOut:     sampleConfig,
		},
		{
			name:       "engine missing fails validation",
			samplePath: "testdata/no_engine.yaml",
			expError:   errors.New("announcer: (engine: cannot be blank.)."),
		},
		{
			name:       "unknown engine fails validation",
			samplePath: "testdata/engine_invalid.yaml",
			expError:   errors.New("announcer: (engine: must be a valid value.)."),
		},
		{
			name:       "gobgpv4 engine is valid",
			samplePath: "testdata/engine_gobgpv4.yaml",
			expOut: Config{
				Announcer: Announcer{
					Engine:       "gobgpv4",
					RouterID:     "10.3.3.3",
					LocalAddress: "10.0.0.1",
					LocalASN:     65999,
					Routes:       []string{"10.0.0.128/32"},
					Peers:        []Peer{{Name: "some_router_1", RemoteAddress: "10.0.0.252", RemoteASN: 65000}},
				},
				Metrics: Metrics{Enabled: true, Address: "127.0.0.1:9090"},
			},
		},
		{
			name:       "wrong config file extension",
			samplePath: "testdata/sample.unknown",
			expError:   errors.New("unexpected file format: `.unknown`"),
		},
		{
			name:       "dual-stack",
			samplePath: "testdata/ipv6_route.yaml",
			expOut: Config{
				Announcer: Announcer{
					Engine:        "gobgpv3",
					RouterID:      "10.3.3.3",
					LocalAddress:  "10.0.0.1",
					LocalAddress6: "2001:db8::1",
					LocalASN:      65999,
					Routes:        []string{"10.0.0.128/32", "2001:db8:1::/48"},
					Peers: []Peer{
						{Name: "some_router_1", RemoteAddress: "10.0.0.252", RemoteASN: 65000},
						{Name: "some_router_2", RemoteAddress: "2001:db8::252", RemoteASN: 65000, EnableMultihop: true, MultihopTTL: 2},
					},
				},
				Metrics: Metrics{Enabled: true, Address: "address"},
			},
		},
		{
			name:       "IPv6-only host",
			samplePath: "testdata/ipv6_only.yaml",
			expOut: Config{
				Announcer: Announcer{
					Engine:   "gobgpv3",
					RouterID: "10.3.3.3",
					LocalASN: 65999,
					Routes:   []string{"2001:db8:1::/48"},
					Peers:    []Peer{{Name: "some_router_1", RemoteAddress: "2001:db8::252", RemoteASN: 65000}},
				},
				Metrics: Metrics{Enabled: true, Address: "address"},
			},
		},
		{
			name:       "IPv6 route without IPv6 peer, empty route",
			samplePath: "testdata/ipv6_route_without_ipv6_peer.yaml",
			expError:   errors.New("announcer: (routes: (1: requires an IPv6 peer; 2: must be a valid CIDR.).)."),
		},
		{
			name:       "IPv4 route without IPv4 peer",
			samplePath: "testdata/ipv4_route_without_ipv4_peer.yaml",
			expError:   errors.New("announcer: (routes: (0: requires an IPv4 peer.).)."),
		},
		{
			name:       "multihop peers without local addresses",
			samplePath: "testdata/multihop_without_local_address.yaml",
			expError:   errors.New("announcer: (peers: (0: enable_multihop requires local_address; 1: enable_multihop requires local_address6.).)."),
		},
		{
			name:       "IPv4-mapped addresses",
			samplePath: "testdata/ipv4_mapped_peer.yaml",
			expError:   errors.New("announcer: (local_address6: must not be an IPv4-mapped IPv6 address; peers: (0: (remote_address: must not be an IPv4-mapped IPv6 address.).).)."),
		},
		{
			name:       "link-local and zoned peer addresses",
			samplePath: "testdata/link_local.yaml",
			expError:   errors.New("announcer: (peers: (0: (remote_address: must not be a link-local address.); 1: (remote_address: must not have a zone.).).)."),
		},
		{
			name:       "unspecified local addresses count as unset, unspecified peer address",
			samplePath: "testdata/unspecified_addresses.yaml",
			expError:   errors.New("announcer: (peers: (0: enable_multihop requires local_address; 1: (remote_address: must not be an unspecified address.).).)."),
		},
		{
			name:       "IPv4 local_address6",
			samplePath: "testdata/local_address6_ipv4.yaml",
			expError:   errors.New("announcer: (local_address6: must be a valid IPv6 address.)."),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			cfg, err := NewFromFile(tc.samplePath)
			if tc.expError == nil {
				r.NoError(err)
				r.Equal(tc.expOut.Announcer, cfg.Announcer)
				for i := range tc.expOut.Services {
					r.Equalf(tc.expOut.Services[i].Name, cfg.Services[i].Name, "svc#%d", i)
					r.Equalf(tc.expOut.Services[i].CheckInterval, cfg.Services[i].CheckInterval, "svc#%d", i)
					for j := range tc.expOut.Services[i].Checks {
						r.Equalf(tc.expOut.Services[i].Checks[j].Kind, cfg.Services[i].Checks[j].Kind, "svc#%d check#%d", i, j)
						r.JSONEqf(string(tc.expOut.Services[i].Checks[j].Spec), string(cfg.Services[i].Checks[j].Spec), "svc#%d check#%d", i, j)
					}
				}
				r.Equal(tc.expOut.Metrics, cfg.Metrics)
			} else {
				r.Error(err)
				r.Equal(tc.expError.Error(), err.Error())
			}
		})
	}
}

func TestEmptyConfig(t *testing.T) {
	r := require.New(t)

	_, err := NewFromFile("testdata/empty.yaml")
	r.Error(err)
	r.Equal(
		"announcer: (engine: cannot be blank; local_asn: cannot be blank; peers: cannot be blank; router_id: cannot be blank; routes: cannot be blank.); metrics: (address: cannot be blank; enabled: cannot be blank.); services: cannot be blank.",
		err.Error(),
	)
}

func TestEmptyAnnouncerOnlyConfig(t *testing.T) {
	r := require.New(t)

	// Actually there are no semantics checks so it should pass
	_, err := NewFromFile("testdata/empty_with_announcer.yaml")
	r.NoError(err)
}

func TestBfdConfig(t *testing.T) {
	r := require.New(t)

	t.Run("enabled with gobgpv4 parses intervals as durations", func(t *testing.T) {
		cfg, err := NewFromFile("testdata/bfd_gobgpv4.yaml")
		r.NoError(err)

		bfd := cfg.Announcer.Bfd
		r.True(bfd.Enabled)
		r.Equal(uint16(3784), *bfd.Port)
		r.Equal(th.Duration(300*time.Millisecond), *bfd.MinTxInterval)
		r.Equal(th.Duration(300*time.Millisecond), *bfd.MinRxInterval)
		r.Equal(uint32(3), *bfd.Multiplier)
	})

	t.Run("enabled with gobgpv3 fails closed", func(t *testing.T) {
		_, err := NewFromFile("testdata/bfd_gobgpv3.yaml")
		r.Error(err)
		r.Contains(err.Error(), "requires engine gobgpv4")
	})
}

func TestBfdValidation(t *testing.T) {
	r := require.New(t)

	// Disabled BFD accepts any values (thresholds are only enforced when enabled).
	r.NoError((Bfd{Enabled: false, Port: uint16ptr(0), Multiplier: uint32ptr(0)}).Validate())

	// Enabled BFD rejects an out-of-range port.
	r.Error((Bfd{Enabled: true, Port: uint16ptr(0)}).Validate())
	r.NoError((Bfd{Enabled: true, Port: uint16ptr(1)}).Validate())
	r.NoError((Bfd{Enabled: true, Port: uint16ptr(65535)}).Validate())

	// Enabled BFD rejects intervals below the 50ms floor (anti self-DoS).
	us := th.Duration(500 * time.Microsecond)
	r.Error((Bfd{Enabled: true, MinTxInterval: &us}).Validate())
	r.Error((Bfd{Enabled: true, MinRxInterval: &us}).Validate())
	oneMs := th.Duration(time.Millisecond)
	r.Error((Bfd{Enabled: true, MinTxInterval: &oneMs}).Validate())
	r.Error((Bfd{Enabled: true, MinRxInterval: &oneMs}).Validate())
	subFloor := th.Duration(49 * time.Millisecond)
	r.Error((Bfd{Enabled: true, MinTxInterval: &subFloor}).Validate())
	r.Error((Bfd{Enabled: true, MinRxInterval: &subFloor}).Validate())
	floor := th.Duration(50 * time.Millisecond)
	r.NoError((Bfd{Enabled: true, MinTxInterval: &floor}).Validate())
	r.NoError((Bfd{Enabled: true, MinRxInterval: &floor}).Validate())

	// Enabled BFD rejects intervals above the uint32-µs protocol limit (~71.6 min).
	atMax := th.Duration(4294967295 * time.Microsecond)
	r.NoError((Bfd{Enabled: true, MinTxInterval: &atMax}).Validate())
	r.NoError((Bfd{Enabled: true, MinRxInterval: &atMax}).Validate())
	tooBig := th.Duration(72 * time.Minute)
	r.Error((Bfd{Enabled: true, MinTxInterval: &tooBig}).Validate())
	r.Error((Bfd{Enabled: true, MinRxInterval: &tooBig}).Validate())

	// Enabled BFD rejects a zero multiplier.
	r.Error((Bfd{Enabled: true, Multiplier: uint32ptr(0)}).Validate())
	r.NoError((Bfd{Enabled: true, Multiplier: uint32ptr(1)}).Validate())

	// Enabled BFD caps the detection multiplier at 255 (RFC 5880 8-bit field;
	// GoBGP silently truncates larger values).
	r.NoError((Bfd{Enabled: true, Multiplier: uint32ptr(255)}).Validate())
	r.Error((Bfd{Enabled: true, Multiplier: uint32ptr(256)}).Validate())

	// Omitted pointers are valid when enabled (engine applies its default).
	r.NoError((Bfd{Enabled: true}).Validate())
}

func uint16ptr(v uint16) *uint16 { return &v }

func uint32ptr(v uint32) *uint32 { return &v }

func TestServiceHysteresisValidation(t *testing.T) {
	r := require.New(t)

	base := func() Service {
		return Service{
			Name:          "svc",
			CheckInterval: th.Duration(time.Second),
			Checks:        []Check{{Kind: "http_2xx", Spec: json.RawMessage(`{}`)}},
		}
	}

	t.Run("unset thresholds default to valid 1/1", func(t *testing.T) {
		r.NoError(base().Validate())
	})
	t.Run("equal thresholds valid", func(t *testing.T) {
		s := base()
		s.AnnounceThreshold = 3
		s.DenounceThreshold = 3
		r.NoError(s.Validate())
	})
	t.Run("announce below denounce valid", func(t *testing.T) {
		s := base()
		s.AnnounceThreshold = 1
		s.DenounceThreshold = 5
		r.NoError(s.Validate())
	})
	t.Run("announce above denounce fails closed", func(t *testing.T) {
		s := base()
		s.AnnounceThreshold = 5
		s.DenounceThreshold = 2
		r.Error(s.Validate())
	})
	t.Run("unset denounce with announce>1 fails closed (default 1)", func(t *testing.T) {
		s := base()
		s.AnnounceThreshold = 3 // denounce unset -> default 1 -> 3 > 1
		r.Error(s.Validate())
	})
	t.Run("unset announce with denounce>1 is valid (default 1)", func(t *testing.T) {
		s := base()
		s.DenounceThreshold = 3 // announce unset -> default 1 -> 1 <= 3
		r.NoError(s.Validate())
	})
	t.Run("negative announce rejected", func(t *testing.T) {
		s := base()
		s.AnnounceThreshold = -1
		r.Error(s.Validate())
	})
	t.Run("negative denounce rejected", func(t *testing.T) {
		s := base()
		s.DenounceThreshold = -1
		r.Error(s.Validate())
	})
}
