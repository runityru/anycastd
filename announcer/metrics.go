package announcer

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

type Metrics interface {
	Run(ctx context.Context) error
	Register() error
}

type metrics struct {
	peerCount        *prometheus.GaugeVec
	peerAdminState   *prometheus.GaugeVec
	peerSessionState *prometheus.GaugeVec

	receivedMessagesTotal          *prometheus.GaugeVec
	receivedMessagesNotification   *prometheus.GaugeVec
	receivedMessagesUpdate         *prometheus.GaugeVec
	receivedMessagesOpen           *prometheus.GaugeVec
	receivedMessagesKeepalive      *prometheus.GaugeVec
	receivedMessagesRefresh        *prometheus.GaugeVec
	receivedMessagesWithdrawUpdate *prometheus.GaugeVec
	receivedMessagesWithdrawPrefix *prometheus.GaugeVec

	sentMessagesTotal          *prometheus.GaugeVec
	sentMessagesNotification   *prometheus.GaugeVec
	sentMessagesUpdate         *prometheus.GaugeVec
	sentMessagesOpen           *prometheus.GaugeVec
	sentMessagesKeepalive      *prometheus.GaugeVec
	sentMessagesRefresh        *prometheus.GaugeVec
	sentMessagesWithdrawUpdate *prometheus.GaugeVec
	sentMessagesWithdrawPrefix *prometheus.GaugeVec

	bgpPeerOutQueueCount     *prometheus.GaugeVec
	bgpPeerFlopsCount        *prometheus.GaugeVec
	bgpPeerSendCommunityFlag *prometheus.GaugeVec
	bgpPeerRemovePrivateFlag *prometheus.GaugeVec
	bgpPeerPasswordSetFlag   *prometheus.GaugeVec
	bgpPeerType              *prometheus.GaugeVec

	src      PeerStateSource
	routerID string
	reg      prometheus.Registerer
	logger   *log.Logger
}

func NewMetricsRepository(src PeerStateSource, routerID string, reg prometheus.Registerer, logger *log.Logger) Metrics {
	m := &metrics{
		peerCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "peer_count",
				Help:      "Total amount of peers configured for the GoBGP instance",
			},
			[]string{"router_id"},
		),
		peerAdminState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "peer_admin_state",
				Help:      "Peer state 0=up, 1=down, 2=pfx_ct",
			},
			[]string{"router_id", "peer"},
		),
		peerSessionState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "peer_session_state",
				Help:      "Peer session state 0=unknown, 1=idle, 2=connect, 3=active, 4=opensent, 5=openconfirm, 6=established",
			},
			[]string{"router_id", "peer"},
		),

		receivedMessagesTotal: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "received_messages_total",
				Help:      "Total number of messages received from the peer",
			},
			[]string{"router_id", "peer"},
		),
		receivedMessagesNotification: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "received_messages_notification",
				Help:      "Number of Notification messages received from the peer",
			},
			[]string{"router_id", "peer"},
		),
		receivedMessagesUpdate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "received_messages_update",
				Help:      "Number of Update messages received from the peer",
			},
			[]string{"router_id", "peer"},
		),
		receivedMessagesOpen: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "received_messages_open",
				Help:      "Number of Open messages received from the peer",
			},
			[]string{"router_id", "peer"},
		),
		receivedMessagesKeepalive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "received_messages_keepalive",
				Help:      "Number of Keepalive messages received from the peer",
			},
			[]string{"router_id", "peer"},
		),
		receivedMessagesRefresh: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "received_messages_refresh",
				Help:      "Number of Refresh messages received from the peer",
			},
			[]string{"router_id", "peer"},
		),
		receivedMessagesWithdrawUpdate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "received_messages_withdraw_update",
				Help:      "Number of Withdraw Update messages received from the peer",
			},
			[]string{"router_id", "peer"},
		),
		receivedMessagesWithdrawPrefix: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "received_messages_withdraw_prefix",
				Help:      "Number of Withdraw Prefix messages received from the peer",
			},
			[]string{"router_id", "peer"},
		),

		sentMessagesTotal: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "sent_messages_total",
				Help:      "Total number of messages sent to the peer",
			},
			[]string{"router_id", "peer"},
		),
		sentMessagesNotification: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "sent_messages_notification",
				Help:      "Number of Notification messages sent to the peer",
			},
			[]string{"router_id", "peer"},
		),
		sentMessagesUpdate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "sent_messages_update",
				Help:      "Number of Update messages sent to the peer",
			},
			[]string{"router_id", "peer"},
		),
		sentMessagesOpen: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "sent_messages_open",
				Help:      "Number of Open messages sent to the peer",
			},
			[]string{"router_id", "peer"},
		),
		sentMessagesKeepalive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "sent_messages_keepalive",
				Help:      "Number of Keepalive messages sent to the peer",
			},
			[]string{"router_id", "peer"},
		),
		sentMessagesRefresh: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "sent_messages_refresh",
				Help:      "Number of Refresh messages sent to the peer",
			},
			[]string{"router_id", "peer"},
		),
		sentMessagesWithdrawUpdate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "sent_messages_withdraw_update",
				Help:      "Number of Withdraw Update messages sent to the peer",
			},
			[]string{"router_id", "peer"},
		),
		sentMessagesWithdrawPrefix: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "sent_messages_withdraw_prefix",
				Help:      "Number of Withdraw Prefix messages sent to the peer",
			},
			[]string{"router_id", "peer"},
		),

		bgpPeerOutQueueCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "peer_out_queue_count",
				Help:      "Peer outgoing messages queue",
			},
			[]string{"router_id", "peer"},
		),
		bgpPeerFlopsCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "peer_flops_count",
				Help:      "Peer flops count",
			},
			[]string{"router_id", "peer"},
		),
		bgpPeerSendCommunityFlag: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "peer_send_community_flag",
				Help:      "Whether the peer have send community flag set",
			},
			[]string{"router_id", "peer"},
		),
		bgpPeerRemovePrivateFlag: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "peer_remove_private_flag",
				Help:      "Whether the peer have remove private flag set",
			},
			[]string{"router_id", "peer"},
		),
		bgpPeerPasswordSetFlag: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "peer_password_set_flag",
				Help:      "Whether the peer have peer password set flag set",
			},
			[]string{"router_id", "peer"},
		),
		bgpPeerType: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "anycastd",
				Subsystem: "gobgp",
				Name:      "peer_type",
				Help:      "Peer type 0=internal, 1=external",
			},
			[]string{"router_id", "peer"},
		),

		src:      src,
		routerID: routerID,
		reg:      reg,
		logger:   logger,
	}
	return m
}

// collect fetches the current peer state and updates every gauge. A failure to
// list peers is logged and the loop continues with the previous values, so a
// transient error never aborts the metrics goroutine.
func (m *metrics) collect(ctx context.Context) {
	peers, err := m.src.ListPeers(ctx)
	if err != nil {
		m.logger.WithError(err).Error("failed to list peers; skipping metrics update")
		return
	}

	m.peerCount.WithLabelValues(m.routerID).Set(float64(len(peers)))

	for _, peer := range peers {
		peerRouterID := peer.NeighborAddress

		m.peerAdminState.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.AdminState))

		m.peerSessionState.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.SessionState))

		m.bgpPeerOutQueueCount.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.OutQ))

		m.bgpPeerFlopsCount.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Flops))

		m.bgpPeerSendCommunityFlag.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.SendCommunity))

		m.bgpPeerRemovePrivateFlag.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.RemovePrivate))

		m.bgpPeerPasswordSetFlag.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(boolToFloat64(peer.PasswordSet))

		m.bgpPeerType.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Type))

		m.receivedMessagesTotal.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Received.Total))

		m.receivedMessagesNotification.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Received.Notification))

		m.receivedMessagesUpdate.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Received.Update))

		m.receivedMessagesOpen.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Received.Open))

		m.receivedMessagesKeepalive.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Received.Keepalive))

		m.receivedMessagesRefresh.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Received.Refresh))

		m.receivedMessagesWithdrawUpdate.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Received.WithdrawUpdate))

		m.receivedMessagesWithdrawPrefix.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Received.WithdrawPrefix))

		m.sentMessagesTotal.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Sent.Total))

		m.sentMessagesNotification.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Sent.Notification))

		m.sentMessagesUpdate.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Sent.Update))

		m.sentMessagesOpen.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Sent.Open))

		m.sentMessagesKeepalive.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Sent.Keepalive))

		m.sentMessagesRefresh.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Sent.Refresh))

		m.sentMessagesWithdrawUpdate.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Sent.WithdrawUpdate))

		m.sentMessagesWithdrawPrefix.WithLabelValues(
			m.routerID,
			peerRouterID,
		).Set(float64(peer.Sent.WithdrawPrefix))
	}
}

// Run ticks once per second, refreshing all gauges, until the context is
// cancelled. It returns nil on shutdown so the errgroup in main exits cleanly
// (no panic on SIGTERM/SIGINT), and a transient ListPeers error never aborts
// the loop (collect logs and carries on).
func (m *metrics) Run(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.collect(ctx)
		}
	}
}

// boolToFloat64 converts a boolean into a 0.0/1.0 Prometheus gauge value.
func boolToFloat64(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func (m *metrics) Register() error {
	for _, mc := range []prometheus.Collector{
		m.peerCount,
		m.peerAdminState,
		m.peerSessionState,

		m.bgpPeerOutQueueCount,
		m.bgpPeerFlopsCount,
		m.bgpPeerSendCommunityFlag,
		m.bgpPeerRemovePrivateFlag,
		m.bgpPeerPasswordSetFlag,
		m.bgpPeerType,

		m.receivedMessagesTotal,
		m.receivedMessagesNotification,
		m.receivedMessagesUpdate,
		m.receivedMessagesOpen,
		m.receivedMessagesKeepalive,
		m.receivedMessagesRefresh,
		m.receivedMessagesWithdrawUpdate,
		m.receivedMessagesWithdrawPrefix,

		m.sentMessagesTotal,
		m.sentMessagesNotification,
		m.sentMessagesUpdate,
		m.sentMessagesOpen,
		m.sentMessagesKeepalive,
		m.sentMessagesRefresh,
		m.sentMessagesWithdrawUpdate,
		m.sentMessagesWithdrawPrefix,
	} {
		if err := m.reg.Register(mc); err != nil {
			return err
		}
	}

	return nil
}
