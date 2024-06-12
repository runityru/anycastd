package dns_lookup

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	th "github.com/teran/go-time"
)

func TestMkResolver(t *testing.T) {
	r := require.New(t)

	res := mkResolver("127.0.0.1:53", 3*time.Second)
	r.NotNil(res)

	nativeResolver := res.(*net.Resolver)
	r.True(nativeResolver.PreferGo)
	r.False(nativeResolver.StrictErrors)
}

func TestSpec(t *testing.T) {
	r := require.New(t)

	data, err := os.ReadFile("testdata/spec.json")
	r.NoError(err)

	c, err := NewFromSpec(json.RawMessage(data))
	r.NoError(err)

	dl := c.(*dns_lookup)
	r.Equal("example.com", dl.query)
	r.Equal("127.0.0.1:53", dl.resolver)
	r.Equal(uint8(3), dl.tries)
	r.Equal(300*time.Millisecond, dl.interval)
	r.Equal(3*time.Second, dl.timeout)
}

func (s *checkTestSuite) TestHappyPath() {
	l, err := New(spec{
		Query:    "example.com",
		Resolver: "127.0.0.1:53",
		Tries:    3,
		Interval: th.Duration(2 * time.Second),
		Timeout:  th.Duration(5 * time.Second),
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
		Interval: th.Duration(2 * time.Second),
		Timeout:  th.Duration(5 * time.Second),
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
