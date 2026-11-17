package announcer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestDriverMockSatisfiesInterfaces is a compile-time + behavioural check that
// DriverMock implements the full Driver contract (ServerLifecycle, PeerManager,
// PeerStateSource, BGPServer).
func TestDriverMockSatisfiesInterfaces(t *testing.T) {
	r := require.New(t)

	ctx := context.Background()
	d := NewDriverMock()

	d.On("Serve").Return(nil)
	r.NoError(d.Serve())

	spec := DriverSpec{RouterID: "10.3.3.3", LocalASN: 65999}
	d.On("Start", mock.Anything, spec).Return(nil)
	r.NoError(d.Start(ctx, spec))

	d.On("Stop", mock.Anything).Return(nil)
	r.NoError(d.Stop(ctx))

	peer := PeerSpec{Name: "p1", RemoteAddress: "10.0.0.2", RemoteASN: 64513}
	d.On("AddPeer", mock.Anything, peer).Return(nil)
	r.NoError(d.AddPeer(ctx, peer))

	d.On("AddPath", mock.Anything, "10.0.0.0/24").Return(nil)
	r.NoError(d.AddPath(ctx, "10.0.0.0/24"))

	d.On("DeletePath", mock.Anything, "10.0.0.0/24").Return(nil)
	r.NoError(d.DeletePath(ctx, "10.0.0.0/24"))

	d.AssertExpectations(t)
}

// TestDriverMockAsPeerStateSourceForMetrics verifies DriverMock can feed
// announcer.Metrics.Run (PeerStateSource) — i.e. it is ready for the H4
// metrics test. NewMetricsRepository only needs ListPeers, which the mock
// provides.
func TestDriverMockAsPeerStateSourceForMetrics(t *testing.T) {
	r := require.New(t)

	d := NewDriverMock()
	d.On("ListPeers", mock.Anything).Return([]PeerState{
		{NeighborAddress: "10.0.0.2", SessionState: 6, AdminState: 0, BfdEnabled: true, BfdState: 1},
	}, nil)

	peers, err := d.ListPeers(context.Background())
	r.NoError(err)
	r.Len(peers, 1)
	r.Equal("10.0.0.2", peers[0].NeighborAddress)
	r.Equal(uint32(1), peers[0].BfdState)
	d.AssertExpectations(t)
}
