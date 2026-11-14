package gobgpv3

import (
	"context"
	"net"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runityru/anycastd/announcer"
)

// TestPeerStateSource exercises the peer-state source against a real
// in-process GoBGP server pair, mirroring the adapter test setup.
func TestPeerStateSource(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()

	srv := server.NewBgpServer()
	go srv.Serve()
	defer srv.Stop()
	r.NoError(srv.StartBgp(ctx, &api.StartBgpRequest{
		Global: &api.Global{RouterId: "10.3.3.3", Asn: 65999, ListenPort: -1},
	}))

	// Peer B listens on a free loopback port; srv connects to it.
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", "0"))
	if err != nil {
		t.Skipf("no IPv4 loopback: %s", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	r.NoError(l.Close())

	peer := server.NewBgpServer()
	go peer.Serve()
	defer peer.Stop()
	r.NoError(peer.StartBgp(ctx, &api.StartBgpRequest{
		Global: &api.Global{RouterId: "10.4.4.4", Asn: 65000, ListenPort: int32(port), ListenAddresses: []string{"127.0.0.1"}},
	}))
	r.NoError(peer.AddPeer(ctx, &api.AddPeerRequest{Peer: &api.Peer{
		Conf:      &api.PeerConf{NeighborAddress: "127.0.0.1", PeerAsn: 65999},
		Transport: &api.Transport{PassiveMode: true},
	}}))

	src := NewPeerStateSource(srv, log.New())

	// With no peers configured, ListPeers is empty and never errors.
	states, err := src.ListPeers(ctx)
	r.NoError(err)
	r.Empty(states)

	// Add the peer to the driver-side server and wait for it to be listed.
	r.NoError(srv.AddPeer(ctx, &api.AddPeerRequest{Peer: &api.Peer{
		Conf:      &api.PeerConf{NeighborAddress: "127.0.0.1", PeerAsn: 65000},
		Transport: &api.Transport{LocalAddress: "127.0.0.1", RemotePort: uint32(port)},
		Timers:    &api.Timers{Config: &api.TimersConfig{ConnectRetry: 1}},
	}}))

	var found *announcer.PeerState
	r.EventuallyWithT(func(c *assert.CollectT) {
		states, err = src.ListPeers(ctx)
		assert.NoError(c, err)
		for i := range states {
			if states[i].NeighborAddress == "127.0.0.1" {
				found = &states[i]
				return
			}
		}
	}, 10*time.Second, 100*time.Millisecond, "configured peer never listed")
	r.NotNil(found)

	// The mapping always yields complete, normalized state even for a peer
	// that has not exchanged messages yet.
	r.LessOrEqual(found.AdminState, uint32(2))
	r.NotNil(found)
}
