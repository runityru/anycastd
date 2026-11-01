package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/runityru/anycastd/config"
)

func TestNewPeer(t *testing.T) {
	tcs := []struct {
		name            string
		remoteAddress   string
		localAddress    string
		localAddress6   string
		expLocalAddress string
	}{
		{name: "IPv4 peer", remoteAddress: "10.0.0.1", localAddress: "10.0.0.2", localAddress6: "2001:db8::2", expLocalAddress: "10.0.0.2"},
		{name: "IPv6 peer", remoteAddress: "2001:db8::1", localAddress: "10.0.0.2", localAddress6: "2001:db8::2", expLocalAddress: "2001:db8::2"},
		{name: "IPv4 peer without local_address", remoteAddress: "10.0.0.1", localAddress6: "2001:db8::2"},
		{name: "IPv6 peer without local_address6", remoteAddress: "2001:db8::1", localAddress: "10.0.0.2"},
		{name: "IPv4 peer with unspecified local_address", remoteAddress: "10.0.0.1", localAddress: "0.0.0.0", localAddress6: "2001:db8::2"},
		{name: "IPv6 peer with unspecified local_address6", remoteAddress: "2001:db8::1", localAddress: "10.0.0.2", localAddress6: "::0"},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			p := newPeer(config.Peer{
				Name:           "test-peer",
				RemoteAddress:  tc.remoteAddress,
				RemoteASN:      65000,
				EnableMultihop: true,
				MultihopTTL:    2,
			}, config.Announcer{LocalAddress: tc.localAddress, LocalAddress6: tc.localAddress6})

			r.Equal(tc.remoteAddress, p.Conf.NeighborAddress)
			r.Equal(uint32(65000), p.Conf.PeerAsn)
			r.True(p.EbgpMultihop.Enabled)
			r.Equal(uint32(2), p.EbgpMultihop.MultihopTtl)
			r.NotNil(p.Transport)
			r.Equal(tc.expLocalAddress, p.Transport.LocalAddress)
			r.Equal(uint64(5), p.Timers.Config.ConnectRetry)
		})
	}
}
