package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/kelseyhightower/envconfig"
	apipb "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/teran/anycastd/announcer"
	"github.com/teran/anycastd/checkers"
	"github.com/teran/anycastd/config"
	"github.com/teran/anycastd/service"

	// checkers in build
	_ "github.com/teran/anycastd/checkers/assigned_address"
	_ "github.com/teran/anycastd/checkers/dns_lookup"
	_ "github.com/teran/anycastd/checkers/http_2xx"
)

var (
	appVersion     = "n/a (dev build)"
	buildTimestamp = "undefined"
)

type spec struct {
	ConfigPath string    `envconfig:"CONFIG_PATH" default:"/config.yaml"`
	LogLevel   log.Level `envconfig:"LOG_LEVEL" default:"WARN"`
}

func main() {
	ctx := context.TODO()

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
	bgpSrv := server.NewBgpServer(server.LoggerOption(&announcer.Logger{Logger: &log.Logger{
		Out:       os.Stderr,
		Formatter: lf,
		Hooks:     make(log.LevelHooks),
		Level:     s.LogLevel,
	}}))

	g.Go(func() error {
		log.Trace("Starting to serve GoBGP API")
		bgpSrv.Serve()
		return nil
	})

	log.Trace("Starting BGP sessions ...")
	if err := bgpSrv.StartBgp(ctx, &apipb.StartBgpRequest{
		Global: &apipb.Global{
			RouterId:   cfg.Announcer.RouterID,
			Asn:        cfg.Announcer.LocalASN,
			ListenPort: -1,
		},
	}); err != nil {
		panic(err)
	}
	defer bgpSrv.StopBgp(ctx, &apipb.StopBgpRequest{})

	if err := bgpSrv.WatchEvent(context.Background(), &apipb.WatchEventRequest{
		Peer: &apipb.WatchEventRequest_Peer{},
	}, func(r *apipb.WatchEventResponse) {
		if p := r.GetPeer(); p != nil && p.Type == apipb.WatchEventResponse_PeerEvent_STATE {
			log.Info(p)
		}
	}); err != nil {
		panic(err)
	}

	for _, peer := range cfg.Announcer.Peers {
		err = bgpSrv.AddPeer(context.Background(), &apipb.AddPeerRequest{
			Peer: &apipb.Peer{
				Conf: &apipb.PeerConf{
					NeighborAddress: peer.RemoteAddress,
					PeerAsn:         peer.RemoteASN,
				},
			},
		})
		if err != nil {
			panic(err)
		}
	}

	log.Info("Starting service initialization ...")
	for _, svcCfg := range cfg.Services {
		log.Tracef("Initializing service %s ...", svcCfg.Name)

		checks := []checkers.Checker{}
		for _, check := range svcCfg.Checks {
			log.WithFields(log.Fields{
				"service": svcCfg.Name,
				"check":   check.Kind,
			}).Trace("registering check ...")

			c, err := checkers.NewCheckerByKind(check.Kind, check.Spec)
			if err != nil {
				panic(err)
			}

			checks = append(checks, c)
		}

		a := announcer.New(announcer.Config{
			GoBGP:    bgpSrv,
			Prefixes: cfg.Announcer.Routes,
			NextHop:  cfg.Announcer.LocalAddress,
			LocalASN: cfg.Announcer.LocalASN,
		})
		svc := service.New(svcCfg.Name, a, checks, time.Duration(svcCfg.CheckInterval))

		g.Go(func() error {
			return svc.Run(ctx)
		})
	}

	if cfg.Metrics.Enabled {
		log.Debug("metrics server is enabled, initializing ...")

		g.Go(func() error {
			http.Handle("/metrics", promhttp.Handler())
			return http.ListenAndServe(cfg.Metrics.Address, nil)
		})
	}

	log.Infof("Initialization completed")

	if err := g.Wait(); err != nil {
		panic(err)
	}
}
