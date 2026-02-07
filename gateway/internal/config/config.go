package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the gateway configuration
type Config struct {
	Server       ServerConfig       `yaml:"server"`
	ControlPlane ControlPlaneConfig `yaml:"controlplane"`
	SSH          SSHConfig          `yaml:"ssh"`
	Routing      RoutingConfig      `yaml:"routing"`
	Logging      LoggingConfig      `yaml:"logging"`
	Session      SessionConfig      `yaml:"session"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Host      string `yaml:"host"`
	SSHPort   int    `yaml:"ssh_port"`
	AdminPort int    `yaml:"admin_port"`
}

// ControlPlaneConfig holds Control Plane connection info
type ControlPlaneConfig struct {
	URL                 string `yaml:"url"`
	CAPublicKeyEndpoint string `yaml:"ca_public_key_endpoint"`
	PolicyCheckEndpoint string `yaml:"policy_check_endpoint"`
	TLSSkipVerify       bool   `yaml:"tls_skip_verify"`
}

// SSHConfig holds SSH server configuration
type SSHConfig struct {
	HostKeyPath       string `yaml:"host_key_path"`
	TrustedCAKeysPath string `yaml:"trusted_ca_keys_path"`
}

// TargetConfig represents a backend SSH server
type TargetConfig struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	User string `yaml:"user"`
}

// RoutingConfig holds routing table
type RoutingConfig struct {
	Targets []TargetConfig `yaml:"targets"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// SessionConfig holds session management configuration
type SessionConfig struct {
	Timeout       string `yaml:"timeout"`
	MaxConcurrent int    `yaml:"max_concurrent"`
}

// TimeoutDuration returns the session timeout as time.Duration
func (s *SessionConfig) TimeoutDuration() (time.Duration, error) {
	return time.ParseDuration(s.Timeout)
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// Try local config.yaml as fallback
		if path != "config.yaml" {
			localPath := "config.yaml"
			data, err = os.ReadFile(localPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read config from %s or %s: %w", path, localPath, err)
			}
		} else {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, fmt.Errorf("failed to apply environment overrides: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Server.SSHPort < 1 || c.Server.SSHPort > 65535 {
		return fmt.Errorf("invalid SSH port: %d", c.Server.SSHPort)
	}

	if c.Server.AdminPort < 1 || c.Server.AdminPort > 65535 {
		return fmt.Errorf("invalid admin port: %d", c.Server.AdminPort)
	}

	if c.ControlPlane.URL == "" {
		return fmt.Errorf("control plane URL is required")
	}

	if len(c.Routing.Targets) == 0 {
		return fmt.Errorf("at least one routing target is required")
	}

	for _, target := range c.Routing.Targets {
		if target.Name == "" {
			return fmt.Errorf("target name is required")
		}
		if target.Host == "" {
			return fmt.Errorf("target host is required for %s", target.Name)
		}
		if target.Port < 1 || target.Port > 65535 {
			return fmt.Errorf("invalid target port for %s: %d", target.Name, target.Port)
		}
	}

	return nil
}

// GetTarget returns a target by name
func (c *Config) GetTarget(name string) (*TargetConfig, error) {
	for _, target := range c.Routing.Targets {
		if target.Name == name {
			return &target, nil
		}
	}
	return nil, fmt.Errorf("target not found: %s", name)
}

func applyEnvOverrides(cfg *Config) error {
	overrideString := func(env string, target *string) {
		if value := os.Getenv(env); value != "" {
			*target = value
		}
	}

	overrideInt := func(env string, target *int) error {
		value := os.Getenv(env)
		if value == "" {
			return nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer: %w", env, err)
		}
		*target = parsed
		return nil
	}

	overrideBool := func(env string, target *bool) error {
		value := os.Getenv(env)
		if value == "" {
			return nil
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be a boolean: %w", env, err)
		}
		*target = parsed
		return nil
	}

	// Server
	overrideString("ZTNA_GW_SERVER_HOST", &cfg.Server.Host)
	if err := overrideInt("ZTNA_GW_SSH_PORT", &cfg.Server.SSHPort); err != nil {
		return err
	}
	if err := overrideInt("ZTNA_GW_ADMIN_PORT", &cfg.Server.AdminPort); err != nil {
		return err
	}

	// Control Plane connectivity
	overrideString("ZTNA_GW_CP_URL", &cfg.ControlPlane.URL)
	overrideString("ZTNA_GW_CP_CA_ENDPOINT", &cfg.ControlPlane.CAPublicKeyEndpoint)
	overrideString("ZTNA_GW_CP_POLICY_ENDPOINT", &cfg.ControlPlane.PolicyCheckEndpoint)
	if err := overrideBool("ZTNA_GW_CP_TLS_SKIP_VERIFY", &cfg.ControlPlane.TLSSkipVerify); err != nil {
		return err
	}

	// SSH
	overrideString("ZTNA_GW_HOST_KEY_PATH", &cfg.SSH.HostKeyPath)
	overrideString("ZTNA_GW_TRUSTED_CA_KEYS_PATH", &cfg.SSH.TrustedCAKeysPath)

	// Session and logging
	overrideString("ZTNA_GW_SESSION_TIMEOUT", &cfg.Session.Timeout)
	if err := overrideInt("ZTNA_GW_SESSION_MAX_CONCURRENT", &cfg.Session.MaxConcurrent); err != nil {
		return err
	}
	overrideString("ZTNA_GW_LOG_LEVEL", &cfg.Logging.Level)
	overrideString("ZTNA_GW_LOG_FORMAT", &cfg.Logging.Format)
	overrideString("ZTNA_GW_LOG_OUTPUT", &cfg.Logging.Output)

	return nil
}
