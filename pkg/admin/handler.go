package admin

import (
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/config"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/logging"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/mcp"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/oauth"
)

// Handler serves the admin UI and API.
type Handler struct {
	cfg         *config.Config
	logger      *logging.Logger
	oauthServer *oauth.Server
	version     string
	startTime   time.Time

	// Sessions
	sessionMu sync.RWMutex
	sessions  map[string]time.Time
}

// NewHandler creates a new admin handler.
func NewHandler(cfg *config.Config, logger *logging.Logger, oauthServer *oauth.Server, version string) *Handler {
	return &Handler{
		cfg:         cfg,
		logger:      logger,
		oauthServer: oauthServer,
		version:     version,
		startTime:   time.Now(),
		sessions:    make(map[string]time.Time),
	}
}

// RegisterRoutes registers admin routes on the mux.
func (h *Handler) RegisterRoutes(s *mcp.Server) {
	webContent, _ := fs.Sub(webFS, "web")
	fileServer := http.FileServer(http.FS(webContent))

	// Static files
	s.HandleFunc("/admin/", func(w http.ResponseWriter, r *http.Request) {
		// Serve index.html for /admin/ path
		if r.URL.Path == "/admin/" || r.URL.Path == "/admin" {
			r.URL.Path = "/index.html"
		} else {
			// Strip /admin/ prefix for file serving
			r.URL.Path = r.URL.Path[len("/admin"):]
		}
		fileServer.ServeHTTP(w, r)
	})

	// Login
	s.HandleFunc("/admin/login", h.handleLogin)

	// API endpoints (auth required for non-localhost)
	s.HandleFunc("/admin/api/config", h.requireAuth(h.handleConfig))
	s.HandleFunc("/admin/api/printers", h.requireAuth(h.handlePrinters))
	s.HandleFunc("/admin/api/logs", h.requireAuth(h.handleLogs))
	s.HandleFunc("/admin/api/status", h.requireAuth(h.handleStatus))
	s.HandleFunc("/admin/api/oauth/clients", h.requireAuth(h.handleOAuthClients))
	s.HandleFunc("/admin/api/oauth/clients/", h.requireAuth(h.handleOAuthClientDelete))
	s.HandleFunc("/admin/api/oauth/keys/regenerate", h.requireAuth(h.handleKeyRegenerate))
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Localhost access is always allowed
		if mcp.IsLocalRequest(r) {
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
		Secure:   true,
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
