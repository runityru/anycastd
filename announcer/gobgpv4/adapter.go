// Package gobgpv4 provides a GoBGP v4 implementation of the announcer.GoBGPServer interface.
package gobgpv4

import (
	"context"
	"net/netip"

	"github.com/osrg/gobgp/v4/pkg/apiutil"
	"github.com/osrg/gobgp/v4/pkg/packet/bgp"
	"github.com/osrg/gobgp/v4/pkg/server"

	"github.com/runityru/anycastd/announcer"
)

type adapter struct {
	srv *server.BgpServer
}

// NewAdapter wraps a GoBGP v4 *server.BgpServer into the
// application-level announcer.BGPServer interface, isolating all
// GoBGP-specific types from the rest of the code.
func NewAdapter(srv *server.BgpServer) announcer.BGPServer {
	return &adapter{srv: srv}
}

func (a *adapter) AddPath(ctx context.Context, prefix string) error {
	p, err := newPath(prefix, false)
	if err != nil {
		return err
	}

	resp, err := a.srv.AddPath(apiutil.AddPathRequest{
		Paths: []*apiutil.Path{p},
	})
	if err != nil {
		return err
	}
	if len(resp) > 0 && resp[0].Error != nil {
		return resp[0].Error
	}

	return nil
}

func (a *adapter) DeletePath(ctx context.Context, prefix string) error {
	p, err := newPath(prefix, true)
	if err != nil {
		return err
	}

	return a.srv.DeletePath(apiutil.DeletePathRequest{
		Paths: []*apiutil.Path{p},
	})
}

// newPath builds a GoBGP v4 *apiutil.Path for the given prefix. For an add
// (non-withdrawal) path it carries the unspecified next hop, which GoBGP
// replaces with each session's local address; a withdrawal path must not carry
// a next-hop attribute (GoBGP v4 requires Withdrawal to be set and would
// otherwise fail with "nexthop not found").
func newPath(prefix string, withdraw bool) (*apiutil.Path, error) {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return nil, err
	}

	family := bgp.RF_IPv4_UC
	nextHop := netip.IPv4Unspecified()
	if p.Addr().Is6() {
		family = bgp.RF_IPv6_UC
		nextHop = netip.IPv6Unspecified()
	}

	// p is a validated netip.Prefix, so NewIPAddrPrefix never fails on an
	// invalid prefix; it only masks the address to the prefix length.
	nlri, err := bgp.NewIPAddrPrefix(p)
	if err != nil {
		return nil, err
	}

	attrs := []bgp.PathAttributeInterface{bgp.NewPathAttributeOrigin(0)}
	if !withdraw {
		nh, err := bgp.NewPathAttributeNextHop(nextHop)
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, nh)
	}

	return &apiutil.Path{
		Family:     family,
		Nlri:       nlri,
		Attrs:      attrs,
		Withdrawal: withdraw,
	}, nil
}
