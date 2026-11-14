package main

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/runityru/anycastd/announcer"
	"github.com/runityru/anycastd/announcer/gobgpv3"
	"github.com/runityru/anycastd/announcer/gobgpv4"
)

// newGoBGPDriver maps the config `engine` selector onto a concrete GoBGP
// driver implementation. Both engines share the same construction signature
// and the same engine-agnostic announcer.Driver contract.
func newGoBGPDriver(engine string, spec announcer.DriverSpec, logger *log.Logger) (announcer.Driver, error) {
	switch engine {
	case "gobgpv3":
		return gobgpv3.NewDriver(spec, logger)
	case "gobgpv4":
		return gobgpv4.NewDriver(spec, logger)
	default:
		return nil, fmt.Errorf("unknown BGP engine %q", engine)
	}
}
