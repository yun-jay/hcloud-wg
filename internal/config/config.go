package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HCloudToken string          `yaml:"hcloud_token"`
	WireGuard   WireGuardConfig `yaml:"wireguard"`
	SSH         SSHConfig       `yaml:"ssh"`
	Defaults    DefaultsConfig  `yaml:"defaults"`
}

type WireGuardConfig struct {
	ServerPublicIP   string `yaml:"server_public_ip"`
	ServerPort       int    `yaml:"server_port"`
	ServerPublicKey  string `yaml:"server_public_key"`
	ServerInternalIP string `yaml:"server_internal_ip"`
	Subnet           string `yaml:"subnet"`
	IPRangeStart     int    `yaml:"ip_range_start"`
	IPRangeEnd       int    `yaml:"ip_range_end"`
}

type SSHConfig struct {
	KeyPath string `yaml:"key_path"`
	User    string `yaml:"user"`
}

type DefaultsConfig struct {
	ServerType string            `yaml:"server_type"`
	Image      string            `yaml:"image"`
	Location   string            `yaml:"location"`
	Labels     map[string]string `yaml:"labels"`
}

func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	path := filepath.Join(home, ".config", "hw", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found. Run:\n  mkdir -p ~/.config/hw && $EDITOR ~/.config/hw/config.yaml")
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Env var override
	if token := os.Getenv("HCLOUD_TOKEN"); token != "" {
		cfg.HCloudToken = token
	}

	// Expand ~ in SSH key path
	if len(cfg.SSH.KeyPath) > 0 && cfg.SSH.KeyPath[0] == '~' {
		cfg.SSH.KeyPath = filepath.Join(home, cfg.SSH.KeyPath[1:])
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.HCloudToken == "" {
		return fmt.Errorf("hcloud_token is required (set in config or HCLOUD_TOKEN env var)")
	}
	if c.WireGuard.ServerPublicIP == "" {
		return fmt.Errorf("wireguard.server_public_ip is required")
	}
	if c.WireGuard.ServerPublicKey == "" {
		return fmt.Errorf("wireguard.server_public_key is required")
	}
	if c.WireGuard.ServerInternalIP == "" {
		return fmt.Errorf("wireguard.server_internal_ip is required")
	}
	if c.WireGuard.ServerPort == 0 {
		return fmt.Errorf("wireguard.server_port is required")
	}
	if c.WireGuard.Subnet == "" {
		return fmt.Errorf("wireguard.subnet is required")
	}
	if c.WireGuard.IPRangeStart < 1 || c.WireGuard.IPRangeStart > 254 {
		return fmt.Errorf("wireguard.ip_range_start must be between 1 and 254")
	}
	if c.WireGuard.IPRangeEnd < 1 || c.WireGuard.IPRangeEnd > 254 {
		return fmt.Errorf("wireguard.ip_range_end must be between 1 and 254")
	}
	if c.WireGuard.IPRangeStart >= c.WireGuard.IPRangeEnd {
		return fmt.Errorf("wireguard.ip_range_start must be less than ip_range_end")
	}
	return nil
}
