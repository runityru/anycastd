package announcer

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type goBGPMock struct {
	mock.Mock
}

func newGoBGPMock() *goBGPMock {
	return &goBGPMock{}
}

func (m *goBGPMock) AddPath(_ context.Context, prefix, nextHop string) error {
	args := m.Called(prefix, nextHop)
	return args.Error(0)
}

func (m *goBGPMock) DeletePath(_ context.Context, prefix string) error {
	args := m.Called(prefix)
	return args.Error(0)
}
