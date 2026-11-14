package announcer_test

import (
	"context"
	"testing"

	apiv3 "github.com/osrg/gobgp/v3/api"
	v3 "github.com/osrg/gobgp/v3/pkg/server"
	apiv4 "github.com/osrg/gobgp/v4/api"
	v4 "github.com/osrg/gobgp/v4/pkg/server"
)

// TestGoBGPv3v4Coexist builds a GoBGP v3 and a GoBGP v4 server in the same
// process. Each engine links its own protobuf API and init-time registrations;
// this locks in that both versions coexist in one binary without an
// init/registration conflict.
func TestGoBGPv3v4Coexist(t *testing.T) {
	ctx := context.Background()

	s3 := v3.NewBgpServer()
	go s3.Serve()
	defer s3.Stop()
	if err := s3.StartBgp(ctx, &apiv3.StartBgpRequest{
		Global: &apiv3.Global{RouterId: "10.3.3.3", Asn: 65999, ListenPort: -1},
	}); err != nil {
		t.Fatalf("gobgpv3 start: %v", err)
	}

	s4 := v4.NewBgpServer()
	go s4.Serve()
	defer s4.Stop()
	if err := s4.StartBgp(ctx, &apiv4.StartBgpRequest{
		Global: &apiv4.Global{RouterId: "10.3.3.4", Asn: 65999, ListenPort: -1},
	}); err != nil {
		t.Fatalf("gobgpv4 start: %v", err)
	}
}
