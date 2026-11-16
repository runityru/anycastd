package gobgpv4

import (
	"context"
	"log/slog"
	"time"

	"github.com/osrg/gobgp/v4/api"
	"github.com/osrg/gobgp/v4/pkg/apiutil"
	"github.com/osrg/gobgp/v4/pkg/server"
	log "github.com/sirupsen/logrus"

	"github.com/runityru/anycastd/announcer"
)

var _ announcer.Driver = (*driver)(nil)
var _ announcer.CapabilityProvider = (*driver)(nil)

// driver is the GoBGP v4 implementation of the engine-agnostic
// announcer.Driver. It embeds the path adapter and the peer-state source
// so it can serve simultaneously as an announcer.BGPServer and an
// announcer.PeerStateSource.
type driver struct {
	srv    *server.BgpServer
	logger *slog.Logger
	bfd    announcer.BfdSpec
	adapter
	peerStateSource
}

// NewDriver constructs a GoBGP v4 driver for the given spec. GoBGP v4's server
// only accepts an *slog.Logger, so the logrus *log.Logger shared with the rest
// of the process (and the v3 engine) is bridged onto slog (see slogbridge.go),
// keeping both engines on the same logrus sink. The spec is reserved for
// construction-time engine configuration in future engine versions; the spec's
// peers are applied by Start. The caller is responsible for running Serve
// (in a goroutine) before Start, since GoBGP's StartBgp/WatchEvent block until
// Serve runs.
func NewDriver(spec announcer.DriverSpec, logger *log.Logger) (announcer.Driver, error) {
	slogger := slog.New(&slogBridge{logger: logger})
	srv := server.NewBgpServer(server.LoggerOption(slogger, nil))
	return &driver{
		srv:             srv,
		logger:          slogger,
		bfd:             spec.Bfd,
		adapter:         adapter{srv: srv},
		peerStateSource: peerStateSource{srv: srv, logger: logger},
	}, nil
}

// Serve runs the GoBGP gRPC serve loop until the server is stopped.
func (d *driver) Serve() error {
	d.srv.Serve()
	return nil
}

// Capabilities reports the feature set of the GoBGP v4 engine.
func (d *driver) Capabilities() announcer.Capabilities {
	return announcer.Capabilities{SupportsBFD: true}
}

// Start brings the BGP engine up: it starts the global instance, registers
// peer-state change logging, and applies the peers described by spec.
func (d *driver) Start(ctx context.Context, spec announcer.DriverSpec) error {
	if err := d.srv.StartBgp(ctx, &api.StartBgpRequest{
		Global: &api.Global{
			RouterId:   spec.RouterID,
			Asn:        spec.LocalASN,
			ListenPort: -1,
		},
	}); err != nil {
		return err
	}

	if err := d.srv.WatchEvent(context.Background(), server.WatchEventMessageCallbacks{
		OnPeerUpdate: func(e *apiutil.WatchEventMessage_PeerEvent, _ time.Time) {
			if e.Type == apiutil.PEER_EVENT_STATE {
				d.logger.Info("peer state changed", "peer", e.Peer)
			}
		},
	}, server.WatchPeer()); err != nil {
		return err
	}

	for _, peer := range spec.Peers {
		if err := d.srv.AddPeer(ctx, &api.AddPeerRequest{
			Peer: newPeer(peer, d.bfd),
		}); err != nil {
			return err
		}
	}

	return nil
}

// Stop shuts the BGP engine down.
func (d *driver) Stop(ctx context.Context) error {
	return d.srv.StopBgp(ctx, &api.StopBgpRequest{})
}

// AddPeer adds a single peer to the running server.
func (d *driver) AddPeer(ctx context.Context, peer announcer.PeerSpec) error {
	return d.srv.AddPeer(ctx, &api.AddPeerRequest{
		Peer: newPeer(peer, d.bfd),
	})
}

// newPeer builds a GoBGP v4 peer definition sourced from the peer's resolved
// local address (already family-appropriate in PeerSpec.LocalAddress), or from
// the kernel route-lookup source when that is unset. AfiSafis stays empty, so
// GoBGP enables only the family of the peer's address.
//
// When BFD is enabled the peer carries a *api.BfdPeerConfig. Note that a BFD
// session can only be established when the peer has a valid local address at
// AddPeer time, so callers enabling BFD should set PeerSpec.LocalAddress.
// Unset (nil) BfdSpec fields are left as 0, letting GoBGP apply its defaults
// (tx=rx=1s, multiplier 3, port 3784/4784).
func newPeer(peer announcer.PeerSpec, bfd announcer.BfdSpec) *api.Peer {
	p := &api.Peer{
		Conf: &api.PeerConf{
			NeighborAddress: peer.RemoteAddress,
			PeerAsn:         peer.RemoteASN,
		},
		EbgpMultihop: &api.EbgpMultihop{
			Enabled:     peer.EnableMultihop,
			MultihopTtl: peer.MultihopTTL,
		},
		Transport: &api.Transport{
			LocalAddress: peer.LocalAddress,
		},
		// A bind to a tentative IPv6 address fails until DAD completes, and
		// GoBGP's default retry is 120 s.
		Timers: &api.Timers{Config: &api.TimersConfig{ConnectRetry: 5}},
	}

	if !bfd.Enabled {
		return p
	}

	p.Bfd = &api.BfdPeerConfig{
		Enabled:  true,
		Multihop: peer.EnableMultihop,
	}
	if bfd.Port != nil {
		p.Bfd.Port = uint32(*bfd.Port)
	}
	if bfd.DesiredMinTxInterval != nil {
		// #nosec G115 -- value is capped by config.Bfd maxBfdInterval (uint32 µs limit).
		p.Bfd.DesiredMinimumTxInterval = uint32(bfd.DesiredMinTxInterval.TimeDuration().Microseconds())
	}
	if bfd.RequiredMinReceive != nil {
		// #nosec G115 -- value is capped by config.Bfd maxBfdInterval (uint32 µs limit).
		p.Bfd.RequiredMinimumReceive = uint32(bfd.RequiredMinReceive.TimeDuration().Microseconds())
	}
	if bfd.DetectionMultiplier != nil {
		p.Bfd.DetectionMultiplier = *bfd.DetectionMultiplier
	}

	return p
}
