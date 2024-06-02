package announcer

import (
	"context"

	"github.com/stretchr/testify/mock"
)

var _ Announcer = (*Mock)(nil)

type Mock struct {
	mock.Mock
}

func NewMock() *Mock {
	return &Mock{}
}

func (m *Mock) Announce(ctx context.Context) error {
	args := m.Called()
	return args.Error(0)
}

func (m *Mock) Denounce(ctx context.Context) error {
	args := m.Called()
	return args.Error(0)
}
