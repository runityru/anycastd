package gobgpv4

import (
	"context"

	api "github.com/osrg/gobgp/v4/api"
	"github.com/osrg/gobgp/v4/pkg/server"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/runityru/anycastd/announcer"
)

// peerStateSource adapts a GoBGP v4 *server.BgpServer into the
// application-level announcer.PeerStateSource interface.
type peerStateSource struct {
	srv    *server.BgpServer
	logger *log.Logger
}

// NewPeerStateSource wraps a GoBGP v4 *server.BgpServer into the
// application-level announcer.PeerStateSource interface.
func NewPeerStateSource(srv *server.BgpServer, logger *log.Logger) announcer.PeerStateSource {
	return &peerStateSource{srv: srv, logger: logger}
}

func (s *peerStateSource) ListPeers(ctx context.Context) ([]announcer.PeerState, error) {
	out := []announcer.PeerState{}

	// Gather the BFD session state of every peer that has a BFD session
	// configured (GoBGP v4 only). ListBfdPeer only reports peers with an
	// active BFD config, so a peer's presence here is the "BFD enabled" signal.
	bfdStates := map[string]api.BfdSessionState{}
	s.srv.ListBfdPeer(ctx, func(peerAddress string, state *api.BfdPeerState) {
		if state == nil {
			return
		}
		bfdStates[peerAddress] = state.GetSessionState()
	})

	if err := s.srv.ListPeer(ctx, &api.ListPeerRequest{}, func(p *api.Peer) {
		ps, ok := s.mapPeer(p)
		if !ok {
			return
		}
		if bfdState, present := bfdStates[ps.NeighborAddress]; present {
			ps.BfdEnabled = true
			ps.BfdState = normalizeBfdState(bfdState)
		}
		out = append(out, ps)
	}); err != nil {
		// Only a whole-call failure of the underlying ListPeer is fatal.
		return nil, err
	}

	return out, nil
}

// mapPeer maps a single v4 *api.Peer onto announcer.PeerState. A peer that
// fails to map (e.g. a negative enum value) is logged and skipped (ok=false)
// so that one bad peer does not discard the remaining peers nor abort the
// metrics loop.
func (s *peerStateSource) mapPeer(p *api.Peer) (announcer.PeerState, bool) {
	ps, err := peerStateFromV4(p)
	if err != nil {
		s.logger.WithError(err).Warnf("skipping peer %q: %v", p.GetConf().GetNeighborAddress(), err)
		return announcer.PeerState{}, false
	}
	return ps, true
}

// peerStateFromV4 maps a GoBGP v4 *api.Peer onto the engine-agnostic
// announcer.PeerState. AdminState and Type are normalized because v4 reserves 0
// for UNSPECIFIED and numbers UP/DOWN/PFX_CT as 1/2/3 and INTERNAL/EXTERNAL as
// 1/2, whereas the engine-agnostic PeerState uses 0=up, 1=down, 2=pfx_ct for
// AdminState and 0=internal, 1=external for Type.
func peerStateFromV4(p *api.Peer) (announcer.PeerState, error) {
	state := p.GetState()

	sessionState, err := enumToUint32("session state", int32(state.GetSessionState()))
	if err != nil {
		return announcer.PeerState{}, err
	}
	removePrivate, err := enumToUint32("remove private", int32(state.GetRemovePrivate()))
	if err != nil {
		return announcer.PeerState{}, err
	}

	return announcer.PeerState{
		NeighborAddress: p.GetConf().GetNeighborAddress(),
		AdminState:      normalizeAdminState(state.GetAdminState()),
		SessionState:    sessionState,
		Type:            normalizePeerType(state.GetType()),
		OutQ:            state.GetOutQ(),
		Flops:           state.GetFlops(),
		SendCommunity:   state.GetSendCommunity(),
		RemovePrivate:   removePrivate,
		PasswordSet:     state.GetAuthPassword() != "",
		Received:        messageCounterFromV4(state.GetMessages().GetReceived()),
		Sent:            messageCounterFromV4(state.GetMessages().GetSent()),
	}, nil
}

// normalizeAdminState maps GoBGP v4's PeerState_AdminState onto the
// engine-agnostic 0=up, 1=down, 2=pfx_ct encoding. v4's enum differs from v3:
// it reserves 0 for UNSPECIFIED and numbers UP=1, DOWN=2, PFX_CT=3. Any
// unknown value is treated as up (0).
func normalizeAdminState(v api.PeerState_AdminState) uint32 {
	switch v {
	case api.PeerState_ADMIN_STATE_DOWN:
		return 1
	case api.PeerState_ADMIN_STATE_PFX_CT:
		return 2
	default: // ADMIN_STATE_UP, ADMIN_STATE_UNSPECIFIED, or unknown
		return 0
	}
}

// normalizePeerType maps GoBGP v4's PeerType onto the engine-agnostic
// 0=internal, 1=external encoding, matching v3's enum (INTERNAL=0, EXTERNAL=1).
// v4's enum differs from v3: it reserves 0 for UNSPECIFIED and numbers
// INTERNAL=1, EXTERNAL=2. Any unknown value is treated as internal (0),
// mirroring v3 where an unset peer type defaults to internal.
func normalizePeerType(v api.PeerType) uint32 {
	switch v {
	case api.PeerType_PEER_TYPE_EXTERNAL:
		return 1
	default: // PEER_TYPE_INTERNAL, PEER_TYPE_UNSPECIFIED, or unknown
		return 0
	}
}

// enumToUint32 converts an int32 enum value to uint32, rejecting negative
// values (gosec G115) that protobuf enums could theoretically carry on the wire.
func enumToUint32(name string, v int32) (uint32, error) {
	if v < 0 {
		return 0, errors.Errorf("%s has unexpected negative value: %d", name, v)
	}
	return uint32(v), nil
}

// normalizeBfdState maps a GoBGP v4 api.BfdSessionState onto the engine-agnostic
// 0=down, 1=up encoding used by the peer_bfd_state metric. Only the UP state
// maps to up; every other state (down, admin-down, init, unspecified) is down.
func normalizeBfdState(v api.BfdSessionState) uint32 {
	if v == api.BfdSessionState_BFD_SESSION_STATE_UP {
		return 1
	}
	return 0
}

// messageCounterFromV4 maps a GoBGP v4 *api.Message onto
// announcer.MessageCounter, treating a nil message (no traffic yet) as zeros.
func messageCounterFromV4(m *api.Message) announcer.MessageCounter {
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
