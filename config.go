package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Services []ServiceConfig `json:"services"`
}

type ServiceConfig struct {
	Type string `json:"type"`
	// CardDAV fields
	Endpoint string `json:"endpoint,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	// JMAP fields
	SessionEndpoint string `json:"session_endpoint,omitempty"`
	Token           string `json:"token,omitempty"`
	MaxResults      int    `json:"max_results,omitempty"`
}

func (cfg Config) carddavServices() []ServiceConfig {
	var out []ServiceConfig
	for _, s := range cfg.Services {
		if s.Type == "carddav" {
			out = append(out, s)
		}
	}
	return out
}

func (cfg Config) jmapServices() []ServiceConfig {
	var out []ServiceConfig
	for _, s := range cfg.Services {
		if s.Type == "jmap" {
			out = append(out, s)
		}
	}
	return out
}

func configDir() string {
	if dir := os.Getenv("FRM_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".frm")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

func loadConfig() (Config, error) {
	var cfg Config
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg, fmt.Errorf("cannot read config file %s: %w\nCreate it with your CardDAV credentials.", configPath(), err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid config JSON: %w", err)
	}

	knownTypes := map[string]bool{"carddav": true, "jmap": true}
	for i, svc := range cfg.Services {
		if svc.Type == "" {
			return cfg, fmt.Errorf("service %d has no type (must be \"carddav\" or \"jmap\")", i)
		}
		if !knownTypes[svc.Type] {
			fmt.Fprintf(os.Stderr, "WARNING: service %d has unknown type %q (expected \"carddav\" or \"jmap\") — skipping\n", i, svc.Type)
		}
	}

	carddavSvcs := cfg.carddavServices()
	if len(carddavSvcs) == 0 {
		return cfg, fmt.Errorf("config must include at least one carddav service")
	}
	for i, svc := range carddavSvcs {
		if svc.Endpoint == "" || svc.Username == "" || svc.Password == "" {
			return cfg, fmt.Errorf("carddav service %d must include endpoint, username, and password", i)
		}
	}
	for i, svc := range cfg.jmapServices() {
		if svc.SessionEndpoint == "" || svc.Token == "" {
			return cfg, fmt.Errorf("jmap service %d must include session_endpoint and token", i)
		}
	}
	return cfg, nil
}
