package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/config"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/dns"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/mcp"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/oauth"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/printer"
)

func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(h.cfg)

	case http.MethodPost:
		var newCfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Preserve admin password if not provided
		if newCfg.AdminPassword == "" {
			newCfg.AdminPassword = h.cfg.AdminPassword
		}
		if newCfg.LogDir == "" {
			newCfg.LogDir = h.cfg.LogDir
		}

		if err := config.Save(&newCfg); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save: %s", err), http.StatusInternalServerError)
			return
		}

		*h.cfg = newCfg
		h.logger.Info("Config updated via admin API")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handlePrinters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	printers, err := printer.ListPrinters()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list printers: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printers)
}

func (h *Handler) handlePrintTestAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	results := printer.PrintAllTestPages()
	h.logger.Info("Test pages sent to all printers via admin")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (h *Handler) handlePrinterPaperSizes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	printers, err := printer.ListPrintersWithPaperSizes()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printers)
}

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logDir := h.cfg.LogDir
	if logDir == "" {
		logDir = config.DefaultConfig().LogDir
	}

	// Find latest log file
	pattern := filepath.Join(logDir, "*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("No log files found"))
		return
	}

	// Read last log file (sorted by name, which includes date)
	latestLog := matches[len(matches)-1]
	data, err := os.ReadFile(latestLog)
	if err != nil {
		http.Error(w, "Failed to read log", http.StatusInternalServerError)
		return
	}

	// Return last 500 lines
	lines := strings.Split(string(data), "\n")
	start := 0
	if len(lines) > 500 {
		start = len(lines) - 500
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(strings.Join(lines[start:], "\n")))
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uptime := time.Since(h.startTime)
	certInfo := mcp.GetCertInfo()
	status := map[string]interface{}{
		"version":       h.version,
		"uptime":        uptime.String(),
		"domain":        h.cfg.Domain,
		"httpsPort":     h.cfg.HTTPSPort,
		"httpPort":      h.cfg.HTTPPort,
		"useSelfSigned": h.cfg.UseSelfSigned,
		"goVersion":     runtime.Version(),
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"numCPU":        runtime.NumCPU(),
		"pid":           os.Getpid(),
		"logLevel":      h.cfg.LogLevel,
		"certificate":   certInfo,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (h *Handler) handleOAuthClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.oauthServer == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*oauth.OAuthClient{})
		return
	}

	clients := h.oauthServer.Store().ListClients()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clients)
}

func (h *Handler) handleOAuthClientDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract client ID from path: /admin/api/oauth/clients/{id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		http.Error(w, "Missing client ID", http.StatusBadRequest)
		return
	}
	clientID := parts[len(parts)-1]

	if h.oauthServer == nil {
		http.Error(w, "OAuth not configured", http.StatusInternalServerError)
		return
	}

	if err := h.oauthServer.Store().DeleteClient(clientID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete client: %s", err), http.StatusInternalServerError)
		return
	}

	h.logger.Info("OAuth client deleted via admin: %s", clientID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) handleKeyRegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keyPath := config.OAuthKeyPath()
	if _, err := oauth.RegenerateKey(keyPath); err != nil {
		http.Error(w, fmt.Sprintf("Failed to regenerate keys: %s", err), http.StatusInternalServerError)
		return
	}

	h.logger.Info("OAuth signing keys regenerated via admin")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) handleDNSStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result := map[string]interface{}{
		"config": map[string]interface{}{
			"enabled":        h.cfg.DNSEnabled,
			"domain":         h.cfg.DNSDomain,
			"hasCredentials": h.cfg.AWSAccessKeyID != "" && h.cfg.AWSSecretAccessKey != "",
			"intervalSecs":   h.cfg.DNSUpdateInterval,
		},
	}

	if h.dnsUpdater != nil {
		result["updater"] = h.dnsUpdater.GetStatus()
	}

	// Get current public IP
	ip, err := dns.GetPublicIP()
	if err == nil {
		result["currentPublicIp"] = ip
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *Handler) handleDNSConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Return DNS config (mask secret key)
		resp := map[string]interface{}{
			"dnsEnabled":        h.cfg.DNSEnabled,
			"dnsDomain":         h.cfg.DNSDomain,
			"awsAccessKeyId":    h.cfg.AWSAccessKeyID,
			"hasSecretKey":      h.cfg.AWSSecretAccessKey != "",
			"dnsUpdateInterval": h.cfg.DNSUpdateInterval,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

	case http.MethodPost:
		var req struct {
			DNSEnabled         bool   `json:"dnsEnabled"`
			DNSDomain          string `json:"dnsDomain"`
			AWSAccessKeyID     string `json:"awsAccessKeyId"`
			AWSSecretAccessKey string `json:"awsSecretAccessKey"`
			DNSUpdateInterval  int    `json:"dnsUpdateInterval"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		h.cfg.DNSEnabled = req.DNSEnabled
		h.cfg.DNSDomain = req.DNSDomain
		h.cfg.AWSAccessKeyID = req.AWSAccessKeyID
		if req.AWSSecretAccessKey != "" {
			h.cfg.AWSSecretAccessKey = req.AWSSecretAccessKey
		}
		if req.DNSUpdateInterval > 0 {
			h.cfg.DNSUpdateInterval = req.DNSUpdateInterval
		}

		if err := config.Save(h.cfg); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save: %s", err), http.StatusInternalServerError)
			return
		}

		// Restart or stop the updater based on enabled state
		if h.dnsUpdater != nil {
			if h.cfg.DNSEnabled && h.cfg.AWSAccessKeyID != "" && h.cfg.AWSSecretAccessKey != "" && h.cfg.DNSDomain != "" {
				if err := h.dnsUpdater.Start(
					h.dnsCtx,
					h.cfg.AWSAccessKeyID,
					h.cfg.AWSSecretAccessKey,
					h.cfg.DNSDomain,
					h.cfg.DNSUpdateInterval,
				); err != nil {
					h.logger.Error("Failed to start DNS updater: %v", err)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]interface{}{
						"status": "saved",
						"error":  fmt.Sprintf("Config saved but DNS updater failed to start: %v", err),
					})
					return
				}
				h.logger.Info("DNS updater started for %s", h.cfg.DNSDomain)
			} else {
				h.dnsUpdater.Stop()
				h.logger.Info("DNS updater stopped")
			}
		}

		h.logger.Info("DNS config updated via admin API")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleDNSTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.cfg.AWSAccessKeyID == "" || h.cfg.AWSSecretAccessKey == "" {
		http.Error(w, "AWS credentials not configured", http.StatusBadRequest)
		return
	}
	if h.cfg.DNSDomain == "" {
		http.Error(w, "DNS domain not configured", http.StatusBadRequest)
		return
	}

	updater := dns.NewUpdater(func(msg string) {
		h.logger.Info("[DNS] %s", msg)
	})

	result, err := updater.RunOnce(h.cfg.AWSAccessKeyID, h.cfg.AWSSecretAccessKey, h.cfg.DNSDomain)
	if err != nil {
		http.Error(w, fmt.Sprintf("DNS update failed: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "result": result})
}

func (h *Handler) handleDNSPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try to resolve the hosted zone ID for a more specific policy
	hostedZoneID := ""
	if h.dnsUpdater != nil {
		status := h.dnsUpdater.GetStatus()
		hostedZoneID = status.HostedZoneID
	}

	policy := dns.GenerateIAMPolicy(hostedZoneID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"policy": policy})
}
