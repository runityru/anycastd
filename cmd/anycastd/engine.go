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
	var (
		driver announcer.Driver
		err    error
	)
	switch engine {
	case "gobgpv3":
		driver, err = gobgpv3.NewDriver(spec, logger)
	case "gobgpv4":
		driver, err = gobgpv4.NewDriver(spec, logger)
	default:
		return nil, fmt.Errorf("unknown BGP engine %q", engine)
	}
	if err != nil {
		return nil, err
	}

	// Fail closed on the driver's own capability report: even if config
	// validation is bypassed or a future engine adds config parsing before BFD
	// support, enabling BFD against an engine that cannot honour it is an
	// error rather than a silent no-op.
	if err := ensureBFDCapability(engine, spec, driver); err != nil {
		return nil, err
	}

	return driver, nil
}

// ensureBFDCapability returns an error when BFD is enabled but the driver does
// not report BFD support via its CapabilityProvider. It accepts the driver as
// an untyped value so a driver that fails to advertise the (optional)
// CapabilityProvider interface is treated as non-supporting (fail-closed).
func ensureBFDCapability(engine string, spec announcer.DriverSpec, d any) error {
	if !spec.Bfd.Enabled {
		return nil
	}
	cp, ok := d.(announcer.CapabilityProvider)
	if !ok || !cp.Capabilities().SupportsBFD {
		return fmt.Errorf("engine %q does not support BFD", engine)
	}
	return nil
}
