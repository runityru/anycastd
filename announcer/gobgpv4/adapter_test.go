package gobgpv4

import (
	"context"
	"net"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v4/api"
	"github.com/osrg/gobgp/v4/pkg/apiutil"
	"github.com/osrg/gobgp/v4/pkg/packet/bgp"
	"github.com/osrg/gobgp/v4/pkg/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddDeletePath(t *testing.T) {
	tcs := []struct {
		name         string
		prefix       string
		family       bgp.Family
		expNextHop   string
		loopback     string
		localAddress string
		peerASN      uint32
	}{
		{name: "IPv4 eBGP", prefix: "10.0.0.128/32", family: bgp.RF_IPv4_UC, expNextHop: "0.0.0.0", loopback: "127.0.0.1", localAddress: "127.0.0.1", peerASN: 65000},
		{name: "IPv6 eBGP", prefix: "2001:db8:1::/48", family: bgp.RF_IPv6_UC, expNextHop: "::", loopback: "::1", localAddress: "::1", peerASN: 65000},
		{name: "IPv4 eBGP without local address", prefix: "10.0.0.128/32", family: bgp.RF_IPv4_UC, expNextHop: "0.0.0.0", loopback: "127.0.0.1", peerASN: 65000},
		{name: "IPv6 eBGP without local address", prefix: "2001:db8:1::/48", family: bgp.RF_IPv6_UC, expNextHop: "::", loopback: "::1", peerASN: 65000},
		{name: "IPv4 iBGP", prefix: "10.0.0.128/32", family: bgp.RF_IPv4_UC, expNextHop: "0.0.0.0", loopback: "127.0.0.1", localAddress: "127.0.0.1", peerASN: 65999},
		{name: "IPv6 iBGP", prefix: "2001:db8:1::/48", family: bgp.RF_IPv6_UC, expNextHop: "::", loopback: "::1", localAddress: "::1", peerASN: 65999},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			ctx := context.Background()

			srv := server.NewBgpServer()
			go srv.Serve()
			defer srv.Stop()
			r.NoError(srv.StartBgp(ctx, &api.StartBgpRequest{
				Global: &api.Global{RouterId: "10.3.3.3", Asn: 65999, ListenPort: -1},
			}))

			// Peer B listens on a free loopback port; srv connects to it.
			l, err := net.Listen("tcp", net.JoinHostPort(tc.loopback, "0"))
			if err != nil {
				t.Skipf("no %s loopback: %s", tc.loopback, err)
			}
			port := l.Addr().(*net.TCPAddr).Port
			r.NoError(l.Close())

			peer := server.NewBgpServer()
			go peer.Serve()
			defer peer.Stop()
			r.NoError(peer.StartBgp(ctx, &api.StartBgpRequest{
				Global: &api.Global{RouterId: "10.4.4.4", Asn: tc.peerASN, ListenPort: int32(port), ListenAddresses: []string{tc.loopback}},
			}))
			r.NoError(peer.AddPeer(ctx, &api.AddPeerRequest{Peer: &api.Peer{
				Conf:      &api.PeerConf{NeighborAddress: tc.loopback, PeerAsn: 65999},
				Transport: &api.Transport{PassiveMode: true},
			}}))
			r.NoError(srv.AddPeer(ctx, &api.AddPeerRequest{Peer: &api.Peer{
				Conf:      &api.PeerConf{NeighborAddress: tc.loopback, PeerAsn: tc.peerASN},
				Transport: &api.Transport{LocalAddress: tc.localAddress, RemotePort: uint32(port)},
				Timers:    &api.Timers{Config: &api.TimersConfig{ConnectRetry: 1}},
			}}))

			nextHops := func(t assert.TestingT, s *server.BgpServer) []string {
				var out []string
				assert.NoError(t, s.ListPath(apiutil.ListPathRequest{TableType: api.TableType_TABLE_TYPE_GLOBAL, Family: tc.family}, func(_ bgp.NLRI, paths []*apiutil.Path) {
					for _, p := range paths {
						for _, a := range p.Attrs {
							switch attr := a.(type) {
							case *bgp.PathAttributeNextHop:
								out = append(out, attr.Value.String())
							case *bgp.PathAttributeMpReachNLRI:
								out = append(out, attr.Nexthop.String())
							}
						}
					}
				}))
				return out
			}

			a := NewAdapter(srv)
			r.NoError(a.AddPath(ctx, tc.prefix))
			r.Equal([]string{tc.expNextHop}, nextHops(t, srv))
			r.EventuallyWithT(func(c *assert.CollectT) {
				assert.Equal(c, []string{tc.loopback}, nextHops(c, peer))
			}, 10*time.Second, 100*time.Millisecond, "peer did not receive "+tc.prefix)

			r.NoError(a.DeletePath(ctx, tc.prefix))
			r.Empty(nextHops(t, srv))
			r.EventuallyWithT(func(c *assert.CollectT) {
				assert.Empty(c, nextHops(c, peer))
			}, 10*time.Second, 100*time.Millisecond, "peer kept "+tc.prefix)
		})
	}
}
