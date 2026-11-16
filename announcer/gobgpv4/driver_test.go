package gobgpv4

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/osrg/gobgp/v4/api"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	th "github.com/teran/go-time"

	"github.com/runityru/anycastd/announcer"
)

// captureHook records every logrus entry a bridge forwards.
type captureHook struct {
	mu   sync.Mutex
	msgs []string
}

func (h *captureHook) Levels() []log.Level { return log.AllLevels }
func (h *captureHook) Fire(e *log.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = append(h.msgs, e.Message)
	return nil
}

func TestSlogBridge(t *testing.T) {
	r := require.New(t)

	h := &captureHook{}
	logger := log.New()
	logger.SetOutput(io.Discard)
	logger.AddHook(h)

	sl := slog.New(&slogBridge{logger: logger})
	sl.Info("hello from slog")
	sl.Warn("warning from slog")

	h.mu.Lock()
	defer h.mu.Unlock()
	r.Contains(h.msgs, "hello from slog")
	r.Contains(h.msgs, "warning from slog")
}

func TestNewDriver(t *testing.T) {
	r := require.New(t)

	// The test logger must write to a real writer, not a nil handler.
	logger := log.New()
	logger.SetOutput(io.Discard)
	d, err := NewDriver(announcer.DriverSpec{}, logger)
	r.NoError(err)
	r.NotNil(d)
}

func TestNewPeer(t *testing.T) {
	tcs := []struct {
		name            string
		peer            announcer.PeerSpec
		expLocalAddress string
	}{
		{
			name: "IPv4 peer",
			peer: announcer.PeerSpec{
				Name: "test-peer", RemoteAddress: "10.0.0.1", RemoteASN: 65000,
				EnableMultihop: true, MultihopTTL: 2, LocalAddress: "10.0.0.2",
			},
			expLocalAddress: "10.0.0.2",
		},
		{
			name: "IPv6 peer",
			peer: announcer.PeerSpec{
				Name: "test-peer", RemoteAddress: "2001:db8::1", RemoteASN: 65000,
				EnableMultihop: true, MultihopTTL: 2, LocalAddress: "2001:db8::2",
			},
			expLocalAddress: "2001:db8::2",
		},
		{
			name: "peer without local_address",
			peer: announcer.PeerSpec{
				Name: "test-peer", RemoteAddress: "10.0.0.1", RemoteASN: 65000,
				EnableMultihop: true, MultihopTTL: 2,
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			p := newPeer(tc.peer, announcer.BfdSpec{})

			r.Equal(tc.peer.RemoteAddress, p.Conf.NeighborAddress)
			r.Equal(uint32(65000), p.Conf.PeerAsn)
			r.True(p.EbgpMultihop.Enabled)
			r.Equal(uint32(2), p.EbgpMultihop.MultihopTtl)
			r.NotNil(p.Transport)
			r.Equal(tc.expLocalAddress, p.Transport.LocalAddress)
			r.Equal(uint64(5), p.Timers.Config.ConnectRetry)
			r.Nil(p.Bfd)
		})
	}
}

func TestNewPeerBFD(t *testing.T) {
	oneSecond := th.Duration(time.Second)
	halfSecond := th.Duration(500 * time.Millisecond)
	port := uint16(4784)
	multiplier := uint32(5)

	tcs := []struct {
		name string
		peer announcer.PeerSpec
		bfd  announcer.BfdSpec
		exp  *api.BfdPeerConfig
	}{
		{
			name: "BFD disabled -> nil config",
			peer: announcer.PeerSpec{RemoteAddress: "10.0.0.1", RemoteASN: 65000, EnableMultihop: true},
			bfd:  announcer.BfdSpec{},
			exp:  nil,
		},
		{
			name: "BFD enabled with defaults (nil fields -> 0)",
			peer: announcer.PeerSpec{RemoteAddress: "10.0.0.1", RemoteASN: 65000, EnableMultihop: false},
			bfd:  announcer.BfdSpec{Enabled: true},
			exp: &api.BfdPeerConfig{
				Enabled:  true,
				Multihop: false,
			},
		},
		{
			name: "BFD enabled with all fields set",
			peer: announcer.PeerSpec{RemoteAddress: "10.0.0.1", RemoteASN: 65000, EnableMultihop: true, MultihopTTL: 2},
			bfd: announcer.BfdSpec{
				Enabled:              true,
				Port:                 &port,
				DesiredMinTxInterval: &oneSecond,
				RequiredMinReceive:   &halfSecond,
				DetectionMultiplier:  &multiplier,
			},
			exp: &api.BfdPeerConfig{
				Enabled:                  true,
				Port:                     uint32(port),
				DesiredMinimumTxInterval: uint32(time.Second.Microseconds()),
				RequiredMinimumReceive:   uint32((500 * time.Millisecond).Microseconds()),
				DetectionMultiplier:      multiplier,
				Multihop:                 true,
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			p := newPeer(tc.peer, tc.bfd)

			if tc.exp == nil {
				r.Nil(p.Bfd)
				return
			}
			r.NotNil(p.Bfd)
			r.Equal(tc.exp, p.Bfd)
			// Multihop in the BFD config must track the peer's multihop state.
			r.Equal(p.EbgpMultihop.Enabled, p.Bfd.Multihop)
		})
	}
}

func TestCapabilities(t *testing.T) {
	r := require.New(t)

	logger := log.New()
	logger.SetOutput(io.Discard)
	d, err := NewDriver(announcer.DriverSpec{}, logger)
	r.NoError(err)

	cp, ok := d.(announcer.CapabilityProvider)
	r.True(ok)
	r.True(cp.Capabilities().SupportsBFD)
}
