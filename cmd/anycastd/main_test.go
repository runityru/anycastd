package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	th "github.com/teran/go-time"

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

func TestDriverSpecBfd(t *testing.T) {
	port := uint16(4784)
	tx := th.Duration(200 * time.Millisecond)
	rx := th.Duration(300 * time.Millisecond)
	mult := uint32(3)

	a := config.Announcer{
		RouterID: "192.0.2.1",
		LocalASN: 64512,
		Bfd: config.Bfd{
			Enabled:       true,
			Port:          &port,
			MinTxInterval: &tx,
			MinRxInterval: &rx,
			Multiplier:    &mult,
		},
	}

	spec := driverSpec(a)

	require.True(t, spec.Bfd.Enabled)
	require.Equal(t, &port, spec.Bfd.Port)
	require.Equal(t, &tx, spec.Bfd.DesiredMinTxInterval)
	require.Equal(t, &rx, spec.Bfd.RequiredMinReceive)
	require.Equal(t, &mult, spec.Bfd.DetectionMultiplier)
}

func TestDriverSpecBfdDefaults(t *testing.T) {
	a := config.Announcer{
		RouterID: "192.0.2.1",
		LocalASN: 64512,
	}

	spec := driverSpec(a)

	// BFD is off by default and all fields map through as-is (nil = unset).
	require.False(t, spec.Bfd.Enabled)
	require.Nil(t, spec.Bfd.Port)
	require.Nil(t, spec.Bfd.DesiredMinTxInterval)
	require.Nil(t, spec.Bfd.RequiredMinReceive)
	require.Nil(t, spec.Bfd.DetectionMultiplier)
}
