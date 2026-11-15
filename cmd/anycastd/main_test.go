package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/runityru/anycastd/config"
)

func TestDriverSpec(t *testing.T) {
	a := config.Announcer{
		Engine:        "gobgpv3",
		RouterID:      "192.0.2.1",
		LocalASN:      64512,
		LocalAddress:  "10.0.0.1",
		LocalAddress6: "2001:db8::1",
		Peers: []config.Peer{
			{Name: "v4", RemoteAddress: "10.0.0.2", RemoteASN: 64513},
			{Name: "v6", RemoteAddress: "2001:db8::2", RemoteASN: 64513},
		},
	}

	spec := driverSpec(a)

	require.Equal(t, "192.0.2.1", spec.RouterID)
	require.Equal(t, uint32(64512), spec.LocalASN)
	require.Len(t, spec.Peers, 2)

	// IPv4 peer resolves to the IPv4 local address.
	require.Equal(t, "10.0.0.1", spec.Peers[0].LocalAddress)
	// IPv6 peer resolves to the IPv6 local address.
	require.Equal(t, "2001:db8::1", spec.Peers[1].LocalAddress)

	require.Equal(t, "v4", spec.Peers[0].Name)
	require.Equal(t, "10.0.0.2", spec.Peers[0].RemoteAddress)
	require.Equal(t, uint32(64513), spec.Peers[0].RemoteASN)
}

func TestDriverSpecUnsetLocalAddress(t *testing.T) {
	a := config.Announcer{
		RouterID: "192.0.2.1",
		LocalASN: 64512,
		Peers: []config.Peer{
			{Name: "v4", RemoteAddress: "10.0.0.2", RemoteASN: 64513},
			{Name: "v6", RemoteAddress: "2001:db8::2", RemoteASN: 64513},
		},
	}

	spec := driverSpec(a)

	// Unset/unspecified local addresses resolve to "" so the engine falls back
	// to the kernel route-lookup source (config.Announcer.LocalAddressFor).
	require.Equal(t, "", spec.Peers[0].LocalAddress)
	require.Equal(t, "", spec.Peers[1].LocalAddress)
}
