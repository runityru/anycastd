package announcer

import (
	"context"
)

// GoBGPServer is the interface for a BGP server that can announce and
// withdraw routes. Implementations handle the low-level details of
// interacting with a specific BGP implementation (e.g. GoBGP v3, v4).
type GoBGPServer interface {
	// AddPath announces prefix in its own address family, with each session's
	// local address as the next hop.
	AddPath(ctx context.Context, prefix string) error
	DeletePath(ctx context.Context, prefix string) error
}

type Announcer interface {
	Announce(ctx context.Context) error
	Denounce(ctx context.Context) error
}

type Config struct {
	GoBGP    GoBGPServer
	Prefixes []string
}

type announcer struct {
	gobgp    GoBGPServer
	prefixes []string
}

func New(cfg Config) Announcer {
	return &announcer{
		gobgp:    cfg.GoBGP,
		prefixes: cfg.Prefixes,
	}
}

func (a *announcer) Announce(ctx context.Context) error {
	for _, prefix := range a.prefixes {
		if err := a.gobgp.AddPath(ctx, prefix); err != nil {
			return err
		}
	}

	return nil
}

func (a *announcer) Denounce(ctx context.Context) error {
	for _, prefix := range a.prefixes {
		if err := a.gobgp.DeletePath(ctx, prefix); err != nil {
			return err
		}
	}
	return nil
}
