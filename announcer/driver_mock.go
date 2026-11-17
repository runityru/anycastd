package announcer

import (
	"context"

	"github.com/stretchr/testify/mock"
)

var _ Driver = (*DriverMock)(nil)

// DriverMock is a hand-written testify mock of the full announcer.Driver
// interface (ServerLifecycle + PeerManager + PeerStateSource + BGPServer).
// The project does not use mockgen; mocks are written by hand (see AGENTS.md).
type DriverMock struct {
	mock.Mock
}

func NewDriverMock() *DriverMock {
	return &DriverMock{}
}

func (m *DriverMock) Serve() error {
	args := m.Called()
	return args.Error(0)
}

func (m *DriverMock) Start(ctx context.Context, spec DriverSpec) error {
	args := m.Called(ctx, spec)
	return args.Error(0)
}

func (m *DriverMock) Stop(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *DriverMock) AddPeer(ctx context.Context, peer PeerSpec) error {
	args := m.Called(ctx, peer)
	return args.Error(0)
}

func (m *DriverMock) ListPeers(ctx context.Context) ([]PeerState, error) {
	args := m.Called(ctx)
	return args.Get(0).([]PeerState), args.Error(1)
}

func (m *DriverMock) AddPath(ctx context.Context, prefix string) error {
	args := m.Called(ctx, prefix)
	return args.Error(0)
}

func (m *DriverMock) DeletePath(ctx context.Context, prefix string) error {
	args := m.Called(ctx, prefix)
	return args.Error(0)
}
