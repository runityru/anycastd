package gobgpv3

import (
	"context"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/runityru/anycastd/announcer"
)

// peerStateSource adapts a GoBGP v3 *server.BgpServer into the
// application-level announcer.PeerStateSource interface.
type peerStateSource struct {
	srv    *server.BgpServer
	logger *log.Logger
}

// NewPeerStateSource wraps a GoBGP v3 *server.BgpServer into the
// application-level announcer.PeerStateSource interface.
func NewPeerStateSource(srv *server.BgpServer, logger *log.Logger) announcer.PeerStateSource {
	return &peerStateSource{srv: srv, logger: logger}
}

func (s *peerStateSource) ListPeers(ctx context.Context) ([]announcer.PeerState, error) {
	out := []announcer.PeerState{}
	if err := s.srv.ListPeer(ctx, &api.ListPeerRequest{}, func(p *api.Peer) {
		ps, err := peerStateFromV3(p)
		if err != nil {
			// A single peer failing to map (e.g. a negative enum value) must
			// not discard the remaining peers nor abort the metrics loop: log
			// and skip just this peer.
			s.logger.WithError(err).Warnf("skipping peer %q: %v", p.GetConf().GetNeighborAddress(), err)
			return
		}
		out = append(out, ps)
	}); err != nil {
		// Only a whole-call failure of the underlying ListPeer is fatal.
		return nil, err
	}

	return out, nil
}

// peerStateFromV3 maps a GoBGP v3 *api.Peer onto the engine-agnostic
// announcer.PeerState. GoBGP v3 already normalizes AdminState to 0/1/2, so it
// is passed through unchanged.
func peerStateFromV3(p *api.Peer) (announcer.PeerState, error) {
	state := p.GetState()

	adminState, err := enumToUint32("admin state", int32(state.GetAdminState()))
	if err != nil {
		return announcer.PeerState{}, err
	}
	sessionState, err := enumToUint32("session state", int32(state.GetSessionState()))
	if err != nil {
		return announcer.PeerState{}, err
	}
	peerType, err := enumToUint32("peer type", int32(state.GetType()))
	if err != nil {
		return announcer.PeerState{}, err
	}
	removePrivate, err := enumToUint32("remove private", int32(state.GetRemovePrivate()))
	if err != nil {
		return announcer.PeerState{}, err
	}

	return announcer.PeerState{
		NeighborAddress: p.GetConf().GetNeighborAddress(),
		AdminState:      adminState,
		SessionState:    sessionState,
		Type:            peerType,
		OutQ:            state.GetOutQ(),
		Flops:           state.GetFlops(),
		SendCommunity:   state.GetSendCommunity(),
		RemovePrivate:   removePrivate,
		PasswordSet:     state.GetAuthPassword() != "",
		Received:        messageCounterFromV3(state.GetMessages().GetReceived()),
		Sent:            messageCounterFromV3(state.GetMessages().GetSent()),
	}, nil
}

// enumToUint32 converts an int32 enum value to uint32, rejecting negative
// values (gosec G115) that protobuf enums could theoretically carry on the wire.
func enumToUint32(name string, v int32) (uint32, error) {
	if v < 0 {
		return 0, errors.Errorf("%s has unexpected negative value: %d", name, v)
	}
	return uint32(v), nil
}

// messageCounterFromV3 maps a GoBGP v3 *api.Message onto
// announcer.MessageCounter, treating a nil message (no traffic yet) as zeros.
func messageCounterFromV3(m *api.Message) announcer.MessageCounter {
	if m == nil {
		return announcer.MessageCounter{}
	}

	return announcer.MessageCounter{
		Total:          m.GetTotal(),
		Notification:   m.GetNotification(),
		Update:         m.GetUpdate(),
		Open:           m.GetOpen(),
		Keepalive:      m.GetKeepalive(),
		Refresh:        m.GetRefresh(),
		WithdrawUpdate: m.GetWithdrawUpdate(),
		WithdrawPrefix: m.GetWithdrawPrefix(),
	}
}
