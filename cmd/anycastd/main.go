package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/runityru/anycastd/announcer"
	"github.com/runityru/anycastd/checkers"
	"github.com/runityru/anycastd/config"
	"github.com/runityru/anycastd/service"

	// checkers in build
	_ "github.com/runityru/anycastd/checkers/assigned_address"
	_ "github.com/runityru/anycastd/checkers/dns_lookup"
	_ "github.com/runityru/anycastd/checkers/http_2xx"
	_ "github.com/runityru/anycastd/checkers/icmp_ping"
	_ "github.com/runityru/anycastd/checkers/ntpq"
	_ "github.com/runityru/anycastd/checkers/tftp_rrq"
	_ "github.com/runityru/anycastd/checkers/tls_certificate"
)

var (
	appVersion     = "n/a (dev build)"
	buildTimestamp = "undefined"
)

// driverSpec maps the config Announcer onto the engine-agnostic
// announcer.DriverSpec, resolving each peer's local address to the
// family-appropriate value ("" lets the engine use the kernel source).
func driverSpec(a config.Announcer) announcer.DriverSpec {
	peers := make([]announcer.PeerSpec, 0, len(a.Peers))
	for _, p := range a.Peers {
		peers = append(peers, announcer.PeerSpec{
			Name:           p.Name,
			RemoteAddress:  p.RemoteAddress,
			RemoteASN:      p.RemoteASN,
			EnableMultihop: p.EnableMultihop,
			MultihopTTL:    p.MultihopTTL,
			LocalAddress:   a.LocalAddressFor(p),
		})
	}

	return announcer.DriverSpec{
		RouterID: a.RouterID,
		LocalASN: a.LocalASN,
		Peers:    peers,
		Bfd: announcer.BfdSpec{
			Enabled:              a.Bfd.Enabled,
			Port:                 a.Bfd.Port,
			DesiredMinTxInterval: a.Bfd.MinTxInterval,
			RequiredMinReceive:   a.Bfd.MinRxInterval,
			DetectionMultiplier:  a.Bfd.Multiplier,
		},
	}
}

type spec struct {
	ConfigPath string    `envconfig:"CONFIG_PATH" default:"/config.yaml"`
	LogLevel   log.Level `envconfig:"LOG_LEVEL" default:"WARN"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := spec{}
	envconfig.MustProcess("", &s)

	cfg, err := config.NewFromFile(s.ConfigPath)
	if err != nil {
		panic(err)
	}

	log.SetLevel(s.LogLevel)

	lf := new(log.TextFormatter)
	lf.FullTimestamp = true
	log.SetFormatter(lf)

	log.Infof("Initializing anycastd (%s @ %s) ...", appVersion, buildTimestamp)

	// runCtx drives the cancellable goroutines (service checks and the GoBGP
	// metrics poller). It is derived from the signal ctx and cancelled
	// explicitly in shutdown() so those loops stop before any prefix is
	// withdrawn.
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	g, gctx := errgroup.WithContext(runCtx)

	log.Trace("Initializing BGP server ...")
	dspec := driverSpec(cfg.Announcer)

	driver, err := newGoBGPDriver(cfg.Announcer.Engine, dspec, &log.Logger{
		Out:       os.Stderr,
		Formatter: lf,
		Hooks:     make(log.LevelHooks),
		Level:     s.LogLevel,
	})
	if err != nil {
		panic(err)
	}

	// Serve runs on its own goroutine. Both GoBGP engines' Serve() loops block
	// until the server is stopped, and the gobgpv3 engine's loop never returns,
	// so Serve is intentionally excluded from the cancellable errgroup `g`
	// above: that group must finish during shutdown, which a never-returning
	// Serve would prevent. Serve is terminated by driver.Stop() in shutdown().
	go func() {
		if err := driver.Serve(); err != nil {
			log.Errorf("BGP server Serve exited with error: %s", err)
		}
	}()

	log.Trace("Starting BGP sessions ...")
	if err := driver.Start(ctx, dspec); err != nil {
		panic(err)
	}

	metrics, err := service.NewMetrics(prometheus.DefaultRegisterer, appVersion)
	if err != nil {
		panic(err)
	}

	log.Info("Starting service initialization ...")

	services, announcers, err := buildServices(
		cfg.Services,
		cfg.Announcer.Routes,
		driver,
		metrics,
		checkers.NewCheckerByKind,
	)
	if err != nil {
		panic(err)
	}

	for _, svc := range services {
		g.Go(func() error {
			return svc.Run(gctx)
		})
	}

	var httpSrv *http.Server
	if cfg.Metrics.Enabled {
		log.Debug("metrics server is enabled, initializing ...")

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		// The metrics endpoint sets explicit timeouts to mitigate slowloris
		// (gosec G112) and bound resource use per request/connection.
		// Prometheus scrapes are small and fast, so these are far above what
		// a scrape needs (see SPEC.md §8).
		httpSrv = &http.Server{
			Addr:              cfg.Metrics.Address,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		}

		// The metrics HTTP listener is kept out of `g` and shut down explicitly
		// in shutdown(); ListenAndServe only returns on Shutdown, so waiting on
		// it via the errgroup would deadlock the withdrawal sequence.
		go func() {
			if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Errorf("metrics server error: %s", err)
			}
		}()

		amr := announcer.NewMetricsRepository(driver, cfg.Announcer.RouterID, prometheus.DefaultRegisterer, log.StandardLogger())
		if err := amr.Register(); err != nil {
			panic(err)
		}

		g.Go(func() error {
			return amr.Run(gctx)
		})
	}

	log.Infof("Initialization completed")

	// Block until a termination signal, or until a cancellable goroutine fails
	// (which is treated as a shutdown trigger). We never block shutdown on
	// g.Wait() directly: Serve and, until closed, the metrics listener do not
	// return, so the previous `defer driver.Stop(ctx)` pattern never ran and
	// left peers holding the anycast route until the hold timer expired.
	go func() {
		if err := g.Wait(); err != nil {
			log.Errorf("fatal error: %s", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("Shutting down ...")

	shutdown(driver, announcers, httpSrv, cancelRun, g, shutdownTimeout)
}
