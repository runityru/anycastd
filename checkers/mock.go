package checkers

import (
	"context"

	"github.com/stretchr/testify/mock"
)

var _ Checker = (*Mock)(nil)

type Mock struct {
	mock.Mock
}

func NewMock() *Mock {
	return &Mock{}
}

func (m *Mock) Check(context.Context) error {
	args := m.Called()
	return args.Error(0)
}
