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
}
