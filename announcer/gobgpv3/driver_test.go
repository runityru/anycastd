package gobgpv3

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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

// testLogWriter accumulates logrus output in a thread-safe buffer. It never
// touches the testing.T from the engine's background watch goroutines (which
// keep logging after the test body returns), avoiding data races; the buffer
// is flushed to the test log only from the test goroutine in a Cleanup.
type testLogWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *testLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *testLogWriter) dump(t testing.TB) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t.Logf("%s", w.buf.String())
}

// newTestLogger returns a logrus logger that buffers into a thread-safe writer
// and flushes to the test log only when the test failed, so diagnostic engine
// logs surface on failure without racing the testing.T.
func newTestLogger(t testing.TB) *log.Logger {
	t.Helper()
	w := &testLogWriter{}
	logger := log.New()
	logger.SetOutput(w)
	logger.SetLevel(log.TraceLevel)
	t.Cleanup(func() {
		if t.Failed() {
			w.dump(t)
		}
	})
	return logger
}

// startTestPeer runs a second in-memory BgpServer on a free loopback port in
// PassiveMode, so the driver under test dials it via RemotePort.
func startTestPeer(t *testing.T, loopback string, peerASN uint32) (*server.BgpServer, uint32) {
	t.Helper()
	r := require.New(t)
	ctx := t.Context()

	l, err := net.Listen("tcp", net.JoinHostPort(loopback, "0"))
	r.NoError(err)
	port := l.Addr().(*net.TCPAddr).Port
	r.NoError(l.Close())

	srv := server.NewBgpServer()
	go srv.Serve()
	t.Cleanup(func() { srv.Stop() })
	r.NoError(srv.StartBgp(ctx, &api.StartBgpRequest{
		Global: &api.Global{RouterId: "192.0.2.254", Asn: peerASN, ListenPort: int32(port), ListenAddresses: []string{loopback}},
	}))
	// The test peer accepts the driver's connection (driver ASN 64512).
	r.NoError(srv.AddPeer(ctx, &api.AddPeerRequest{Peer: &api.Peer{
		Conf:      &api.PeerConf{NeighborAddress: loopback, PeerAsn: 64512},
		Transport: &api.Transport{PassiveMode: true},
	}}))
	return srv, uint32(port)
}

// awaitEstablished polls ListPeers until the peer at addr reports an
// ESTABLISHED session with AdminState up.
func awaitEstablished(t *testing.T, d announcer.Driver, addr string) {
	t.Helper()
	ctx := t.Context()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		peers, err := d.ListPeers(ctx)
		assert.NoError(c, err)
		established := false
		for _, p := range peers {
			if p.NeighborAddress == addr &&
				p.SessionState == uint32(api.PeerState_ESTABLISHED) &&
				p.AdminState == 0 {
				established = true
				break
			}
		}
		assert.True(c, established, "peer %s not established; peers=%v", addr, peers)
	}, 10*time.Second, 100*time.Millisecond)
}

// newDriverWithPeer builds a driver started via Serve+Start with the given
// peers, returning the running driver and the Serve completion channel.
func newDriverWithPeer(t *testing.T, peers []announcer.PeerSpec) (announcer.Driver, chan error) {
	t.Helper()
	r := require.New(t)
	ctx := t.Context()

	spec := announcer.DriverSpec{
		RouterID: "192.0.2.1",
		LocalASN: 64512,
		Peers:    peers,
	}

	d, err := NewDriver(spec, newTestLogger(t))
	r.NoError(err)

	done := make(chan error, 1)
	go func() { done <- d.Serve() }()
	r.NoError(d.Start(ctx, spec))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	return d, done
}

func TestStartEstablishesPeer(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

	peerSrv, port := startTestPeer(t, "127.0.0.1", 64513)
	defer peerSrv.Stop()

	d, _ := newDriverWithPeer(t, []announcer.PeerSpec{{
		Name: "peer1", RemoteAddress: "127.0.0.1", RemoteASN: 64513,
		LocalAddress: "127.0.0.1", RemotePort: port,
	}})

	awaitEstablished(t, d, "127.0.0.1")

	// The peer must be present with an up admin state.
	peers, err := d.ListPeers(ctx)
	r.NoError(err)
	r.Len(peers, 1)
	r.Equal("127.0.0.1", peers[0].NeighborAddress)
	r.Equal(uint32(0), peers[0].AdminState)

	// Stop must be clean.
	r.NoError(d.Stop(ctx))
}

func TestAddPeerDynamic(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

	peerA, portA := startTestPeer(t, "127.0.0.1", 64513)
	defer peerA.Stop()
	peerB, portB := startTestPeer(t, "::1", 64514)
	defer peerB.Stop()

	d, _ := newDriverWithPeer(t, []announcer.PeerSpec{{
		Name: "peer1", RemoteAddress: "127.0.0.1", RemoteASN: 64513,
		LocalAddress: "127.0.0.1", RemotePort: portA,
	}})

	awaitEstablished(t, d, "127.0.0.1")

	// Add a second peer dynamically (a different address, since GoBGP keys
	// peers by address); it must also come up.
	r.NoError(d.AddPeer(ctx, announcer.PeerSpec{
		Name: "peer2", RemoteAddress: "::1", RemoteASN: 64514,
		LocalAddress: "::1", RemotePort: portB,
	}))

	awaitEstablished(t, d, "::1")

	peers, err := d.ListPeers(ctx)
	r.NoError(err)
	r.Len(peers, 2)
}

func TestAddPeerInvalid(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

	d, _ := newDriverWithPeer(t, nil)

	// An unparseable remote address must be rejected at AddPeer time.
	err := d.AddPeer(ctx, announcer.PeerSpec{
		Name: "bad", RemoteAddress: "not-an-ip", RemoteASN: 64513,
	})
	r.Error(err)

	peers, err := d.ListPeers(ctx)
	r.NoError(err)
	r.Empty(peers)
}

func TestStop(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

	peerSrv, port := startTestPeer(t, "127.0.0.1", 64513)
	defer peerSrv.Stop()

	d, _ := newDriverWithPeer(t, []announcer.PeerSpec{{
		Name: "peer1", RemoteAddress: "127.0.0.1", RemoteASN: 64513,
		LocalAddress: "127.0.0.1", RemotePort: port,
	}})

	awaitEstablished(t, d, "127.0.0.1")

	// The peer is present before shutdown.
	peers, err := d.ListPeers(ctx)
	r.NoError(err)
	r.Len(peers, 1)

	r.NoError(d.Stop(ctx))

	// After Stop the neighbor is torn down, so ListPeers is empty.
	peers, err = d.ListPeers(ctx)
	r.NoError(err)
	r.Empty(peers)
}

func TestServeRunsDriver(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

	d, done := newDriverWithPeer(t, nil)

	// The driver is functional while Serve runs (management ops require the
	// serve loop). Verify ListPeers works and an invalid peer is rejected.
	peers, err := d.ListPeers(ctx)
	r.NoError(err)
	r.Empty(peers)

	r.Error(d.AddPeer(ctx, announcer.PeerSpec{Name: "bad", RemoteAddress: "not-an-ip", RemoteASN: 64513}))

	r.NoError(d.Stop(ctx))

	// NOTE: gobgp v3's Serve() loop is an un-terminable dispatch loop that
	// only returns at process exit; the public driver API has no way to stop
	// it. So unlike v4 we do not wait on done here. The goroutine is blocked
	// on Serve forever, which is inherent to the v3 engine.
	_ = done
}
