// Package gobgpv3 provides a GoBGP v3 implementation of the announcer.BGPServer interface.
package gobgpv3

import (
	"context"
	"net/netip"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	"github.com/pkg/errors"
	apb "google.golang.org/protobuf/types/known/anypb"

	"github.com/runityru/anycastd/announcer"
)

type adapter struct {
	srv *server.BgpServer
}

// NewAdapter wraps a GoBGP v3 *server.BgpServer into the
// application-level announcer.BGPServer interface, isolating all
// GoBGP-specific types from the rest of the code.
func NewAdapter(srv *server.BgpServer) announcer.BGPServer {
	return &adapter{srv: srv}
}

func (a *adapter) AddPath(ctx context.Context, prefix string) error {
	p, err := newPath(prefix)
	if err != nil {
		return err
	}

	_, err = a.srv.AddPath(ctx, &api.AddPathRequest{
		Path: p,
	})
	return err
}

func (a *adapter) DeletePath(ctx context.Context, prefix string) error {
	p, err := newPath(prefix)
	if err != nil {
		return err
	}

	return a.srv.DeletePath(ctx, &api.DeletePathRequest{
		Path: p,
	})
}

// newPath builds a GoBGP v3 unicast *api.Path for the given prefix with the
// unspecified next hop, which GoBGP replaces with each session's local address.
func newPath(prefix string) (*api.Path, error) {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return nil, err
	}

	afi, nextHop := api.Family_AFI_IP, netip.IPv4Unspecified()
	if p.Addr().Is6() {
		afi, nextHop = api.Family_AFI_IP6, netip.IPv6Unspecified()
	}

	// p is a validated netip.Prefix, so Bits() is always in [0,128]; guard it
	// explicitly to keep the int->uint32 conversion provably safe (gosec G115).
	bits := p.Bits()
	if bits < 0 || bits > 128 {
		return nil, errors.Errorf("invalid prefix length: %d", bits)
	}

	nlri, err := apb.New(&api.IPAddressPrefix{
		Prefix:    p.Addr().String(),
		PrefixLen: uint32(bits),
	})
	if err != nil {
		return nil, err
	}

	a1, err := apb.New(&api.OriginAttribute{
		Origin: 0,
	})
	if err != nil {
		return nil, err
	}

	a2, err := apb.New(&api.NextHopAttribute{
		NextHop: nextHop.String(),
	})
	if err != nil {
		return nil, err
	}

	return &api.Path{
		Family: &api.Family{Afi: afi, Safi: api.Family_SAFI_UNICAST},
		Nlri:   nlri,
		Pattrs: []*apb.Any{a1, a2},
	}, nil
}
