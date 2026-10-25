package gobgpv3

import (
	"context"
	"net"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddDeletePath(t *testing.T) {
	tcs := []struct {
		name         string
		prefix       string
		afi          api.Family_Afi
		expNextHop   string
		loopback     string
		localAddress string
		peerASN      uint32
	}{
		{name: "IPv4 eBGP", prefix: "10.0.0.128/32", afi: api.Family_AFI_IP, expNextHop: "0.0.0.0", loopback: "127.0.0.1", localAddress: "127.0.0.1", peerASN: 65000},
		{name: "IPv6 eBGP", prefix: "2001:db8:1::/48", afi: api.Family_AFI_IP6, expNextHop: "::", loopback: "::1", localAddress: "::1", peerASN: 65000},
		{name: "IPv4 eBGP without local address", prefix: "10.0.0.128/32", afi: api.Family_AFI_IP, expNextHop: "0.0.0.0", loopback: "127.0.0.1", peerASN: 65000},
		{name: "IPv6 eBGP without local address", prefix: "2001:db8:1::/48", afi: api.Family_AFI_IP6, expNextHop: "::", loopback: "::1", peerASN: 65000},
		{name: "IPv4 iBGP", prefix: "10.0.0.128/32", afi: api.Family_AFI_IP, expNextHop: "0.0.0.0", loopback: "127.0.0.1", localAddress: "127.0.0.1", peerASN: 65999},
		{name: "IPv6 iBGP", prefix: "2001:db8:1::/48", afi: api.Family_AFI_IP6, expNextHop: "::", loopback: "::1", localAddress: "::1", peerASN: 65999},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			ctx := context.Background()
			family := &api.Family{Afi: tc.afi, Safi: api.Family_SAFI_UNICAST}

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
				assert.NoError(t, s.ListPath(ctx, &api.ListPathRequest{Family: family}, func(d *api.Destination) {
					for _, a := range d.Paths[0].Pattrs {
						var nh api.NextHopAttribute
						var mp api.MpReachNLRIAttribute
						switch {
						case a.UnmarshalTo(&nh) == nil:
							out = append(out, nh.NextHop)
						case a.UnmarshalTo(&mp) == nil:
							out = append(out, mp.NextHops[0])
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
