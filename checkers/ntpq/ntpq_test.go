package ntpq

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/beevik/ntp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	th "github.com/teran/go-time"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/runityru/anycastd/checkers"
)

func TestSpec(t *testing.T) {
	r := require.New(t)

	data, err := os.ReadFile("testdata/spec.json")
	r.NoError(err)

	c, err := checkers.NewCheckerByKind("ntpq", json.RawMessage(data))
	r.NoError(err)
	r.Equal("ntpq", c.Kind())

	n := c.(*ntpq)
	r.Equal("0.ru.pool.ntp.org", n.server)
	r.Equal("192.168.0.1", n.srcAddr)
	r.Equal(uint8(3), n.tries)
	r.Equal(125*time.Millisecond, n.offsetThreshold)
	r.Equal(100*time.Millisecond, n.interval)
	r.Equal(5*time.Second, n.timeout)
}

func TestCheck(t *testing.T) {
	now := time.Now()
	resp := func(offset time.Duration) *ntp.Response {
		return &ntp.Response{
			Version:       4,
			Stratum:       2,
			Time:          now,
			ReferenceTime: now.Add(-time.Minute),
			RTT:           15 * time.Millisecond,
			ClockOffset:   offset,
			ReferenceID:   1,
		}
	}
	ok := resp(10 * time.Millisecond)
	tooBig := resp(150 * time.Millisecond)
	negTooBig := resp(-150 * time.Millisecond)
	unsynced := resp(150 * time.Millisecond)
	unsynced.Stratum = 16
	notInSync := resp(150 * time.Millisecond)
	notInSync.Leap = ntp.LeapNotInSync
	kod := resp(150 * time.Millisecond)
	kod.Stratum = 0
	noReply := (*ntp.Response)(nil)

	tcs := []struct {
		name       string
		responses  []*ntp.Response
		expError   string
		unverified bool
	}{
		{name: "offset within threshold", responses: []*ntp.Response{ok}},
		{name: "negative offset within threshold", responses: []*ntp.Response{resp(-10 * time.Millisecond)}},
		{name: "offset equal to threshold", responses: []*ntp.Response{resp(125 * time.Millisecond)}},
		{
			name:      "offset too big",
			responses: []*ntp.Response{tooBig, tooBig, tooBig},
			expError:  "150ms exceeds 125ms: offset is too big",
		},
		{
			name:      "negative offset too big",
			responses: []*ntp.Response{negTooBig, negTooBig, negTooBig},
			expError:  "-150ms exceeds 125ms: offset is too big",
		},
		{name: "recovers on retry", responses: []*ntp.Response{tooBig, ok}},
		{
			name:      "unverified retry after offset failure",
			responses: []*ntp.Response{tooBig, kod},
			expError:  "150ms exceeds 125ms: offset is too big",
		},
		{
			name:      "no reply after offset failure",
			responses: []*ntp.Response{tooBig, noReply, noReply},
			expError:  "150ms exceeds 125ms: offset is too big",
		},
		{name: "unsynchronized reference", responses: []*ntp.Response{unsynced}, unverified: true},
		{name: "reference leap not in sync", responses: []*ntp.Response{notInSync}, unverified: true},
		{name: "kiss of death", responses: []*ntp.Response{kod}, unverified: true},
		{name: "no reply", responses: []*ntp.Response{noReply, noReply, noReply}, unverified: true},
	}

	const server, srcAddr = "pool.ntp.org", "192.168.0.1"
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			l, err := New(spec{
				Server:          server,
				SrcAddr:         srcAddr,
				Tries:           3,
				OffsetThreshold: th.Duration(125 * time.Millisecond),
				Interval:        th.Duration(time.Millisecond),
				Timeout:         th.Duration(5 * time.Second),
			})
			r.NoError(err)

			m := &mockNtp{}
			m.Test(t)
			c := l.(*ntpq)
			c.queryFn = m.queryMock

			opts := mock.MatchedBy(func(o ntp.QueryOptions) bool {
				return o.LocalAddress == srcAddr && o.Timeout == 5*time.Second && o.Dialer != nil
			})
			var verified *ntp.Response
			for _, resp := range tc.responses {
				var err error
				if resp == nil {
					err = errors.New("i/o timeout")
				} else if resp.Validate() == nil {
					verified = resp
				}
				m.On("queryMock", server, opts).Return(resp, err).Once()
			}

			ntpOffset.Reset()
			ntpRtt.Reset()
			ntpUnverified.Reset()
			// A value from an earlier run must not survive an unverified reply.
			ntpOffset.WithLabelValues(checkName, server, srcAddr).Set(1)
			ntpRtt.WithLabelValues(checkName, server, srcAddr).Set(1)

			err = c.Check(context.Background())
			if tc.expError == "" {
				r.NoError(err)
			} else {
				r.ErrorContains(err, tc.expError)
			}
			m.AssertExpectations(t)

			if tc.unverified {
				r.Equal(0, testutil.CollectAndCount(ntpOffset))
				r.Equal(0, testutil.CollectAndCount(ntpRtt))
				r.Equal(1.0, testutil.ToFloat64(ntpUnverified.WithLabelValues(checkName, server, srcAddr)))
				return
			}

			r.Equal(0, testutil.CollectAndCount(ntpUnverified))
			r.Equal(1, testutil.CollectAndCount(ntpOffset, "anycastd_check_last_ntp_offset_seconds"))
			r.Equal(1, testutil.CollectAndCount(ntpRtt, "anycastd_check_last_ntp_rtt_seconds"))
			r.InDelta(verified.ClockOffset.Seconds(), testutil.ToFloat64(ntpOffset.WithLabelValues(checkName, server, srcAddr)), 1e-9)
			r.InDelta(verified.RTT.Seconds(), testutil.ToFloat64(ntpRtt.WithLabelValues(checkName, server, srcAddr)), 1e-9)
		})
	}
}

func TestDialPicksSourceFamily(t *testing.T) {
	dnsAddr := fakeDNS(t)
	orig := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "udp", dnsAddr)
		},
	}
	t.Cleanup(func() { net.DefaultResolver = orig })

	for _, src := range []string{"127.0.0.1", "::1"} {
		t.Run(src, func(t *testing.T) {
			r := require.New(t)

			srv, err := net.ListenPacket("udp", net.JoinHostPort(src, "0"))
			if err != nil {
				t.Skipf("%s unavailable: %s", src, err)
			}
			t.Cleanup(func() { _ = srv.Close() })

			_, port, err := net.SplitHostPort(srv.LocalAddr().String())
			r.NoError(err)

			conn, err := dial(src, net.JoinHostPort("ntp.test.", port))
			r.NoError(err)
			t.Cleanup(func() { _ = conn.Close() })

			r.Equal(srv.LocalAddr().String(), conn.RemoteAddr().String())
			r.Equal(src, conn.LocalAddr().(*net.UDPAddr).IP.String())
		})
	}
}

// fakeDNS answers A queries with 127.0.0.1 and AAAA queries with ::1.
func fakeDNS(t *testing.T) string {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = pc.Close() })

	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			var m dnsmessage.Message
			if m.Unpack(buf[:n]) != nil || len(m.Questions) != 1 {
				continue
			}
			q := m.Questions[0]
			var body dnsmessage.ResourceBody = &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}}
			if q.Type == dnsmessage.TypeAAAA {
				body = &dnsmessage.AAAAResource{AAAA: [16]byte(net.IPv6loopback)}
			}
			m.Response = true
			m.Answers = []dnsmessage.Resource{{
				Header: dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: 60},
				Body:   body,
			}}
			if msg, err := m.Pack(); err == nil {
				_, _ = pc.WriteTo(msg, addr)
			}
		}
	}()

	return pc.LocalAddr().String()
}

type mockNtp struct {
	mock.Mock
}

func (m *mockNtp) queryMock(server string, opts ntp.QueryOptions) (*ntp.Response, error) {
	args := m.Called(server, opts)
	return args.Get(0).(*ntp.Response), args.Error(1)
}
