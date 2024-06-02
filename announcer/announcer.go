package announcer

import (
	"context"
	"strconv"
	"strings"

	api "github.com/osrg/gobgp/v3/api"
	apb "google.golang.org/protobuf/types/known/anypb"
)

type GoBGPServer interface {
	AddPath(ctx context.Context, r *api.AddPathRequest) (*api.AddPathResponse, error)
	DeletePath(ctx context.Context, r *api.DeletePathRequest) error
}

type Announcer interface {
	Announce(ctx context.Context) error
	Denounce(ctx context.Context) error
}

type Config struct {
	GoBGP    GoBGPServer
	Prefixes []string
	NextHop  string
}

type announcer struct {
	gobgp    GoBGPServer
	prefixes []string
	nexthop  string
}

func New(cfg Config) Announcer {
	return &announcer{
		gobgp:    cfg.GoBGP,
		prefixes: cfg.Prefixes,
		nexthop:  cfg.NextHop,
	}
}

func (a *announcer) Announce(ctx context.Context) error {
	pp, err := a.newPathList()
	if err != nil {
		return err
	}

	for _, p := range pp {
		_, err = a.gobgp.AddPath(ctx, &api.AddPathRequest{
			Path: p,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *announcer) Denounce(ctx context.Context) error {
	pp, err := a.newPathList()
	if err != nil {
		return err
	}

	for _, p := range pp {
		err := a.gobgp.DeletePath(ctx, &api.DeletePathRequest{
			Path: p,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *announcer) newPathList() ([]*api.Path, error) {
	prefixes := []*api.Path{}
	for _, p := range a.prefixes {
		l := strings.SplitN(p, "/", 2)
		prefixLen, err := strconv.ParseUint(l[1], 10, 32)
		if err != nil {
			return nil, err
		}

		nlri, _ := apb.New(&api.IPAddressPrefix{
			Prefix:    l[0],
			PrefixLen: uint32(prefixLen),
		})

		a1, _ := apb.New(&api.OriginAttribute{
			Origin: 0,
		})
		a2, _ := apb.New(&api.NextHopAttribute{
			NextHop: a.nexthop,
		})
		a3, _ := apb.New(&api.AsPathAttribute{
			Segments: []*api.AsSegment{
				{
					Type:    2,
					Numbers: []uint32{6762, 39919, 65000, 35753, 65000},
				},
			},
		})
		attrs := []*apb.Any{a1, a2, a3}

		prefixes = append(prefixes, &api.Path{
			Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
			Nlri:   nlri,
			Pattrs: attrs,
		})
	}

	return prefixes, nil
}
