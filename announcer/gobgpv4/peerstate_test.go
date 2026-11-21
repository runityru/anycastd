package gobgpv4

import (
	"io"
	"net"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v4/api"
	"github.com/osrg/gobgp/v4/pkg/server"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/runityru/anycastd/announcer"
)

// TestPeerStateSource exercises the peer-state source against a real
// in-process GoBGP server pair, mirroring the adapter test setup.
func TestPeerStateSource(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

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

	logger := log.New()
	logger.SetOutput(io.Discard)

	src := NewPeerStateSource(srv, logger)

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

func TestNormalizeAdminState(t *testing.T) {
	tcs := []struct {
		name string
		in   api.PeerState_AdminState
		exp  uint32
	}{
		{name: "unspecified maps to up", in: api.PeerState_ADMIN_STATE_UNSPECIFIED, exp: 0},
		{name: "up maps to up", in: api.PeerState_ADMIN_STATE_UP, exp: 0},
		{name: "down maps to down", in: api.PeerState_ADMIN_STATE_DOWN, exp: 1},
		{name: "pfx_ct maps to pfx_ct", in: api.PeerState_ADMIN_STATE_PFX_CT, exp: 2},
		{name: "unknown maps to up", in: api.PeerState_AdminState(99), exp: 0},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tc.exp, normalizeAdminState(tc.in))
		})
	}
}

func TestNormalizePeerType(t *testing.T) {
	tcs := []struct {
		name string
		in   api.PeerType
		exp  uint32
	}{
		{name: "unspecified maps to internal", in: api.PeerType_PEER_TYPE_UNSPECIFIED, exp: 0},
		{name: "internal maps to internal", in: api.PeerType_PEER_TYPE_INTERNAL, exp: 0},
		{name: "external maps to external", in: api.PeerType_PEER_TYPE_EXTERNAL, exp: 1},
		{name: "unknown maps to internal", in: api.PeerType(99), exp: 0},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tc.exp, normalizePeerType(tc.in))
		})
	}
}

func TestNormalizeBfdState(t *testing.T) {
	tcs := []struct {
		name string
		in   api.BfdSessionState
		exp  uint32
	}{
		{name: "up maps to up", in: api.BfdSessionState_BFD_SESSION_STATE_UP, exp: 1},
		{name: "down maps to down", in: api.BfdSessionState_BFD_SESSION_STATE_DOWN, exp: 0},
		{name: "admin down maps to down", in: api.BfdSessionState_BFD_SESSION_STATE_ADMIN_DOWN, exp: 0},
		{name: "init maps to down", in: api.BfdSessionState_BFD_SESSION_STATE_INIT, exp: 0},
		{name: "unspecified maps to down", in: api.BfdSessionState_BFD_SESSION_STATE_UNSPECIFIED, exp: 0},
		{name: "unknown maps to down", in: api.BfdSessionState(99), exp: 0},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tc.exp, normalizeBfdState(tc.in))
		})
	}
}

// TestListPeersSkipsBadPeer verifies the per-peer mapping used by ListPeers:
// a single peer that fails to map is skipped (logged, not errored) without
// discarding the remaining healthy peers.
func TestListPeersSkipsBadPeer(t *testing.T) {
	r := require.New(t)
	logger := log.New()
	logger.SetOutput(io.Discard)
	src := NewPeerStateSource(nil, logger).(*peerStateSource)

	// A peer carrying a negative enum value fails to map and is skipped.
	bad := &api.Peer{
		Conf:  &api.PeerConf{NeighborAddress: "10.0.0.1"},
		State: &api.PeerState{SessionState: api.PeerState_SessionState(-1)},
	}
	_, ok := src.mapPeer(bad)
	r.False(ok, "a bad peer should be skipped, not error")

	// A healthy peer is still mapped and returned.
	good := &api.Peer{
		Conf:  &api.PeerConf{NeighborAddress: "10.0.0.2"},
		State: &api.PeerState{},
	}
	ps, ok := src.mapPeer(good)
	r.True(ok, "a healthy peer should be mapped")
	r.Equal("10.0.0.2", ps.NeighborAddress)
}

// awaitPeerState polls ListPeers until a peer with the given neighbor address
// appears, returning its state. The BGP session itself may never establish (the
// BFD test peers do not speak BGP), so we poll for the peer to be listed.
func awaitPeerState(t *testing.T, src announcer.PeerStateSource, addr string) announcer.PeerState {
	t.Helper()
	var ps announcer.PeerState
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		states, err := src.ListPeers(t.Context())
		assert.NoError(c, err)
		for i := range states {
			if states[i].NeighborAddress == addr {
				ps = states[i]
				return
			}
		}
	}, 10*time.Second, 100*time.Millisecond, "peer %q never listed", addr)
	return ps
}

// TestPeerStateSourceBFD exercises the ListBfdPeer -> PeerState.Bfd bridge in
// peerStateSource.ListPeers against a real in-memory GoBGP v4 server with a
// live BFD session (reusing the bfd_integration_test topology), so the
// metric's source of truth (peer_bfd_state) is verified end to end.
func TestPeerStateSourceBFD(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	t.Run("BFD session up maps to Bfd.Enabled=true Bfd.State=1", func(t *testing.T) {
		peer := newBfdTestPeer(t, "udp4", server.BfdServerPort)
		defer peer.close()
		bfdPort := uint16(peer.Port())

		srv := startBFDDriver(t,
			announcer.PeerSpec{Name: "bfd-peer", RemoteAddress: "127.0.0.1", RemoteASN: 65000, LocalAddress: "127.0.0.1"},
			announcer.BfdSpec{Enabled: true, Port: &bfdPort},
		)
		waitBFDSessionUp(t, srv)

		ps := awaitPeerState(t, NewPeerStateSource(srv, logger), "127.0.0.1")
		r := require.New(t)
		r.True(ps.Bfd.Enabled)
		r.Equal(uint32(1), ps.Bfd.State)
		// A freshly-established UP session must not yet have recorded any
		// failure transition.
		r.Equal(uint64(0), ps.Bfd.FailureTransitions)
	})

	t.Run("BFD session down maps to Bfd.Enabled=true Bfd.State=0", func(t *testing.T) {
		// BFD enabled towards a port nothing listens on: the session exists but
		// stays DOWN, and is still reported by ListBfdPeer.
		bfdPort := uint16(65400)
		srv := startBFDDriver(t,
			announcer.PeerSpec{Name: "bfd-peer", RemoteAddress: "127.0.0.1", RemoteASN: 65000, LocalAddress: "127.0.0.1"},
			announcer.BfdSpec{Enabled: true, Port: &bfdPort},
		)

		ps := awaitPeerState(t, NewPeerStateSource(srv, logger), "127.0.0.1")
		r := require.New(t)
		r.True(ps.Bfd.Enabled)
		r.Equal(uint32(0), ps.Bfd.State)
	})

	t.Run("BFD disabled leaves Bfd.Enabled=false Bfd.State=0", func(t *testing.T) {
		srv := startBFDDriver(t,
			announcer.PeerSpec{Name: "no-bfd", RemoteAddress: "127.0.0.1", RemoteASN: 65000, LocalAddress: "127.0.0.1"},
			announcer.BfdSpec{Enabled: false},
		)

		ps := awaitPeerState(t, NewPeerStateSource(srv, logger), "127.0.0.1")
		r := require.New(t)
		r.False(ps.Bfd.Enabled)
		r.Equal(uint32(0), ps.Bfd.State)
	})
}

// TestCollectBfdState covers the defensive nil guards in the ListBfdPeer
// callback: a nil *api.BfdPeerState leaves the peer BFD-disabled, a non-nil
// state is recorded with every field populated, and a nil BfdAsync sub-message
// leaves the async packet counters at zero.
func TestCollectBfdState(t *testing.T) {
	r := require.New(t)
	m := map[string]announcer.BfdState{}

	// A nil *api.BfdPeerState is skipped entirely.
	collectBfdState(m, "10.0.0.1", nil)
	r.NotContains(m, "10.0.0.1")

	// A fully-populated state round-trips into every engine-agnostic field.
	collectBfdState(m, "10.0.0.2", &api.BfdPeerState{
		SessionState:                 api.BfdSessionState_BFD_SESSION_STATE_UP,
		RemoteSessionState:           api.BfdSessionState_BFD_SESSION_STATE_UP,
		LastFailureTime:              1234,
		FailureTransitions:           7,
		LocalDiscriminator:           100,
		RemoteDiscriminator:          200,
		LocalDiagnosticCode:          api.BfdDiagnosticCode_BFD_DIAGNOSTIC_CODE_PATH_DOWN,
		RemoteDiagnosticCode:         api.BfdDiagnosticCode_BFD_DIAGNOSTIC_CODE_DETECTION_TIMEOUT,
		RemoteMinimumReceiveInterval: 100000,
		BfdAsync: &api.BfdAsyncCounters{
			LastPacketTransmitted: 111,
			LastPacketReceived:    222,
			TransmittedPackets:    333,
			ReceivedPackets:       444,
		},
	})
	r.Contains(m, "10.0.0.2")
	r.Equal(true, m["10.0.0.2"].Enabled)
	r.Equal(uint32(1), m["10.0.0.2"].State)
	r.Equal(uint32(1), m["10.0.0.2"].RemoteState)
	r.Equal(uint64(1234), m["10.0.0.2"].LastFailureTime)
	r.Equal(uint64(7), m["10.0.0.2"].FailureTransitions)
	r.Equal(uint32(100), m["10.0.0.2"].LocalDiscriminator)
	r.Equal(uint32(200), m["10.0.0.2"].RemoteDiscriminator)
	r.Equal(uint32(5), m["10.0.0.2"].LocalDiagnostic)
	r.Equal(uint32(1), m["10.0.0.2"].RemoteDiagnostic)
	r.Equal(uint32(100000), m["10.0.0.2"].RemoteMinRxInterval)
	r.Equal(uint64(111), m["10.0.0.2"].LastPacketTransmitted)
	r.Equal(uint64(222), m["10.0.0.2"].LastPacketReceived)
	r.Equal(uint64(333), m["10.0.0.2"].TransmittedPackets)
	r.Equal(uint64(444), m["10.0.0.2"].ReceivedPackets)

	// A nil BfdAsync sub-message is tolerated: the state maps with zero async
	// packet counters instead of panicking.
	collectBfdState(m, "10.0.0.3", &api.BfdPeerState{
		SessionState: api.BfdSessionState_BFD_SESSION_STATE_DOWN,
	})
	r.Contains(m, "10.0.0.3")
	r.Equal(uint32(0), m["10.0.0.3"].State)
	r.Equal(uint64(0), m["10.0.0.3"].TransmittedPackets)
	r.Equal(uint64(0), m["10.0.0.3"].ReceivedPackets)
}

// TestPeerStateFromV4RouteAndASN asserts that peerStateFromV4 sums the route
// counters across the peer's AFI/SAFI entries and carries the timers/ASN
// values through, without leaking GoBGP types into announcer.PeerState.
func TestPeerStateFromV4RouteAndASN(t *testing.T) {
	r := require.New(t)

	uptime := &timestamppb.Timestamp{Seconds: 1700000000}
	p := &api.Peer{
		Conf: &api.PeerConf{NeighborAddress: "192.0.2.1"},
		State: &api.PeerState{
			SessionState: api.PeerState_SessionState(6),
			PeerAsn:      65001,
			LocalAsn:     65000,
		},
		Timers: &api.Timers{
			State: &api.TimersState{Uptime: uptime},
		},
		AfiSafis: []*api.AfiSafi{
			{State: &api.AfiSafiState{Received: 10, Accepted: 11, Advertised: 12}},
			{State: &api.AfiSafiState{Received: 20, Accepted: 21, Advertised: 22}},
			nil, // nil entry must be skipped
			{},  // entry with nil state must be skipped
		},
	}

	ps, err := peerStateFromV4(p)
	r.NoError(err)
	r.Equal(uint64(30), ps.RoutesReceived)
	r.Equal(uint64(32), ps.RoutesAccepted)
	r.Equal(uint64(34), ps.RoutesAdvertised)
	r.Equal(uint64(1700000000), ps.Uptime)
	r.Equal(uint32(65001), ps.RemoteASN)
	r.Equal(uint32(65000), ps.LocalASN)
}
