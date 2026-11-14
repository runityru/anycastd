package announcer

import (
	"context"
	"io"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// fakePeerStateSource is a hand-written stub PeerStateSource returning a fixed
// set of peer states.
type fakePeerStateSource struct {
	states []PeerState
}

func (f fakePeerStateSource) ListPeers(ctx context.Context) ([]PeerState, error) {
	return f.states, nil
}

// peer returns a PeerState whose every field carries a DISTINCT non-zero value
// (except AdminState, normalized to 0=up) so any wrong field mapping fails.
func peer(neighbor string, passwordSet bool) PeerState {
	return PeerState{
		NeighborAddress: neighbor,
		AdminState:      0,
		SessionState:    6,
		Type:            1,
		OutQ:            2,
		Flops:           3,
		SendCommunity:   4,
		RemovePrivate:   5,
		PasswordSet:     passwordSet,
		Received: MessageCounter{
			Total: 11, Notification: 12, Update: 13, Open: 14, Keepalive: 15,
			Refresh: 16, WithdrawUpdate: 17, WithdrawPrefix: 18,
		},
		Sent: MessageCounter{
			Total: 21, Notification: 22, Update: 23, Open: 24, Keepalive: 25,
			Refresh: 26, WithdrawUpdate: 27, WithdrawPrefix: 28,
		},
	}
}

func newTestMetrics(t *testing.T, src PeerStateSource) *metrics {
	t.Helper()

	reg := prometheus.NewRegistry()
	lg := log.New()
	lg.SetOutput(io.Discard)

	m, ok := NewMetricsRepository(src, "10.9.9.9", reg, lg).(*metrics)
	require.True(t, ok, "NewMetricsRepository should return *metrics")
	require.NoError(t, m.Register())

	return m
}

// TestMetricsRepositoryAllGauges asserts that every one of the 25 gauges is
// populated from the correct PeerState field. The repository registers into a
// fresh, hermetic registry and collect() runs synchronously for determinism.
func TestMetricsRepositoryAllGauges(t *testing.T) {
	r := require.New(t)

	src := fakePeerStateSource{states: []PeerState{peer("10.0.0.1", true)}}
	m := newTestMetrics(t, src)

	m.collect(context.Background())

	cases := []struct {
		name   string
		metric func() prometheus.Collector
		want   float64
	}{
		{"peer_count", func() prometheus.Collector { return m.peerCount }, 1},

		{"peer_admin_state", func() prometheus.Collector { return m.peerAdminState }, 0},
		{"peer_session_state", func() prometheus.Collector { return m.peerSessionState }, 6},
		{"peer_out_queue_count", func() prometheus.Collector { return m.bgpPeerOutQueueCount }, 2},
		{"peer_flops_count", func() prometheus.Collector { return m.bgpPeerFlopsCount }, 3},
		{"peer_send_community_flag", func() prometheus.Collector { return m.bgpPeerSendCommunityFlag }, 4},
		{"peer_remove_private_flag", func() prometheus.Collector { return m.bgpPeerRemovePrivateFlag }, 5},
		{"peer_password_set_flag", func() prometheus.Collector { return m.bgpPeerPasswordSetFlag }, 1},
		{"peer_type", func() prometheus.Collector { return m.bgpPeerType }, 1},

		{"received_messages_total", func() prometheus.Collector { return m.receivedMessagesTotal }, 11},
		{"received_messages_notification", func() prometheus.Collector { return m.receivedMessagesNotification }, 12},
		{"received_messages_update", func() prometheus.Collector { return m.receivedMessagesUpdate }, 13},
		{"received_messages_open", func() prometheus.Collector { return m.receivedMessagesOpen }, 14},
		{"received_messages_keepalive", func() prometheus.Collector { return m.receivedMessagesKeepalive }, 15},
		{"received_messages_refresh", func() prometheus.Collector { return m.receivedMessagesRefresh }, 16},
		{"received_messages_withdraw_update", func() prometheus.Collector { return m.receivedMessagesWithdrawUpdate }, 17},
		{"received_messages_withdraw_prefix", func() prometheus.Collector { return m.receivedMessagesWithdrawPrefix }, 18},

		{"sent_messages_total", func() prometheus.Collector { return m.sentMessagesTotal }, 21},
		{"sent_messages_notification", func() prometheus.Collector { return m.sentMessagesNotification }, 22},
		{"sent_messages_update", func() prometheus.Collector { return m.sentMessagesUpdate }, 23},
		{"sent_messages_open", func() prometheus.Collector { return m.sentMessagesOpen }, 24},
		{"sent_messages_keepalive", func() prometheus.Collector { return m.sentMessagesKeepalive }, 25},
		{"sent_messages_refresh", func() prometheus.Collector { return m.sentMessagesRefresh }, 26},
		{"sent_messages_withdraw_update", func() prometheus.Collector { return m.sentMessagesWithdrawUpdate }, 27},
		{"sent_messages_withdraw_prefix", func() prometheus.Collector { return m.sentMessagesWithdrawPrefix }, 28},
	}

	r.Len(cases, 25, "must assert exactly 25 gauges")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, testutil.ToFloat64(c.metric()))
		})
	}
}

// TestMetricsRepositoryPasswordUnset covers the PasswordSet=false → 0.0 branch.
func TestMetricsRepositoryPasswordUnset(t *testing.T) {
	r := require.New(t)

	src := fakePeerStateSource{states: []PeerState{peer("10.0.0.2", false)}}
	m := newTestMetrics(t, src)

	m.collect(context.Background())

	r.Equal(float64(0), testutil.ToFloat64(m.bgpPeerPasswordSetFlag.WithLabelValues("10.9.9.9", "10.0.0.2")))
}
