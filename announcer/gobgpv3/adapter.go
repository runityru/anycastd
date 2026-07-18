// Package gobgpv3 provides a GoBGP v3 implementation of the announcer.GoBGPServer interface.
package gobgpv3

import (
	"context"
	"net/netip"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	apb "google.golang.org/protobuf/types/known/anypb"

	"github.com/runityru/anycastd/announcer"
)

type adapter struct {
	srv *server.BgpServer
}

// NewAdapter wraps a GoBGP v3 *server.BgpServer into the
// application-level announcer.GoBGPServer interface, isolating all
// GoBGP-specific types from the rest of the code.
func NewAdapter(srv *server.BgpServer) announcer.GoBGPServer {
	return &adapter{srv: srv}
}

func (a *adapter) AddPath(ctx context.Context, prefix, nextHop string) error {
	p, err := newPath(prefix, nextHop)
	if err != nil {
		return err
	}

	_, err = a.srv.AddPath(ctx, &api.AddPathRequest{
		Path: p,
	})
	return err
}

func (a *adapter) DeletePath(ctx context.Context, prefix string) error {
	// For deletion in v3 we only need the NLRI part. Pass an empty nextHop
	// since the attribute is ignored during withdrawal.
	p, err := newPath(prefix, "")
	if err != nil {
		return err
	}

	return a.srv.DeletePath(ctx, &api.DeletePathRequest{
		Path: p,
	})
}

// newPath builds a GoBGP v3 *api.Path for the given prefix and nextHop.
func newPath(prefix, nextHop string) (*api.Path, error) {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return nil, err
	}

	nlri, err := apb.New(&api.IPAddressPrefix{
		Prefix:    p.Addr().String(),
		PrefixLen: uint32(p.Bits()),
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

	attrs := []*apb.Any{a1}
	if nextHop != "" {
		nh, err := netip.ParseAddr(nextHop)
		if err != nil {
			return nil, err
		}

		a2, err := apb.New(&api.NextHopAttribute{
			NextHop: nh.String(),
		})
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, a2)
	}

	return &api.Path{
		Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
		Nlri:   nlri,
		Pattrs: attrs,
	}, nil
}
