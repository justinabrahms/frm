package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Endpoint string `json:"endpoint"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func configDir() string {
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
	if cfg.Endpoint == "" || cfg.Username == "" || cfg.Password == "" {
		return cfg, fmt.Errorf("config must include endpoint, username, and password")
	}
	return cfg, nil
}
