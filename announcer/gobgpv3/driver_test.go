package gobgpv3

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/runityru/anycastd/announcer"
)

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

			p := newPeer(tc.peer)

			r.Equal(tc.peer.RemoteAddress, p.Conf.NeighborAddress)
			r.Equal(uint32(65000), p.Conf.PeerAsn)
			r.True(p.EbgpMultihop.Enabled)
			r.Equal(uint32(2), p.EbgpMultihop.MultihopTtl)
			r.NotNil(p.Transport)
			r.Equal(tc.expLocalAddress, p.Transport.LocalAddress)
			r.Equal(uint64(5), p.Timers.Config.ConnectRetry)
		})
	}
}
