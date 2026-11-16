package main

import (
	"io"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/runityru/anycastd/announcer"
)

func TestNewGoBGPDriver(t *testing.T) {
	r := require.New(t)
	logger := log.New()
	logger.SetOutput(io.Discard)
	spec := announcer.DriverSpec{RouterID: "10.3.3.3", LocalASN: 65999}

	t.Run("gobgpv3", func(t *testing.T) {
		d, err := newGoBGPDriver("gobgpv3", spec, logger)
		r.NoError(err)
		r.NotNil(d)
	})

	t.Run("gobgpv4", func(t *testing.T) {
		d, err := newGoBGPDriver("gobgpv4", spec, logger)
		r.NoError(err)
		r.NotNil(d)
	})

	t.Run("unknown engine errors", func(t *testing.T) {
		_, err := newGoBGPDriver("bogus", spec, logger)
		r.Error(err)
		r.Equal(`unknown BGP engine "bogus"`, err.Error())
	})

	t.Run("gobgpv4 with BFD enabled is allowed", func(t *testing.T) {
		bfd := spec
		bfd.Bfd = announcer.BfdSpec{Enabled: true}
		d, err := newGoBGPDriver("gobgpv4", bfd, logger)
		r.NoError(err)
		r.NotNil(d)
	})

	t.Run("gobgpv3 with BFD enabled fails closed", func(t *testing.T) {
		bfd := spec
		bfd.Bfd = announcer.BfdSpec{Enabled: true}
		_, err := newGoBGPDriver("gobgpv3", bfd, logger)
		r.Error(err)
	})
}

type fakeCapDriver struct{ supports bool }

func (f fakeCapDriver) Capabilities() announcer.Capabilities {
	return announcer.Capabilities{SupportsBFD: f.supports}
}

// plainDriver implements neither CapabilityProvider nor any other interface,
// standing in for an engine that fails to advertise optional capabilities.
type plainDriver struct{}

// TestEnsureBFDCapability verifies the wiring gate consults the driver's
// Capabilities() at runtime and fails closed when BFD is requested but the
// engine cannot honour it.
func TestEnsureBFDCapability(t *testing.T) {
	r := require.New(t)
	bfdSpec := announcer.DriverSpec{Bfd: announcer.BfdSpec{Enabled: true}}

	// BFD disabled: always allowed, capability never consulted.
	r.NoError(ensureBFDCapability("gobgpv4", announcer.DriverSpec{}, plainDriver{}))

	// BFD enabled + supporting driver: allowed.
	r.NoError(ensureBFDCapability("gobgpv4", bfdSpec, fakeCapDriver{supports: true}))

	// BFD enabled + driver that does not support BFD: fails closed.
	err := ensureBFDCapability("gobgpv3", bfdSpec, fakeCapDriver{supports: false})
	r.Error(err)
	r.Equal(`engine "gobgpv3" does not support BFD`, err.Error())

	// BFD enabled + driver that does not implement CapabilityProvider: fails closed.
	err = ensureBFDCapability("gobgpv4", bfdSpec, plainDriver{})
	r.Error(err)
	r.Equal(`engine "gobgpv4" does not support BFD`, err.Error())
}
