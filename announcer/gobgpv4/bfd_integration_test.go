package gobgpv4

// BFD integration test.
//
// These tests drive a real GoBGP v4 BFD session from a single in-memory
// GoBGP server (the anycastd "driver" side, peers configured through newPeer,
// i.e. api.Peer.Bfd) against a minimal external BFD peer that speaks the RFC
// 5880 protocol over a raw UDP socket. This exercises the MR1/MR2 BFD wiring
// end to end: newPeer fills api.Peer.Bfd, the server opens a real BFD session
// and the session reaches the UP state over actual packets.
//
// NOTE on topology: we deliberately use ONE server.BgpServer + an external UDP
// peer rather than two server.BgpServer instances. GoBGP's BFD receive sockets
// are bound to the fixed RFC ports (server.go: StartBgp calls
// s.bfdServer.Start(ctx, oc.BfdConfig{Port: BfdServerPort}) with
// BfdServerPort=3784, 4784 for multihop) on all interfaces. Two BgpServer
// instances on one host therefore both bind :3784 (with SO_REUSEADDR) and only
// the first binder ever receives BFD packets, so a two-server test cannot reach
// UP on both sides (verified empirically: one side goes UP, the other stays
// DOWN forever). The external-peer topology reaches a real, deterministic UP
// while still validating anycastd's driver->api.Peer.Bfd path.
//
// Tests are NOT parallel (t.Parallel) because each driver server binds the
// shared fixed BFD port 3784/4784; serializing avoids cross-test collisions.

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v4/api"
	"github.com/osrg/gobgp/v4/pkg/packet/bfd"
	"github.com/osrg/gobgp/v4/pkg/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"github.com/runityru/anycastd/announcer"
)

// bfdTestPeer is a minimal external RFC 5880 BFD peer. It binds an ephemeral
// UDP socket (whose port the driver peer is configured to send BFD to), and
// runs the Down->Init->Up handshake in response to the server's packets, so
// the server's BFD session comes UP.
type bfdTestPeer struct {
	t        *testing.T
	conn     *net.UDPConn
	network  string // "udp4" or "udp6"
	destPort int    // server BFD receive port: 3784 single-hop, 4784 multihop

	mu         sync.Mutex
	myDisc     uint32
	remoteDisc uint32
	state      bfd.StateType
}

// newBfdTestPeer binds a fresh ephemeral UDP socket on the loopback of the
// given family. t.Skip is called when the loopback family is unavailable
// (e.g. no ::1). The bound port is exposed via Port().
func newBfdTestPeer(t *testing.T, network string, destPort int) *bfdTestPeer {
	t.Helper()
	host := "127.0.0.1"
	if network == "udp6" {
		host = "::1"
	}
	conn, err := net.ListenUDP(network, &net.UDPAddr{IP: net.ParseIP(host)})
	if err != nil {
		t.Skipf("no %s loopback available: %v", host, err)
	}

	// Single-hop BFD requires TTL/Hop Limit 255 on transmitted packets
	// (RFC 5881 section 5); without it the server discards our packets.
	if network == "udp6" {
		if err := ipv6.NewPacketConn(conn).SetHopLimit(255); err != nil {
			_ = conn.Close()
			t.Fatalf("set hop limit 255: %v", err)
		}
	} else {
		if err := ipv4.NewPacketConn(conn).SetTTL(255); err != nil {
			_ = conn.Close()
			t.Fatalf("set ttl 255: %v", err)
		}
	}

	p := &bfdTestPeer{
		t:        t,
		conn:     conn,
		network:  network,
		destPort: destPort,
		myDisc:   0x01CD0D,
		state:    bfd.StateDown,
	}
	go p.loop()
	return p
}

// Port returns the local UDP port the peer listens on; the driver peer must be
// configured (api.BfdPeerConfig.Port) to send its BFD packets here.
func (p *bfdTestPeer) Port() int {
	return p.conn.LocalAddr().(*net.UDPAddr).Port
}

func (p *bfdTestPeer) close() { _ = p.conn.Close() }

func (p *bfdTestPeer) loop() {
	buf := make([]byte, 2048)
	for {
		n, _, err := p.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		var h bfd.BFDHeader
		if err := h.UnmarshalBinary(buf[:n]); err != nil {
			p.t.Logf("bfd peer: dropping malformed packet: %v", err)
			continue
		}
		p.handle(&h)
	}
}

// handle runs the peer side of the BFD handshake: once it has seen the
// server's session (Down), it answers Init so the server moves toward UP, and
// answers Init/Up with Up so the session stabilises UP.
func (p *bfdTestPeer) handle(h *bfd.BFDHeader) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.remoteDisc = h.MyDiscriminator
	switch h.State {
	case bfd.StateDown:
		if p.state == bfd.StateDown {
			p.state = bfd.StateInit
		}
	case bfd.StateInit, bfd.StateUp:
		p.state = bfd.StateUp
	case bfd.StateAdminDown:
		p.state = bfd.StateDown
	}
	p.sendLocked(p.state)
}

func (p *bfdTestPeer) sendLocked(state bfd.StateType) {
	host := "127.0.0.1"
	if p.network == "udp6" {
		host = "::1"
	}
	h := &bfd.BFDHeader{
		Version:               1,
		State:                 state,
		DetectTimeMultiplier:  5,
		MyDiscriminator:       p.myDisc,
		YourDiscriminator:     p.remoteDisc,
		DesiredMinTxInterval:  200000,
		RequiredMinRxInterval: 200000,
	}
	b, err := h.MarshalBinary()
	if err != nil {
		p.t.Logf("bfd peer: marshal: %v", err)
		return
	}
	// Reply to the server's fixed BFD receive port, never to the ephemeral
	// source port the server happened to send from.
	if _, err := p.conn.WriteToUDP(b, &net.UDPAddr{IP: net.ParseIP(host), Port: p.destPort}); err != nil {
		p.t.Logf("bfd peer: send: %v", err)
	}
}

// startBFDDriver starts an in-memory GoBGP server with a single peer described
// by peerSpec/bfdSpec, configured through newPeer (the anycastd code path under
// test). It returns the server; the caller stops it with t.Cleanup.
func startBFDDriver(t *testing.T, peerSpec announcer.PeerSpec, bfdSpec announcer.BfdSpec) *server.BgpServer {
	t.Helper()
	ctx := context.Background()

	srv := server.NewBgpServer()
	go srv.Serve()
	t.Cleanup(srv.Stop)
	require.NoError(t, srv.StartBgp(ctx, &api.StartBgpRequest{
		Global: &api.Global{RouterId: "10.3.3.3", Asn: 65999, ListenPort: -1},
	}))
	require.NoError(t, srv.AddPeer(ctx, &api.AddPeerRequest{Peer: newPeer(peerSpec, bfdSpec)}))
	return srv
}

// waitBFDSessionUp polls ListBfdPeer until the BFD session reports UP.
func waitBFDSessionUp(t *testing.T, srv *server.BgpServer) {
	t.Helper()
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		state := bfdSessionState(srv)
		assert.Equal(c, api.BfdSessionState_BFD_SESSION_STATE_UP, state, "BFD session did not reach UP")
	}, 8*time.Second, 100*time.Millisecond)
}

// bfdSessionState returns the SessionState of the single BFD peer known to srv,
// or UNKNOWN (0) when no BFD session exists.
func bfdSessionState(srv *server.BgpServer) api.BfdSessionState {
	ctx := context.Background()
	var state api.BfdSessionState
	var found bool
	srv.ListBfdPeer(ctx, func(_ string, st *api.BfdPeerState) {
		found = true
		state = st.SessionState
	})
	if !found {
		return 0 // api.BfdSessionState_BFD_SESSION_STATE_UNKNOWN
	}
	return state
}

func TestBFDIntegrationIPv4Up(t *testing.T) {
	// Bind the external peer first so the driver's Bfd.Port matches a free port.
	peer := newBfdTestPeer(t, "udp4", server.BfdServerPort)
	defer peer.close()

	bfdPort := uint16(peer.Port())
	srv := startBFDDriver(t,
		announcer.PeerSpec{Name: "bfd-peer", RemoteAddress: "127.0.0.1", RemoteASN: 65000, LocalAddress: "127.0.0.1"},
		announcer.BfdSpec{Enabled: true, Port: &bfdPort},
	)

	waitBFDSessionUp(t, srv)
}

func TestBFDIntegrationIPv6Up(t *testing.T) {
	peer := newBfdTestPeer(t, "udp6", server.BfdServerPort)
	defer peer.close()

	bfdPort := uint16(peer.Port())
	srv := startBFDDriver(t,
		announcer.PeerSpec{Name: "bfd-peer", RemoteAddress: "::1", RemoteASN: 65000, LocalAddress: "::1"},
		announcer.BfdSpec{Enabled: true, Port: &bfdPort},
	)

	waitBFDSessionUp(t, srv)
}

func TestBFDIntegrationMultihopUp(t *testing.T) {
	// 4784 is the RFC 5883 multihop BFD port (GoBGP's bfdMultihopPort, unexported).
	peer := newBfdTestPeer(t, "udp4", 4784)
	defer peer.close()

	bfdPort := uint16(peer.Port())
	srv := startBFDDriver(t,
		announcer.PeerSpec{Name: "bfd-peer", RemoteAddress: "127.0.0.1", RemoteASN: 65000, LocalAddress: "127.0.0.1", EnableMultihop: true, MultihopTTL: 2},
		announcer.BfdSpec{Enabled: true, Port: &bfdPort},
	)

	waitBFDSessionUp(t, srv)
}

func TestBFDIntegrationDeletePeerTearsSessionDown(t *testing.T) {
	peer := newBfdTestPeer(t, "udp4", server.BfdServerPort)
	defer peer.close()

	bfdPort := uint16(peer.Port())
	srv := startBFDDriver(t,
		announcer.PeerSpec{Name: "bfd-peer", RemoteAddress: "127.0.0.1", RemoteASN: 65000, LocalAddress: "127.0.0.1"},
		announcer.BfdSpec{Enabled: true, Port: &bfdPort},
	)

	waitBFDSessionUp(t, srv)

	// Removing the peer must remove the BFD session.
	require.NoError(t, srv.DeletePeer(context.Background(), &api.DeletePeerRequest{Address: "127.0.0.1"}))

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var count int
		srv.ListBfdPeer(context.Background(), func(_ string, _ *api.BfdPeerState) { count++ })
		assert.Equal(c, 0, count, "BFD session should be gone after DeletePeer")
	}, 3*time.Second, 100*time.Millisecond)
}

func TestBFDIntegrationDisabledOnDriverStaysDown(t *testing.T) {
	// A peer with BFD disabled must NOT create a BFD session, even though the
	// remote keeps sending BFD packets (BFD is opt-in on both sides).
	peer := newBfdTestPeer(t, "udp4", server.BfdServerPort)
	defer peer.close()

	srv := startBFDDriver(t,
		announcer.PeerSpec{Name: "no-bfd", RemoteAddress: "127.0.0.1", RemoteASN: 65000, LocalAddress: "127.0.0.1"},
		announcer.BfdSpec{Enabled: false},
	)

	// Give the (sending) remote time to be heard; the session must never appear.
	time.Sleep(1500 * time.Millisecond)
	var count int
	srv.ListBfdPeer(context.Background(), func(_ string, _ *api.BfdPeerState) { count++ })
	require.Zero(t, count, "BFD session must not exist when the driver peer has BFD disabled")
}
