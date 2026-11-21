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
	// RemotePort is the TCP port the engine dials on the peer. 0 (the zero
	// value) means the BGP default of 179. It is useful for testing against a
	// peer listening on an ephemeral port, and is a legitimate BGP feature for
	// non-standard session ports.
	RemotePort uint16
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

// BfdState is an engine-agnostic snapshot of a peer's BFD session. It carries
// the full set of fields GoBGP v4 exposes for a BFD session. For engine
// versions without BFD support (v3), the whole value is the zero struct and
// Enabled stays false.
type BfdState struct {
	// Enabled reports whether the peer has a BFD session configured (GoBGP v4
	// only; always false for v3, which has no BFD support). All other fields
	// are only meaningful when Enabled is true.
	Enabled bool
	// State is the normalized local BFD session state: 0=down, 1=up. It
	// carries the legacy BfdState value, so the peer_bfd_state metric is
	// unchanged.
	State uint32
	// RemoteState is the normalized remote BFD session state: 0=down, 1=up.
	RemoteState uint32
	// FailureTransitions counts the number of times the BFD session has
	// transitioned to a failed state (monotonic).
	FailureTransitions uint64
	// LastFailureTime is the unix time (seconds) of the last BFD failure.
	LastFailureTime uint64
	// LocalDiagnostic is the local BFD diagnostic code (RFC 5880).
	LocalDiagnostic uint32
	// RemoteDiagnostic is the remote BFD diagnostic code (RFC 5880).
	RemoteDiagnostic uint32
	// LocalDiscriminator is the BFD discriminator of this endpoint.
	LocalDiscriminator uint32
	// RemoteDiscriminator is the BFD discriminator of the remote endpoint.
	RemoteDiscriminator uint32
	// RemoteMinRxInterval is the remote minimum receive interval in
	// microseconds.
	RemoteMinRxInterval uint32
	// LastPacketTransmitted is the unix time (seconds) the last BFD packet was
	// transmitted.
	LastPacketTransmitted uint64
	// LastPacketReceived is the unix time (seconds) the last BFD packet was
	// received.
	LastPacketReceived uint64
	// TransmittedPackets is the number of BFD packets transmitted (monotonic).
	TransmittedPackets uint64
	// ReceivedPackets is the number of BFD packets received (monotonic).
	ReceivedPackets uint64
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
	// Bfd carries the peer's BFD session state. Bfd.Enabled is the legacy
	// BfdEnabled flag and Bfd.State the legacy BfdState value, so the
	// peer_bfd_state metric is unchanged.
	Bfd BfdState
	// RoutesReceived / RoutesAccepted / RoutesAdvertised are the route counts
	// summed across the peer's enabled address families. Uptime is the unix
	// time (seconds) at which the BGP session last came up. RemoteASN / LocalASN
	// are the peer's and this router's AS numbers for the session.
	RoutesReceived   uint64
	RoutesAccepted   uint64
	RoutesAdvertised uint64
	Uptime           uint64
	RemoteASN        uint32
	LocalASN         uint32
	Received         MessageCounter
	Sent             MessageCounter
}

// PeerSessionEstablished is the SessionState value of an ESTABLISHED BGP
// session, as normalized into PeerState.SessionState (GoBGP's ESTABLISHED
// state).
const PeerSessionEstablished uint32 = 6

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
