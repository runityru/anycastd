package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/runityru/anycastd/announcer"
)

// callLog records the ordered sequence of lifecycle events during shutdown, so
// tests can assert both that the right operations ran and that they ran in the
// correct order (withdrawal before driver stop).
type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *callLog) record(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, s)
}

func (l *callLog) list() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.calls))
	copy(out, l.calls)
	return out
}

// shutdownDriver is a minimal hand-written announcer.Driver that records calls
// and detects whether it received a cancelled context (which must not happen:
// shutdown must pass a fresh, non-cancelled context).
type shutdownDriver struct {
	log *callLog
}

func (d *shutdownDriver) Serve() error { return nil }
func (d *shutdownDriver) Start(context.Context, announcer.DriverSpec) error {
	return nil
}
func (d *shutdownDriver) Stop(ctx context.Context) error {
	if ctx.Err() != nil {
		d.log.record("stop-cancelled-ctx")
	} else {
		d.log.record("stop")
	}
	return nil
}
func (d *shutdownDriver) AddPeer(context.Context, announcer.PeerSpec) error { return nil }
func (d *shutdownDriver) ListPeers(context.Context) ([]announcer.PeerState, error) {
	return nil, nil
}
func (d *shutdownDriver) AddPath(context.Context, string) error    { return nil }
func (d *shutdownDriver) DeletePath(context.Context, string) error { return nil }

// shutdownAnnouncer records Denounce and detects a cancelled context.
type shutdownAnnouncer struct {
	log *callLog
}

func (a *shutdownAnnouncer) Announce(context.Context) error { return nil }

func (a *shutdownAnnouncer) Denounce(ctx context.Context) error {
	if ctx.Err() != nil {
		a.log.record("denounce-cancelled-ctx")
	} else {
		a.log.record("denounce")
	}
	return nil
}

// TestShutdownOrder asserts the teardown sequence: health/metrics loops stop
// first, then every prefix is withdrawn, then the driver is stopped — and that
// neither Denounce nor Stop receives a cancelled context.
func TestShutdownOrder(t *testing.T) {
	r := require.New(t)

	logs := &callLog{}

	driver := &shutdownDriver{log: logs}
	ann1 := &shutdownAnnouncer{log: logs}
	ann2 := &shutdownAnnouncer{log: logs}

	ctx, cancel := context.WithCancel(context.Background())
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		<-gctx.Done()
		logs.record("loop-stopped")
		return nil
	})

	shutdown(driver, []announcer.Announcer{ann1, ann2}, nil, cancel, g, time.Second)

	r.Equal([]string{
		"loop-stopped",
		"denounce",
		"denounce",
		"stop",
	}, logs.list())
}

// TestShutdownClosesMetricsServer asserts that shutdown closes the metrics HTTP
// listener gracefully (Serve returns http.ErrServerClosed).
func TestShutdownClosesMetricsServer(t *testing.T) {
	r := require.New(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	r.NoError(err)

	httpSrv := &http.Server{Handler: http.NewServeMux()}
	served := make(chan error, 1)
	go func() { served <- httpSrv.Serve(ln) }()

	r.Eventually(func() bool {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}, time.Second, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		<-gctx.Done()
		return nil
	})

	shutdown(&shutdownDriver{log: &callLog{}}, nil, httpSrv, cancel, g, time.Second)

	select {
	case err := <-served:
		r.ErrorIs(err, http.ErrServerClosed)
	case <-time.After(time.Second):
		t.Fatal("metrics server did not shut down")
	}
}

// shutdownLogHook captures logrus warning messages emitted during shutdown so
// tests can assert the expected branch (timeout vs. failing loop) was actually
// taken, rather than only that shutdown happened to reach Denounce/Stop.
type shutdownLogHook struct {
	mu     sync.Mutex
	levels []log.Level
	msgs   []string
}

func (h *shutdownLogHook) Levels() []log.Level {
	return []log.Level{log.WarnLevel}
}

func (h *shutdownLogHook) Fire(e *log.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.levels = append(h.levels, e.Level)
	h.msgs = append(h.msgs, e.Message)
	return nil
}

func (h *shutdownLogHook) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.msgs))
	copy(out, h.msgs)
	return out
}

// attachShutdownLogHook installs a capture hook on the standard logrus logger
// and registers cleanup that removes it. logrus exposes no RemoveHook, so the
// hook is removed by filtering it out of the standard logger's Warn-level hook
// slice, leaving any pre-existing hooks intact.
func attachShutdownLogHook(t *testing.T) *shutdownLogHook {
	t.Helper()
	hook := &shutdownLogHook{}
	log.AddHook(hook)
	t.Cleanup(func() {
		std := log.StandardLogger()
		hooks := std.Hooks[log.WarnLevel]
		for i, h := range hooks {
			if h == hook {
				std.Hooks[log.WarnLevel] = append(hooks[:i], hooks[i+1:]...)
				break
			}
		}
	})
	return hook
}

// TestShutdownFailContinue asserts that a health/metrics loop returning a
// non-nil error is logged and shutdown still proceeds to withdraw prefixes and
// stop BGP — the failure is not fatal.
func TestShutdownFailContinue(t *testing.T) {
	r := require.New(t)

	hook := attachShutdownLogHook(t)

	logs := &callLog{}
	driver := &shutdownDriver{log: logs}
	ann := &shutdownAnnouncer{log: logs}

	ctx, cancel := context.WithCancel(context.Background())
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		<-gctx.Done() // blocked until shutdown cancels the run context
		return errors.New("check loop crashed")
	})

	shutdown(driver, []announcer.Announcer{ann}, nil, cancel, g, time.Second)

	// The failing loop must not stop the withdrawal sequence.
	r.Equal([]string{"denounce", "stop"}, logs.list())
	r.Contains(strings.Join(hook.messages(), "\n"), "exited with error during shutdown")
}

// TestShutdownTimeout asserts that a loop which never returns after
// cancellation causes waitForGroup to hit errGroupTimeout (a tiny timeout), and
// that shutdown still reaches Denounce and Stop without hanging. The wedged
// loop is released only after the test completes, so no goroutine is leaked.
func TestShutdownTimeout(t *testing.T) {
	r := require.New(t)

	hook := attachShutdownLogHook(t)

	logs := &callLog{}
	driver := &shutdownDriver{log: logs}
	ann := &shutdownAnnouncer{log: logs}

	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	defer close(release) // unblock the wedged loop after shutdown returns

	g, _ := errgroup.WithContext(ctx)
	g.Go(func() error {
		<-ctx.Done() // cancelled by cancelRun, but the loop "wedges" past the timeout
		<-release    // do not return until the test is done
		return nil
	})

	shutdown(driver, []announcer.Announcer{ann}, nil, cancel, g, 50*time.Millisecond)

	r.Equal([]string{"denounce", "stop"}, logs.list())
	r.Contains(strings.Join(hook.messages(), "\n"), "did not exit within")
}
