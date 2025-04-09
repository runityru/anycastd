package http_2xx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	ptr "github.com/teran/go-ptr"
	th "github.com/teran/go-time"
)

var goDefaultHeaders = http.Header{
	"Accept-Encoding": []string{"gzip"},
	"User-Agent":      []string{"anycastd/1.0 (http_2xx checker)"},
}

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestSpec(t *testing.T) {
	r := require.New(t)

	data, err := os.ReadFile("testdata/spec.json")
	r.NoError(err)

	c, err := NewFromSpec(json.RawMessage(data))
	r.NoError(err)

	h := c.(*http_2xx)
	r.Equal("example.com", h.url)
	r.Equal("GET", h.method)
	r.Equal(uint8(10), h.tries)
	r.Equal(1*time.Second, h.interval)
	r.Equal(10*time.Second, h.client.Timeout)
}

func (s *http2xxTestSuite) TestTrivial() {
	s.handlerMock.
		On("ServeHTTP", http.MethodGet, strings.TrimPrefix(s.srv.URL, "http://"), goDefaultHeaders.Clone(), "/ping", []byte(nil)).
		Return(http.StatusOK).
		Once()

	c, err := New(spec{
		URL:      s.srv.URL + "/ping",
		Method:   "GET",
		Tries:    1,
		Interval: th.Duration(1 * time.Second),
		Timeout:  th.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	err = c.Check(context.TODO())
	s.Require().NoError(err)
}

func (s *http2xxTestSuite) TestTrivialWithHeaders() {
	v := goDefaultHeaders.Clone()
	v["X-Test-Header"] = []string{"x-test-value"}

	s.handlerMock.
		On("ServeHTTP", http.MethodGet, "example.com", v, "/ping", []byte(nil)).
		Return(http.StatusOK).
		Once()

	c, err := New(spec{
		URL:    s.srv.URL + "/ping",
		Method: "GET",
		Headers: map[string]string{
			"Host":          "example.com",
			"X-Test-Header": "x-test-value",
		},
		Tries:    1,
		Interval: th.Duration(1 * time.Second),
		Timeout:  th.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	err = c.Check(context.TODO())
	s.Require().NoError(err)
}

func (s *http2xxTestSuite) TestTrivialWithPayload() {
	v := goDefaultHeaders.Clone()
	v["Content-Length"] = []string{"4"}

	s.handlerMock.
		On("ServeHTTP", http.MethodPost, strings.TrimPrefix(s.srv.URL, "http://"), v, "/ping", []byte("ping")).
		Return(http.StatusOK).
		Once()

	c, err := New(spec{
		URL:      s.srv.URL + "/ping",
		Method:   "POST",
		Payload:  ptr.String("ping"),
		Tries:    1,
		Interval: th.Duration(1 * time.Second),
		Timeout:  th.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	err = c.Check(context.TODO())
	s.Require().NoError(err)
}

func (s *http2xxTestSuite) TestFiveTries() {
	s.handlerMock.
		On("ServeHTTP", http.MethodGet, strings.TrimPrefix(s.srv.URL, "http://"), goDefaultHeaders.Clone(), "/ping", []byte(nil)).
		Return(http.StatusOK).
		Once()

	c, err := New(spec{
		URL:      s.srv.URL + "/ping",
		Method:   "GET",
		Tries:    5,
		Interval: th.Duration(1 * time.Second),
		Timeout:  th.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	err = c.Check(context.TODO())
	s.Require().NoError(err)
}

func (s *http2xxTestSuite) TestSuccessFromThirdTime() {
	s.handlerMock.
		On("ServeHTTP", http.MethodGet, strings.TrimPrefix(s.srv.URL, "http://"), goDefaultHeaders.Clone(), "/ping", []byte(nil)).
		Return(http.StatusServiceUnavailable).
		Once()
	s.handlerMock.
		On("ServeHTTP", http.MethodGet, strings.TrimPrefix(s.srv.URL, "http://"), goDefaultHeaders.Clone(), "/ping", []byte(nil)).
		Return(http.StatusServiceUnavailable).
		Once()
	s.handlerMock.
		On("ServeHTTP", http.MethodGet, strings.TrimPrefix(s.srv.URL, "http://"), goDefaultHeaders.Clone(), "/ping", []byte(nil)).
		Return(http.StatusOK).
		Once()

	c, err := New(spec{
		URL:      s.srv.URL + "/ping",
		Method:   "GET",
		Tries:    3,
		Interval: th.Duration(1 * time.Millisecond),
		Timeout:  th.Duration(5 * time.Second),
	})
	s.Require().NoError(err)

	err = c.Check(context.TODO())
	s.Require().NoError(err)
}

func (s *http2xxTestSuite) TestNegative() {
	s.handlerMock.
		On("ServeHTTP", http.MethodGet, strings.TrimPrefix(s.srv.URL, "http://"), goDefaultHeaders.Clone(), "/ping", []byte(nil)).
		Return(http.StatusServiceUnavailable).
		Twice()

	c, err := New(spec{
		URL:      s.srv.URL + "/ping",
		Method:   "GET",
		Tries:    2,
		Interval: th.Duration(1 * time.Millisecond),
		Timeout:  th.Duration(5 * time.Second),
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
	var (
		payload []byte = nil
		err     error
	)

	if r.Header.Get("Content-Length") != "" && r.Header.Get("Content-Length") != "0" {
		payload, err = io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		defer func() { _ = r.Body.Close() }()
	}

	args := m.Called(r.Method, r.Host, r.Header, r.URL.Path, payload)

	rw.WriteHeader(args.Int(0))
}
