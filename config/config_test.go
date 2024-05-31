package config

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	type testCase struct {
		name       string
		samplePath string
		expOut     Config
		expError   error
	}

	sampleConfig := Config{
		Services: []Service{
			{
				Name:          "http",
				CheckOperator: "and",
				CheckInterval: Duration(time.Duration(10 * time.Second)),
				Checks: []Check{
					{
						Kind: "dns_lookup",
						Spec: json.RawMessage(`{"interval":"100ms","query":"example.com","resolver":"127.0.0.1","tries":3}`),
					},
					{
						Kind: "http_2xx",
						Spec: json.RawMessage(`{"address":"127.0.0.1:8080","interval":"100ms","path":"/","timeout":"2s","tries":3}`),
					},
					{
						Kind: "assigned_address",
						Spec: json.RawMessage(`{"interface":"dummy0","ipv4":"10.0.0.128"}`),
					},
				},
				Peers: []Peer{
					{
						Name:          "some_router_1",
						RemoteAddress: "10.0.0.252",
						RemoteAS:      65000,
						LocalAddress:  "10.0.0.1",
						LocalAS:       65999,
						Routes:        []string{"10.0.0.128/32"},
					},
					{
						Name:          "some_router_2",
						RemoteAddress: "10.0.0.253",
						RemoteAS:      65000,
						LocalAddress:  "10.0.0.1",
						LocalAS:       65999,
						Routes:        []string{"10.0.0.128/32"},
					},
				},
			},
		},
		Metrics: Metrics{
			Enabled: true,
			Address: "127.0.0.1:9090",
		},
	}

	tcs := []testCase{
		{
			name:       "YAML configuration",
			samplePath: "testdata/sample.yaml",
			expOut:     sampleConfig,
		},
		{
			name:       "JSON configuration",
			samplePath: "testdata/sample.json",
			expOut:     sampleConfig,
		},
		{
			name:       "wrong config file extension",
			samplePath: "testdata/sample.unknown",
			expError:   errors.New("unexpected file format: `.unknown`"),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			cfg, err := NewFromFile(tc.samplePath)
			if tc.expError == nil {
				r.NoError(err)
				for i := range tc.expOut.Services {
					r.Equalf(tc.expOut.Services[i].Name, cfg.Services[i].Name, "svc#%d", i)
					r.Equalf(tc.expOut.Services[i].CheckInterval, cfg.Services[i].CheckInterval, "svc#%d", i)
					r.Equalf(tc.expOut.Services[i].CheckOperator, cfg.Services[i].CheckOperator, "svc#%d", i)
					for j := range tc.expOut.Services[i].Checks {
						r.Equalf(tc.expOut.Services[i].Checks[j].Kind, cfg.Services[i].Checks[j].Kind, "svc#%d check#%d", i, j)
						r.JSONEqf(string(tc.expOut.Services[i].Checks[j].Spec), string(cfg.Services[i].Checks[j].Spec), "svc#%d check#%d", i, j)
					}
					r.Equalf(tc.expOut.Services[i].Peers, cfg.Services[i].Peers, "svc#%d", i)
				}
				r.Equal(tc.expOut.Metrics, cfg.Metrics)
			} else {
				r.Error(err)
				r.Equal(tc.expError.Error(), err.Error())
			}
		})
	}
}
