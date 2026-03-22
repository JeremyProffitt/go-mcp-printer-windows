package config

import (
	"os"
	"path/filepath"
)

const (
	AppName    = "go-mcp-printer-windows"
	DataDir    = "go-mcp-printer-windows"
	ServiceName = "GoMCPPrinter"
)

// Config holds all server configuration.
type Config struct {
	Domain          string   `json:"domain"`
	Port            int      `json:"port"`
	LogDir          string   `json:"logDir"`
	LogLevel        string   `json:"logLevel"`
	DefaultPrinter  string   `json:"defaultPrinter"`
	AllowedPrinters []string `json:"allowedPrinters"`
	BlockedPrinters []string `json:"blockedPrinters"`
	PhotoPrinters   []string `json:"photoPrinters"`
	AllowedPaths    []string `json:"allowedPaths"`
	AdminPassword   string   `json:"adminPassword"`
	RateLimitCalls  int      `json:"rateLimitCalls"`
	RateLimitWindow int      `json:"rateLimitWindow"`

	// Admin UI
	AdminPort int `json:"adminPort"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Domain:          "localhost",
		Port:            80,
		LogDir:          filepath.Join(DefaultDataDir(), "logs"),
		LogLevel:        "info",
		RateLimitCalls:  10,
		RateLimitWindow: 20,
		AdminPort:       8787,
	}
}

// DefaultDataDir returns the default data directory path.
func DefaultDataDir() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, DataDir)
}

// ConfigPath returns the path to the config file.
func ConfigPath() string {
	return filepath.Join(DefaultDataDir(), "config.json")
}


