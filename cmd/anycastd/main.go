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
	"github.com/runityru/anycastd/announcer/gobgpv3"
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
		RouterID:      a.RouterID,
		LocalASN:      a.LocalASN,
		LocalAddress:  a.LocalAddress,
		LocalAddress6: a.LocalAddress6,
		Peers:         peers,
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
	driver, err := gobgpv3.NewDriver(dspec, &log.Logger{
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
	for _, svcCfg := range cfg.Services {
		log.Tracef("Initializing service %s ...", svcCfg.Name)

		checks := []service.Checker{}
		for _, check := range svcCfg.Checks {
			log.WithFields(log.Fields{
				"service": svcCfg.Name,
				"check":   check.Kind,
			}).Trace("registering check ...")

			c, err := checkers.NewCheckerByKind(check.Kind, check.Spec)
			if err != nil {
				panic(err)
			}

			checks = append(checks, service.Checker{Check: c, Group: check.Group})
		}

		a := announcer.New(announcer.Config{
			GoBGP:    driver,
			Prefixes: cfg.Announcer.Routes,
		})

		strategy, err := service.GetStrategy(svcCfg.Strategy, svcCfg.StrategyOptions)
		if err != nil {
			panic(err)
		}

		svc := service.New(svcCfg.Name, a, checks, svcCfg.CheckInterval.TimeDuration(), metrics, strategy)

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
