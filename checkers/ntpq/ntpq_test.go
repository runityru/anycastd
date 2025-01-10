package ntpq

import (
	"context"
	"testing"
	"time"

	"github.com/beevik/ntp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	th "github.com/teran/go-time"
)

func (s *checkTestSuite) TestOffsetTooBig() {
	l, err := New(spec{
		Server:          "pool.ntp.org",
		SrcAddr:         "192.168.0.1",
		Tries:           3,
		OffsetThreshold: th.Duration(125 * time.Millisecond),
		Interval:        th.Duration(2 * time.Second),
		Timeout:         th.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	c := l.(*ntpq)
	c.queryFn = s.ntpM.queryMock

	s.ntpM.On("queryMock", "pool.ntp.org", ntp.QueryOptions{
		LocalAddress: "192.168.0.1",
		Timeout:      (5 * time.Second),
	}).Return(&ntp.Response{
		RTT:         time.Duration(15 * time.Millisecond),
		ClockOffset: time.Duration(150 * time.Millisecond),
		ReferenceID: 1,
	}, nil).Times(int(c.tries))

	err = c.Check(context.Background())
	s.Require().Error(err)
}

func (s *checkTestSuite) TestOffset() {
	l, err := New(spec{
		Server:          "pool.ntp.org",
		SrcAddr:         "192.168.0.1",
		Tries:           3,
		OffsetThreshold: th.Duration(125 * time.Millisecond),
		Interval:        th.Duration(2 * time.Second),
		Timeout:         th.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	c := l.(*ntpq)
	c.queryFn = s.ntpM.queryMock

	s.ntpM.On("queryMock", "pool.ntp.org", ntp.QueryOptions{
		LocalAddress: "192.168.0.1",
		Timeout:      (5 * time.Second),
	}).Return(&ntp.Response{
		RTT:         time.Duration(15 * time.Millisecond),
		ClockOffset: time.Duration(10 * time.Millisecond),
		ReferenceID: 1,
	}, nil).Times(int(c.tries))

	err = c.Check(context.Background())
	s.Require().NoError(err)
}

func (s *checkTestSuite) TestNegativeOffset() {
	l, err := New(spec{
		Server:          "pool.ntp.org",
		SrcAddr:         "192.168.0.1",
		Tries:           3,
		OffsetThreshold: th.Duration(125 * time.Millisecond),
		Interval:        th.Duration(2 * time.Second),
		Timeout:         th.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	c := l.(*ntpq)
	c.queryFn = s.ntpM.queryMock

	s.ntpM.On("queryMock", "pool.ntp.org", ntp.QueryOptions{
		LocalAddress: "192.168.0.1",
		Timeout:      (5 * time.Second),
	}).Return(&ntp.Response{
		RTT:         time.Duration(15 * time.Millisecond),
		ClockOffset: time.Duration(-10 * time.Millisecond),
		ReferenceID: 1,
	}, nil).Times(int(c.tries))

	err = c.Check(context.Background())
	s.Require().NoError(err)
}

type checkTestSuite struct {
	suite.Suite

	ntpM *mockNtp
}

func (s *checkTestSuite) SetupTest() {
	s.ntpM = &mockNtp{}
}

func TestCheckTestSuite(t *testing.T) {
	suite.Run(t, &checkTestSuite{})
}

type mockNtp struct {
	mock.Mock
}

func (m *mockNtp) queryMock(server string, opts ntp.QueryOptions) (*ntp.Response, error) {
	args := m.Called(server, opts)
	return args.Get(0).(*ntp.Response), args.Error(1)
}
