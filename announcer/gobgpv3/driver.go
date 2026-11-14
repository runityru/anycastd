package gobgpv3

import (
	"context"

	apipb "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	log "github.com/sirupsen/logrus"

	"github.com/runityru/anycastd/announcer"
)

var _ announcer.Driver = (*driver)(nil)

// driver is the GoBGP v3 implementation of the engine-agnostic
// announcer.Driver. It embeds the path adapter and the peer-state source
// so it can serve simultaneously as an announcer.BGPServer and an
// announcer.PeerStateSource.
type driver struct {
	srv    *server.BgpServer
	logger *log.Logger
	adapter
	peerStateSource
}

// NewDriver constructs a GoBGP v3 driver for the given spec. For GoBGP v3 the
// server is built solely from the logrus bridge, so the spec is reserved for
// construction-time engine configuration in future engine versions; the spec's
// peers are applied by Start. The caller is responsible for running Serve
// (in a goroutine) before Start, since GoBGP's StartBgp/WatchEvent block until
// Serve runs.
func NewDriver(spec announcer.DriverSpec, logger *log.Logger) (announcer.Driver, error) {
	srv := server.NewBgpServer(server.LoggerOption(&announcer.Logger{Logger: logger}))
	return &driver{
		srv:             srv,
		logger:          logger,
		adapter:         adapter{srv: srv},
		peerStateSource: peerStateSource{srv: srv, logger: logger},
	}, nil
}

// Serve runs the GoBGP gRPC serve loop until the server is stopped.
func (d *driver) Serve() error {
	d.srv.Serve()
	return nil
}

// Start brings the BGP engine up: it starts the global instance, registers
// peer-state change logging, and applies the peers described by spec.
func (d *driver) Start(ctx context.Context, spec announcer.DriverSpec) error {
	if err := d.srv.StartBgp(ctx, &apipb.StartBgpRequest{
		Global: &apipb.Global{
			RouterId:   spec.RouterID,
			Asn:        spec.LocalASN,
			ListenPort: -1,
		},
	}); err != nil {
		return err
	}

	if err := d.srv.WatchEvent(context.Background(), &apipb.WatchEventRequest{
		Peer: &apipb.WatchEventRequest_Peer{},
	}, func(r *apipb.WatchEventResponse) {
		if p := r.GetPeer(); p != nil && p.Type == apipb.WatchEventResponse_PeerEvent_STATE {
			d.logger.Info(p)
		}
	}); err != nil {
		return err
	}

	for _, peer := range spec.Peers {
		if err := d.srv.AddPeer(ctx, &apipb.AddPeerRequest{
			Peer: newPeer(peer),
		}); err != nil {
			return err
		}
	}

	return nil
}

// Stop shuts the BGP engine down.
func (d *driver) Stop(ctx context.Context) error {
	return d.srv.StopBgp(ctx, &apipb.StopBgpRequest{})
}

// AddPeer adds a single peer to the running server.
func (d *driver) AddPeer(ctx context.Context, peer announcer.PeerSpec) error {
	return d.srv.AddPeer(ctx, &apipb.AddPeerRequest{
		Peer: newPeer(peer),
	})
}

// newPeer builds a GoBGP v3 peer definition sourced from the peer's resolved
// local address (already family-appropriate in PeerSpec.LocalAddress), or from
// the kernel route-lookup source when that is unset. AfiSafis stays empty, so
// GoBGP enables only the family of the peer's address.
func newPeer(peer announcer.PeerSpec) *apipb.Peer {
	return &apipb.Peer{
		Conf: &apipb.PeerConf{
			NeighborAddress: peer.RemoteAddress,
			PeerAsn:         peer.RemoteASN,
		},
		EbgpMultihop: &apipb.EbgpMultihop{
			Enabled:     peer.EnableMultihop,
			MultihopTtl: peer.MultihopTTL,
		},
		Transport: &apipb.Transport{
			LocalAddress: peer.LocalAddress,
		},
		// A bind to a tentative IPv6 address fails until DAD completes, and
		// GoBGP's default retry is 120 s.
		Timers: &apipb.Timers{Config: &apipb.TimersConfig{ConnectRetry: 5}},
	}
}
