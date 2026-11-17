package http_2xx

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	th "github.com/teran/go-time"
)

// roundTripFunc adapts a function to the http.RoundTripper interface so tests
// can inject deterministic responses without a network round trip.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// trackingCloser records whether Close was called on a response body.
type trackingCloser struct {
	io.Reader
	closed bool
}

func (t *trackingCloser) Close() error {
	t.closed = true
	return nil
}

// TestCheckClosesResponseBody asserts that check() closes resp.Body on every
// return path (both 2xx and non-2xx). A body that is never closed leaks the
// connection/file descriptor and eventually exhausts the transport pool.
func TestCheckClosesResponseBody(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "success", statusCode: http.StatusOK, wantErr: false},
		{name: "non-2xx", statusCode: http.StatusServiceUnavailable, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			body := &trackingCloser{Reader: strings.NewReader("ok")}

			tr := roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tc.statusCode,
					Body:       body,
					Header:     make(http.Header),
				}, nil
			})

			h := &http_2xx{
				client: &http.Client{Transport: tr},
				url:    "http://example.com/ping",
				method: "GET",
			}

			err := h.check(context.TODO())
			if tc.wantErr {
				r.Error(err)
			} else {
				r.NoError(err)
			}

			r.True(body.closed, "response body must be closed on every return path")
		})
	}
}

// TestCheckReusesKeepAliveConnection verifies that check() drains and closes
// resp.Body so the client's keep-alive connection is reused across successive
// health checks. A custom http.Server with a ConnState hook counts distinct
// TCP connections the server ever sees: if the body is never drained+closed,
// the transport cannot reuse the connection and opens a fresh one each check,
// so connsCreated grows past 1 and the test fails.
func TestCheckReusesKeepAliveConnection(t *testing.T) {
	var (
		mu           sync.Mutex
		connsCreated int
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}),
		ConnState: func(_ net.Conn, cs http.ConnState) {
			if cs == http.StateNew {
				mu.Lock()
				connsCreated++
				mu.Unlock()
			}
		},
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	c, err := New(spec{
		URL:      "http://" + ln.Addr().String() + "/ping",
		Method:   "GET",
		Tries:    1,
		Interval: th.Duration(1 * time.Second),
		Timeout:  th.Duration(5 * time.Second),
	})
	require.NoError(t, err)

	// Three sequential health checks should ride a single keep-alive connection.
	for i := 0; i < 3; i++ {
		require.NoError(t, c.Check(context.TODO()))
	}

	mu.Lock()
	got := connsCreated
	mu.Unlock()

	require.Equal(t, 1, got,
		"expected the health-check to reuse a single keep-alive connection, got %d "+
			"connections created; resp.Body is likely not drained and closed (leak)", got)
}
