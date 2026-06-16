package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/runityru/anycastd/config"
)

func TestNewPeer(t *testing.T) {
	r := require.New(t)

	p := newPeer(config.Peer{
		Name:           "test-peer",
		RemoteAddress:  "10.0.0.1",
		RemoteASN:      65000,
		EnableMultihop: true,
		MultihopTTL:    2,
	}, "10.0.0.2")

	r.Equal("10.0.0.1", p.Conf.NeighborAddress)
	r.Equal(uint32(65000), p.Conf.PeerAsn)
	r.True(p.EbgpMultihop.Enabled)
	r.Equal(uint32(2), p.EbgpMultihop.MultihopTtl)
	r.NotNil(p.Transport)
	r.Equal("10.0.0.2", p.Transport.LocalAddress)
}
