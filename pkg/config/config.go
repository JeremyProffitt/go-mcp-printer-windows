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
	HTTPSPort       int      `json:"httpsPort"`
	HTTPPort        int      `json:"httpPort"`
	UseSelfSigned   bool     `json:"useSelfSigned"`
	ACMEEmail       string   `json:"acmeEmail"`
	LogDir          string   `json:"logDir"`
	LogLevel        string   `json:"logLevel"`
	DefaultPrinter  string   `json:"defaultPrinter"`
	AllowedPrinters []string `json:"allowedPrinters"`
	BlockedPrinters []string `json:"blockedPrinters"`
	AllowedPaths    []string `json:"allowedPaths"`
	AdminPassword   string   `json:"adminPassword"`
	RateLimitCalls  int      `json:"rateLimitCalls"`
	RateLimitWindow int      `json:"rateLimitWindow"`

	// Admin UI
	AdminPort int `json:"adminPort"`

	// DNS / Route 53
	DNSEnabled         bool   `json:"dnsEnabled"`
	DNSDomain          string `json:"dnsDomain"`
	AWSAccessKeyID     string `json:"awsAccessKeyId"`
	AWSSecretAccessKey string `json:"awsSecretAccessKey"`
	DNSUpdateInterval  int    `json:"dnsUpdateInterval"` // seconds
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Domain:          "localhost",
		HTTPSPort:       443,
		HTTPPort:        80,
		UseSelfSigned:   true,
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

// OAuthKeyPath returns the path to the OAuth private key.
func OAuthKeyPath() string {
	return filepath.Join(DefaultDataDir(), "oauth_private_key.pem")
}

// OAuthStorePath returns the path to the OAuth store file.
func OAuthStorePath() string {
	return filepath.Join(DefaultDataDir(), "oauth_store.json")
}

// CertDir returns the directory for certificate storage.
func CertDir() string {
	return filepath.Join(DefaultDataDir(), "certs")
}
