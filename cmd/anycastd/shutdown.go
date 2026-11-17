package main

import (
	"context"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/runityru/anycastd/announcer"
)

// shutdownTimeout bounds each blocking step of shutdown() (prefix withdrawal,
// metrics server shutdown, BGP engine stop). A slow peer or a wedged listener
// must never stall process exit indefinitely.
const shutdownTimeout = 10 * time.Second

// shutdown performs the graceful anycast teardown in a fixed order. The order
// is critical for anycast traffic:
//
//  1. The health-check and GoBGP metrics loops are stopped and waited on, so no
//     goroutine can re-announce a prefix after it has been withdrawn.
//  2. Every announced prefix is withdrawn over the still-established BGP
//     sessions (an immediate UPDATE-withdrawal via DeletePath).
//  3. The metrics HTTP listener is closed.
//  4. The BGP engine is stopped, closing the peer sessions and, on GoBGP v4,
//     tearing down BFD.
//
// Every blocking step uses a fresh context derived from context.Background with
// a bounded timeout, never the (already-cancelled) signal context, so shutdown
// cannot be starved by a cancelled context.
func shutdown(
	driver announcer.Driver,
	announcers []announcer.Announcer,
	httpSrv *http.Server,
	cancelRun context.CancelFunc,
	g *errgroup.Group,
	timeout time.Duration,
) {
	// 1. Stop the health-check and metrics loops and wait for them to exit.
	cancelRun()
	if err := g.Wait(); err != nil {
		// A failing health/metrics loop is logged (and would already have been
		// reported by the errgroup watcher in main); shutdown still proceeds so
		// prefixes are withdrawn and BGP is stopped regardless.
		log.Warnf("service/metrics loop exited with error during shutdown: %s", err)
	}

	// 2. Withdraw every announced prefix over the live sessions.
	denounceCtx, cancelDenounce := context.WithTimeout(context.Background(), timeout)
	defer cancelDenounce()
	for _, a := range announcers {
		if err := a.Denounce(denounceCtx); err != nil {
			log.Warnf("error withdrawing prefixes: %s", err)
		}
	}

	// 3. Shut the metrics HTTP listener down.
	if httpSrv != nil {
		if err := httpSrv.Shutdown(denounceCtx); err != nil {
			log.Warnf("error shutting down metrics server: %s", err)
		}
	}

	// 4. Stop the BGP engine: closes peer sessions and, on GoBGP v4, tears
	//    down BFD. Uses an independent timeout context.
	stopCtx, cancelStop := context.WithTimeout(context.Background(), timeout)
	defer cancelStop()
	if err := driver.Stop(stopCtx); err != nil {
		log.Warnf("error stopping BGP session: %s", err)
	}
}
