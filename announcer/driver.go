package announcer

import (
	"context"

	th "github.com/teran/go-time"
)

// BfdSpec configures Bidirectional Forwarding Detection (BFD) towards every BGP
// peer. Enabled is the on/off switch (false by default). A nil field means
// "unset", letting the engine apply its own default. Multihop mode is derived
// per-peer from PeerSpec.EnableMultihop, never set here.
type BfdSpec struct {
	Enabled              bool
	Port                 *uint16
	DesiredMinTxInterval *th.Duration
	RequiredMinReceive   *th.Duration
	DetectionMultiplier  *uint32
}

// DriverSpec is the engine-agnostic description of a BGP instance. It carries
// everything a concrete GoBGP implementation (v3, and later v4) needs to start
// a server and wire up its peers, without leaking engine-specific types.
type DriverSpec struct {
	RouterID string
	LocalASN uint32
	Peers    []PeerSpec
	Bfd      BfdSpec // value; Enabled=false by default = the on/off switch
}

// PeerSpec is the engine-agnostic description of a single BGP peer session.
// LocalAddress is the family-appropriate local source address ("" lets the
// engine fall back to the kernel route-lookup source).
type PeerSpec struct {
	Name           string
	RemoteAddress  string
	RemoteASN      uint32
	EnableMultihop bool
	MultihopTTL    uint32
	LocalAddress   string
}

// MessageCounter counts messages exchanged with a peer.
type MessageCounter struct {
	Total          uint64
	Notification   uint64
	Update         uint64
	Open           uint64
	Keepalive      uint64
	Refresh        uint64
	WithdrawUpdate uint64
	WithdrawPrefix uint64
}

// PeerState is an engine-agnostic snapshot of a peer session. AdminState is
// normalized to 0=up, 1=down, 2=pfx_ct across engine versions.
type PeerState struct {
	NeighborAddress string
	AdminState      uint32
	SessionState    uint32
	Type            uint32
	OutQ            uint32
	Flops           uint32
	SendCommunity   uint32
	RemovePrivate   uint32
	PasswordSet     bool
	// BfdEnabled reports whether the peer has a BFD session configured
	// (GoBGP v4 only; always false for v3, which has no BFD support).
	// BfdState is the normalized BFD session state: 0=down, 1=up. Both are
	// only meaningful when BfdEnabled is true.
	BfdEnabled bool
	BfdState   uint32
	Received   MessageCounter
	Sent       MessageCounter
}

// PeerStateSource lists the current state of every configured peer.
type PeerStateSource interface {
	ListPeers(ctx context.Context) ([]PeerState, error)
}

// ServerLifecycle starts and stops a BGP server.
type ServerLifecycle interface {
	Serve() error
	Start(ctx context.Context, spec DriverSpec) error
	Stop(ctx context.Context) error
}

// PeerManager adds BGP peers to a running server.
type PeerManager interface {
	AddPeer(ctx context.Context, peer PeerSpec) error
}

// Driver is the full engine-agnostic BGP driver: it manages the server
// lifecycle, its peers, exposes peer state, and announces/withdraws routes.
type Driver interface {
	ServerLifecycle
	PeerManager
	PeerStateSource
	BGPServer
}

// Capabilities reports the engine-agnostic feature set of a BGP driver.
type Capabilities struct {
	SupportsBFD bool
}

// CapabilityProvider exposes the capabilities of a BGP engine. It is optional —
// deliberately separate from Driver so an engine can adopt it without breaking
// the Driver contract.
type CapabilityProvider interface {
	Capabilities() Capabilities
}
