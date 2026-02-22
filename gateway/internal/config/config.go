// Package config loads and validates the ZTNA gateway configuration.
package config

import (
"fmt"
"os"
"time"

"gopkg.in/yaml.v3"
)

// Config is the root configuration for the ZTNA gateway.
type Config struct {
	ListenAddr     string        `yaml:"listen_addr"`
	GatewayID      string        `yaml:"gateway_id"`
	TLS            TLSConfig     `yaml:"tls"`
	CPURL          string        `yaml:"cp_url"`
	PEPID          string        `yaml:"pep_id"`
	PEPToken       string        `yaml:"pep_token"`
	CPTLSInsecure  bool          `yaml:"cp_tls_insecure"`
	HeartbeatEvery time.Duration `yaml:"heartbeat_every"`
	Routes         []Route       `yaml:"routes"`
}

// TLSConfig holds TLS material paths.
type TLSConfig struct {
	DeviceCACert string `yaml:"device_ca_cert"`
	ServerCert   string `yaml:"server_cert"`
	ServerKey    string `yaml:"server_key"`
}

// Route maps a resource type + match pattern to a backend address.
type Route struct {
	ResourceType  string `yaml:"resource_type"`
	ResourceMatch string `yaml:"resource_match"`
	Target        string `yaml:"target"`
}

// Load reads and validates the YAML configuration at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.ListenAddr == "" {
		c.ListenAddr = "0.0.0.0:4433"
	}
	if c.CPURL == "" {
		return fmt.Errorf("cp_url is required")
	}
	if c.PEPID == "" {
		return fmt.Errorf("pep_id is required")
	}
	if c.PEPToken == "" {
		return fmt.Errorf("pep_token is required")
	}
	if c.HeartbeatEvery == 0 {
		c.HeartbeatEvery = 30 * time.Second
	}
	return nil
}
