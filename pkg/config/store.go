package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	mu     sync.Mutex
	cached *Config
)

// Load reads the config from disk, or returns defaults if the file doesn't exist.
func Load() (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	if cached != nil {
		return cached, nil
	}

	cfg := DefaultConfig()
	path := ConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cached = cfg
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Apply defaults for zero values
	if cfg.HTTPSPort == 0 {
		cfg.HTTPSPort = 443
	}
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 80
	}
	if cfg.AdminPort == 0 {
		cfg.AdminPort = 8787
	}
	if cfg.RateLimitCalls == 0 {
		cfg.RateLimitCalls = 10
	}
	if cfg.RateLimitWindow == 0 {
		cfg.RateLimitWindow = 20
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.LogDir == "" {
		cfg.LogDir = filepath.Join(DefaultDataDir(), "logs")
	}

	cached = cfg
	return cfg, nil
}

// Save writes the config to disk.
func Save(cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()

	path := ConfigPath()
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Write atomically: write to temp file, then rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename config: %w", err)
	}

	cached = cfg
	return nil
}

// Reload forces a reload of the config from disk.
func Reload() (*Config, error) {
	mu.Lock()
	cached = nil
	mu.Unlock()
	return Load()
}
