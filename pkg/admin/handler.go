package admin

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/config"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/logging"
)

// Handler serves the admin UI and API.
type Handler struct {
	cfg       *config.Config
	logger    *logging.Logger
	version   string
	startTime time.Time
	restartCh chan struct{}

	// Sessions
	sessionMu sync.RWMutex
	sessions  map[string]time.Time
}

// NewHandler creates a new admin handler.
func NewHandler(cfg *config.Config, logger *logging.Logger, version string, restartCh chan struct{}) *Handler {
	return &Handler{
		cfg:       cfg,
		logger:    logger,
		version:   version,
		startTime: time.Now(),
		restartCh: restartCh,
		sessions:  make(map[string]time.Time),
	}
}

// RegisterRoutes registers admin routes on the given ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	webContent, _ := fs.Sub(webFS, "web")
	fileServer := http.FileServer(http.FS(webContent))

	// Static files — serve index.html directly for the root path to avoid
	// http.FileServer's automatic /index.html → ./ redirect loop.
	mux.HandleFunc("/admin/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/" || r.URL.Path == "/admin" {
			f, err := webContent.Open("index.html")
			if err != nil {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}
			defer f.Close()
			stat, _ := f.Stat()
			http.ServeContent(w, r, "index.html", stat.ModTime(), f.(io.ReadSeeker))
			return
		}
		// Strip /admin/ prefix for file serving
		r.URL.Path = r.URL.Path[len("/admin"):]
		fileServer.ServeHTTP(w, r)
	})

	// Login
	mux.HandleFunc("/admin/login", h.handleLogin)

	// API endpoints (auth required for non-localhost)
	mux.HandleFunc("/admin/api/config", h.requireAuth(h.handleConfig))
	mux.HandleFunc("/admin/api/printers", h.requireAuth(h.handlePrinters))
	mux.HandleFunc("/admin/api/printers/paper-sizes", h.requireAuth(h.handlePrinterPaperSizes))
	mux.HandleFunc("/admin/api/printers/test-all", h.requireAuth(h.handlePrintTestAll))
	mux.HandleFunc("/admin/api/logs", h.requireAuth(h.handleLogs))
	mux.HandleFunc("/admin/api/status", h.requireAuth(h.handleStatus))
	mux.HandleFunc("/admin/api/restart", h.requireAuth(h.handleRestart))
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Localhost access is always allowed
		if isLocalRequest(r) {
			next(w, r)
			return
		}

		// Check session cookie
		cookie, err := r.Cookie("admin_session")
		if err == nil {
			h.sessionMu.RLock()
			expiry, valid := h.sessions[cookie.Value]
			h.sessionMu.RUnlock()
			if valid && time.Now().Before(expiry) {
				next(w, r)
				return
			}
		}

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin/login.html", http.StatusFound)
		return
	}

	password := r.FormValue("password")
	if h.cfg.AdminPassword == "" || !checkPassword(password, h.cfg.AdminPassword) {
		http.Redirect(w, r, "/admin/login.html?error=1", http.StatusFound)
		return
	}

	// Create session
	sessionID := generateSessionID()
	h.sessionMu.Lock()
	h.sessions[sessionID] = time.Now().Add(24 * time.Hour)
	h.sessionMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    sessionID,
		Path:     "/admin",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400,
	})

	http.Redirect(w, r, "/admin/", http.StatusFound)
}

func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// checkPassword compares plaintext with stored bcrypt hash.
// For simplicity, using plain comparison. In production, use bcrypt.
func checkPassword(plain, stored string) bool {
	// TODO: implement bcrypt comparison when admin sets password
	return plain == stored
}

// isLocalRequest checks if a request originates from localhost.
func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.TrimSpace(host)
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
