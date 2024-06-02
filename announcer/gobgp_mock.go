package announcer

import (
	"context"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/stretchr/testify/mock"
)

type goBGPMock struct {
	mock.Mock
}

func newGoBGPMock() *goBGPMock {
	return &goBGPMock{}
}

func (m *goBGPMock) AddPath(_ context.Context, r *api.AddPathRequest) (*api.AddPathResponse, error) {
	args := m.Called(r.GetPath().GetNlri().String())
	return &api.AddPathResponse{
		Uuid: args.Get(0).([]byte),
	}, args.Error(1)
}

func (m *goBGPMock) DeletePath(ctx context.Context, r *api.DeletePathRequest) error {
	args := m.Called(r.GetPath().GetNlri().String())
	return args.Error(0)
}
