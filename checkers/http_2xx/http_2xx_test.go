package http_2xx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/teran/anycastd/config"
)

func (s *http2xxTestSuite) TestTrivial() {
	s.handlerMock.On("ServeHTTP", http.MethodGet, "/ping").Return(http.StatusOK).Once()

	c, err := New(spec{
		URL:      s.srv.URL + "/ping",
		Method:   "GET",
		Tries:    1,
		Interval: config.Duration(1 * time.Second),
		Timeout:  config.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	err = c.Check(context.TODO())
	s.Require().NoError(err)
}

func (s *http2xxTestSuite) TestFiveTries() {
	s.handlerMock.On("ServeHTTP", http.MethodGet, "/ping").Return(http.StatusOK).Once()

	c, err := New(spec{
		URL:      s.srv.URL + "/ping",
		Method:   "GET",
		Tries:    5,
		Interval: config.Duration(1 * time.Second),
		Timeout:  config.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	err = c.Check(context.TODO())
	s.Require().NoError(err)
}

func (s *http2xxTestSuite) TestSuccessFromThirdTime() {
	s.handlerMock.On("ServeHTTP", http.MethodGet, "/ping").Return(http.StatusServiceUnavailable).Once()
	s.handlerMock.On("ServeHTTP", http.MethodGet, "/ping").Return(http.StatusServiceUnavailable).Once()
	s.handlerMock.On("ServeHTTP", http.MethodGet, "/ping").Return(http.StatusOK).Once()

	c, err := New(spec{
		URL:      s.srv.URL + "/ping",
		Method:   "GET",
		Tries:    3,
		Interval: config.Duration(1 * time.Millisecond),
		Timeout:  config.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	err = c.Check(context.TODO())
	s.Require().NoError(err)
}

func (s *http2xxTestSuite) TestNegative() {
	s.handlerMock.On("ServeHTTP", http.MethodGet, "/ping").Return(http.StatusServiceUnavailable).Twice()

	c, err := New(spec{
		URL:      s.srv.URL + "/ping",
		Method:   "GET",
		Tries:    2,
		Interval: config.Duration(1 * time.Millisecond),
		Timeout:  config.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	err = c.Check(context.TODO())
	s.Require().Error(err)
	s.Require().Equal(
		"check failed: 2 tries with 1ms interval; last error: `Unexpected code received: 503`",
		err.Error(),
	)
}

// Definitions ...
type http2xxTestSuite struct {
	suite.Suite

	handlerMock *handlerMock
	srv         *httptest.Server
}

func (s *http2xxTestSuite) SetupTest() {
	s.handlerMock = &handlerMock{}
	s.srv = httptest.NewServer(s.handlerMock)
}

func (s *http2xxTestSuite) TearDownTest() {
	s.srv.Close()

	s.handlerMock.AssertExpectations(s.T())
}

func TestHttp2xxTestSuite(t *testing.T) {
	suite.Run(t, &http2xxTestSuite{})
}

type handlerMock struct {
	mock.Mock
}

func (m *handlerMock) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	args := m.Called(r.Method, r.URL.Path)

	rw.WriteHeader(args.Int(0))
}