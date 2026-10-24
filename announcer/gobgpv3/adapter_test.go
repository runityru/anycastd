package gobgpv3

import (
	"context"
	"testing"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	"github.com/stretchr/testify/require"
)

func TestAddDeletePath(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()

	srv := server.NewBgpServer()
	go srv.Serve()
	defer srv.Stop()
	r.NoError(srv.StartBgp(ctx, &api.StartBgpRequest{
		Global: &api.Global{RouterId: "10.3.3.3", Asn: 65999, ListenPort: -1},
	}))

	countPaths := func() int {
		n := 0
		r.NoError(srv.ListPath(ctx, &api.ListPathRequest{
			Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
		}, func(*api.Destination) { n++ }))
		return n
	}

	a := NewAdapter(srv)
	r.NoError(a.AddPath(ctx, "10.0.0.128/32", "10.0.0.1"))
	r.Equal(1, countPaths())

	r.NoError(a.DeletePath(ctx, "10.0.0.128/32"))
	r.Equal(0, countPaths())
}
