package ntpq

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/beevik/ntp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	th "github.com/teran/go-time"

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

	tcs := []struct {
		name      string
		responses []*ntp.Response
		expError  string
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
				return o.LocalAddress == srcAddr && o.Timeout == 5*time.Second
			})
			for _, resp := range tc.responses {
				m.On("queryMock", server, opts).Return(resp, nil).Once()
			}

			ntpOffset.Reset()
			ntpRtt.Reset()

			err = c.Check(context.Background())
			if tc.expError == "" {
				r.NoError(err)
			} else {
				r.ErrorContains(err, tc.expError)
			}
			m.AssertExpectations(t)

			last := tc.responses[len(tc.responses)-1]
			r.Equal(1, testutil.CollectAndCount(ntpOffset, "anycastd_check_last_ntp_offset_seconds"))
			r.Equal(1, testutil.CollectAndCount(ntpRtt, "anycastd_check_last_ntp_rtt_seconds"))
			r.InDelta(last.ClockOffset.Seconds(), testutil.ToFloat64(ntpOffset.WithLabelValues(checkName, server, srcAddr)), 1e-9)
			r.InDelta(last.RTT.Seconds(), testutil.ToFloat64(ntpRtt.WithLabelValues(checkName, server, srcAddr)), 1e-9)
		})
	}
}

type mockNtp struct {
	mock.Mock
}

func (m *mockNtp) queryMock(server string, opts ntp.QueryOptions) (*ntp.Response, error) {
	args := m.Called(server, opts)
	return args.Get(0).(*ntp.Response), args.Error(1)
}
