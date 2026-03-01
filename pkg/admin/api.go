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
	status := map[string]interface{}{
		"version":      h.version,
		"uptime":       uptime.String(),
		"domain":       h.cfg.Domain,
		"httpsPort":    h.cfg.HTTPSPort,
		"useSelfSigned": h.cfg.UseSelfSigned,
		"goVersion":    runtime.Version(),
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"numCPU":       runtime.NumCPU(),
		"pid":          os.Getpid(),
		"logLevel":     h.cfg.LogLevel,
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
