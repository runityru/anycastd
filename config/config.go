package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v3"
)

type Announcer struct {
	LocalAddress string   `json:"local_address"`
	LocalAS      uint32   `json:"local_as"`
	Routes       []string `json:"routes"`
	Peers        []Peer   `json:"peers"`
}

type Check struct {
	Kind string          `json:"kind"`
	Spec json.RawMessage `json:"spec"`
}

type Peer struct {
	Name          string `json:"name"`
	RemoteAddress string `json:"remote_address"`
	RemoteAS      uint32 `json:"remote_as"`
}

type Service struct {
	Name          string   `json:"name"`
	CheckOperator string   `json:"check_operator"`
	CheckInterval Duration `json:"check_interval"`
	Checks        []Check  `json:"checks"`
}

type Metrics struct {
	Enabled bool   `json:"enabled"`
	Address string `json:"address"`
}

type Config struct {
	Announcer Announcer `json:"announcer"`
	Services  []Service `json:"services"`
	Metrics   Metrics   `json:"metrics"`
}

func NewFromFile(filename string) (*Config, error) {
	cfg := &Config{}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrap(err, "error reading configuration file")
	}

	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".yml", ".yaml":
		type intermediate struct {
			Announcer map[string]any `yaml:"announcer"`
			Services  []any          `yaml:"services"`
			Metrics   map[string]any `yaml:"metrics"`
		}

		d := intermediate{}

		err := yaml.Unmarshal(data, &d)
		if err != nil {
			return nil, errors.Wrap(err, "error unmarshaling intermediate configuration")
		}

		data, err = json.Marshal(d)
		if err != nil {
			return nil, errors.Wrap(err, "error marshaling intermediate configuration")
		}
	case ".json":
		// no additional action needed
	default:
		return nil, errors.Errorf("unexpected file format: `%s`", ext)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Wrap(err, "error unmarshaling config")
	}

	return cfg, nil
}