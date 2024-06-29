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
	LocalASN uint32
}

type announcer struct {
	gobgp    GoBGPServer
	prefixes []string
	nextHop  string
	localASN uint32
}

func New(cfg Config) Announcer {
	return &announcer{
		gobgp:    cfg.GoBGP,
		prefixes: cfg.Prefixes,
		nextHop:  cfg.NextHop,
		localASN: cfg.LocalASN,
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

		nlri, err := apb.New(&api.IPAddressPrefix{
			Prefix:    l[0],
			PrefixLen: uint32(prefixLen),
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
			NextHop: a.nextHop,
		})
		if err != nil {
			return nil, err
		}

		attrs := []*apb.Any{a1, a2}

		prefixes = append(prefixes, &api.Path{
			Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
			Nlri:   nlri,
			Pattrs: attrs,
		})
	}

	return prefixes, nil
}
