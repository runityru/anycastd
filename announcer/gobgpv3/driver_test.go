package gobgpv3

import (
	"io"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/runityru/anycastd/announcer"
)

func TestNewPeer(t *testing.T) {
	tcs := []struct {
		name            string
		peer            announcer.PeerSpec
		expLocalAddress string
		expRemotePort   uint32
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
		{
			name: "non-zero remote_port",
			peer: announcer.PeerSpec{
				Name: "test-peer", RemoteAddress: "10.0.0.1", RemoteASN: 65000,
				EnableMultihop: true, MultihopTTL: 2,
				LocalAddress: "10.0.0.2", RemotePort: 45678,
			},
			expLocalAddress: "10.0.0.2",
			expRemotePort:   45678,
		},
		{
			name: "zero remote_port leaves default 179",
			peer: announcer.PeerSpec{
				Name: "test-peer", RemoteAddress: "10.0.0.1", RemoteASN: 65000,
				EnableMultihop: true, MultihopTTL: 2,
				LocalAddress: "10.0.0.2", RemotePort: 0,
			},
			expLocalAddress: "10.0.0.2",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			p := newPeer(tc.peer)

			r.Equal(tc.peer.RemoteAddress, p.Conf.NeighborAddress)
			r.Equal(uint32(65000), p.Conf.PeerAsn)
			r.True(p.EbgpMultihop.Enabled)
			r.Equal(uint32(2), p.EbgpMultihop.MultihopTtl)
			r.NotNil(p.Transport)
			r.Equal(tc.expLocalAddress, p.Transport.LocalAddress)
			r.Equal(tc.expRemotePort, p.Transport.RemotePort)
			r.Equal(uint64(5), p.Timers.Config.ConnectRetry)
		})
	}
}

func TestNewDriverBFDFailClosed(t *testing.T) {
	r := require.New(t)

	logger := log.New()
	logger.SetOutput(io.Discard)

	// BFD enabled on the gobgpv3 engine must fail closed.
	_, err := NewDriver(announcer.DriverSpec{
		Bfd: announcer.BfdSpec{Enabled: true},
	}, logger)
	r.Error(err)
	r.Contains(err.Error(), "does not support BFD")

	// BFD disabled must succeed.
	d, err := NewDriver(announcer.DriverSpec{}, logger)
	r.NoError(err)
	r.NotNil(d)
}

func TestCapabilities(t *testing.T) {
	r := require.New(t)

	logger := log.New()
	logger.SetOutput(io.Discard)
	d, err := NewDriver(announcer.DriverSpec{}, logger)
	r.NoError(err)

	cp, ok := d.(announcer.CapabilityProvider)
	r.True(ok)
	r.False(cp.Capabilities().SupportsBFD)
}
