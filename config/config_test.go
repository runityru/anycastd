package config

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	r := require.New(t)

	cfg, err := NewFromFile("testdata/sample.yaml")
	r.NoError(err)
	r.Equal(&Config{
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
	}, cfg)
}
