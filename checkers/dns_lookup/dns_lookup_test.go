package dns_lookup

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/teran/anycastd/config"
)

func (s *checkTestSuite) TestHappyPath() {
	l, err := New(spec{
		Query:    "example.com",
		Resolver: "127.0.0.1:53",
		Tries:    3,
		Interval: config.Duration(2 * time.Second),
		Timeout:  config.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	s.resolverM.On("LookupHost", "example.com").Return([]string{"127.0.0.1"}, nil).Once()

	c := l.(*dns_lookup)
	c.resolverMaker = s.mkResolver

	err = c.Check(context.Background())
	s.Require().NoError(err)
}

func (s *checkTestSuite) TestSecondTry() {
	l, err := New(spec{
		Query:    "example.com",
		Resolver: "127.0.0.1:53",
		Tries:    3,
		Interval: config.Duration(2 * time.Second),
		Timeout:  config.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	s.resolverM.On("LookupHost", "example.com").Return([]string{}, errors.New("blah")).Once()
	s.resolverM.On("LookupHost", "example.com").Return([]string{"127.0.0.1"}, nil).Once()

	c := l.(*dns_lookup)
	c.resolverMaker = s.mkResolver

	err = c.Check(context.Background())
	s.Require().NoError(err)
}

// Definitions
type checkTestSuite struct {
	suite.Suite

	resolverM *mockResolver
}

func (s *checkTestSuite) SetupTest() {
	s.resolverM = &mockResolver{}
}

func (s *checkTestSuite) TearDownTest() {
	s.resolverM.AssertExpectations(s.T())
	s.resolverM = nil
}

func (s *checkTestSuite) mkResolver(resolver string, timeout time.Duration) resolver {
	return s.resolverM
}

func TestCheckTestSuite(t *testing.T) {
	suite.Run(t, &checkTestSuite{})
}

type mockResolver struct {
	mock.Mock
}

func (m *mockResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	args := m.Called(host)
	return args.Get(0).([]string), args.Error(1)
}
