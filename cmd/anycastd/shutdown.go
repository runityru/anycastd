package main

import (
	"context"
	"errors"
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

// errGroupTimeout is returned by waitForGroup when the errgroup's goroutines do
// not finish within the allotted timeout. shutdown() treats it as a non-fatal,
// warn-and-continue condition.
var errGroupTimeout = errors.New("service/metrics loops timed out during shutdown")

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
	//    g.Wait() has no timeout of its own, so it is raced against a bounded
	//    timeout. If the loops wedge (fail to return after cancellation),
	//    shutdown still proceeds to withdraw prefixes and stop BGP rather than
	//    blocking indefinitely.
	cancelRun()
	switch err := waitForGroup(g, timeout); {
	case errors.Is(err, errGroupTimeout):
		// A loop that failed to exit within the timeout must not block the
		// withdrawal sequence: continue to denounce even though the loops did
		// not stop in time.
		log.Warnf("service/metrics loops did not exit within %s during shutdown, continuing", timeout)
	case err != nil:
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

// waitForGroup waits for every goroutine in g to finish, bounded by timeout.
// It returns errGroupTimeout if the group has not finished within timeout, so
// the caller can proceed without blocking forever on a wedged loop. The result
// channel is buffered (capacity 1), so the spawned waiter goroutine never
// leaks: even when waitForGroup returns on the timeout branch without reading
// from it, the waiter still completes its single send into the buffer and
// exits. A stopped timer avoids holding the timeout timer open on the common
// fast path.
func waitForGroup(g *errgroup.Group, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- g.Wait()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-done:
		return err
	case <-timer.C:
		return errGroupTimeout
	}
}
