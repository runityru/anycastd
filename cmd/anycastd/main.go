package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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

	g, _ := errgroup.WithContext(ctx)

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

	g.Go(func() error {
		log.Trace("Starting to serve GoBGP API")
		return driver.Serve()
	})

	log.Trace("Starting BGP sessions ...")
	if err := driver.Start(ctx, dspec); err != nil {
		panic(err)
	}
	defer func() {
		if err := driver.Stop(ctx); err != nil {
			log.Warnf("error stopping BGP session: %s", err)
		}
	}()

	metrics, err := service.NewMetrics(appVersion)
	if err != nil {
		panic(err)
	}

	log.Info("Starting service initialization ...")

	services, err := buildServices(
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
			return svc.Run(ctx)
		})
	}

	if cfg.Metrics.Enabled {
		log.Debug("metrics server is enabled, initializing ...")

		g.Go(func() error {
			http.Handle("/metrics", promhttp.Handler())
			// nosec G114: the metrics endpoint intentionally has no
			// read/write timeouts (documented limitation, SPEC.md §8).
			return http.ListenAndServe(cfg.Metrics.Address, nil) // #nosec G114
		})

		amr := announcer.NewMetricsRepository(driver, cfg.Announcer.RouterID, prometheus.DefaultRegisterer, log.StandardLogger())
		if err := amr.Register(); err != nil {
			panic(err)
		}

		g.Go(func() error {
			return amr.Run(ctx)
		})
	}

	log.Infof("Initialization completed")

	if err := g.Wait(); err != nil {
		panic(err)
	}
}
