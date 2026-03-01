package mcp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// ToolHandler is a function that handles a tool call.
type ToolHandler func(arguments map[string]interface{}) (*CallToolResult, error)

// Server represents an MCP HTTPS server.
type Server struct {
	name    string
	version string
	tools   []Tool
	handlers map[string]ToolHandler
	mu      sync.RWMutex

	// Rate limiting
	toolCallTimestamps []time.Time
	rateLimitMu        sync.Mutex
	rateLimitCalls     int
	rateLimitWindow    time.Duration

	// Auth middleware (set by OAuth package)
	AuthValidator func(r *http.Request) (bool, error)

	// Additional route handlers
	extraRoutes map[string]http.Handler

	// Callbacks
	onToolCall func(name string, args map[string]interface{}, duration time.Duration, success bool)
	onError    func(err error, context string)
}

// NewServer creates a new MCP server.
func NewServer(name, version string, rateLimitCalls int, rateLimitWindow time.Duration) *Server {
	if rateLimitCalls <= 0 {
		rateLimitCalls = 10
	}
	if rateLimitWindow <= 0 {
		rateLimitWindow = 20 * time.Second
	}
	return &Server{
		name:               name,
		version:            version,
		tools:              make([]Tool, 0),
		handlers:           make(map[string]ToolHandler),
		toolCallTimestamps: make([]time.Time, 0),
		rateLimitCalls:     rateLimitCalls,
		rateLimitWindow:    rateLimitWindow,
		extraRoutes:        make(map[string]http.Handler),
	}
}

func (s *Server) SetToolCallCallback(cb func(name string, args map[string]interface{}, duration time.Duration, success bool)) {
	s.onToolCall = cb
}

func (s *Server) SetErrorCallback(cb func(err error, context string)) {
	s.onError = cb
}

// RegisterTool registers a tool with its handler.
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append(s.tools, tool)
	s.handlers[tool.Name] = handler
}

// Handle registers an additional HTTP route handler.
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extraRoutes[pattern] = handler
}

// HandleFunc registers an additional HTTP route handler function.
func (s *Server) HandleFunc(pattern string, handler http.HandlerFunc) {
	s.Handle(pattern, handler)
}

func (s *Server) checkRateLimit() bool {
	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()

	now := time.Now()
	windowStart := now.Add(-s.rateLimitWindow)

	newTimestamps := make([]time.Time, 0)
	for _, ts := range s.toolCallTimestamps {
		if ts.After(windowStart) {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	s.toolCallTimestamps = newTimestamps

	if len(s.toolCallTimestamps) >= s.rateLimitCalls {
		return true
	}

	s.toolCallTimestamps = append(s.toolCallTimestamps, now)
	return false
}

// BuildMux creates the HTTP mux with all routes.
func (s *Server) BuildMux() *http.ServeMux {
	mux := http.NewServeMux()

	// Health check (no auth)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"version": s.version,
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	// MCP endpoint (requires Bearer token)
	mux.HandleFunc("/mcp", s.handleMCPEndpoint)

	// Register extra routes
	s.mu.RLock()
	for pattern, handler := range s.extraRoutes {
		mux.Handle(pattern, handler)
	}
	s.mu.RUnlock()

	return mux
}

func (s *Server) handleMCPEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check authentication
	if s.AuthValidator != nil {
		valid, err := s.AuthValidator(r)
		if err != nil || !valid {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="/.well-known/oauth-protected-resource"`)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(&JSONRPCResponse{
				JSONRPC: "2.0",
				Error: &JSONRPCError{
					Code:    -32001,
					Message: "Unauthorized: invalid or missing Bearer token",
				},
			})
			return
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &JSONRPCError{Code: ParseError, Message: "Parse error"},
		})
		return
	}

	response := s.handleMessage(body)
	if response != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// CertInfo holds information about the current TLS certificate.
type CertInfo struct {
	Domain    string `json:"domain"`
	Issuer    string `json:"issuer"`
	NotBefore string `json:"notBefore"`
	NotAfter  string `json:"notAfter"`
	Mode      string `json:"mode"` // "acme" or "self-signed"
}

// certInfo stores the current certificate details (set during startup).
var (
	currentCertInfo   *CertInfo
	currentCertInfoMu sync.RWMutex
)

// GetCertInfo returns the current TLS certificate info.
func GetCertInfo() *CertInfo {
	currentCertInfoMu.RLock()
	defer currentCertInfoMu.RUnlock()
	if currentCertInfo == nil {
		return &CertInfo{Mode: "unknown"}
	}
	return currentCertInfo
}

func setCertInfo(info *CertInfo) {
	currentCertInfoMu.Lock()
	defer currentCertInfoMu.Unlock()
	currentCertInfo = info
}

// RunHTTPS starts the HTTPS server with ACME or self-signed certificates.
func (s *Server) RunHTTPS(ctx context.Context, domain string, httpsPort, httpPort int, useSelfSigned bool, acmeEmail string, certDir string) error {
	mux := s.BuildMux()

	httpsAddr := fmt.Sprintf(":%d", httpsPort)
	httpAddr := fmt.Sprintf(":%d", httpPort)

	var tlsConfig *tls.Config

	if useSelfSigned || domain == "" || domain == "localhost" {
		// Self-signed certificate
		cert, err := generateSelfSignedCert(domain)
		if err != nil {
			return fmt.Errorf("generate self-signed cert: %w", err)
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}

		// Parse the cert to extract info
		if parsed, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
			setCertInfo(&CertInfo{
				Domain:    domain,
				Issuer:    parsed.Issuer.CommonName,
				NotBefore: parsed.NotBefore.Format(time.RFC3339),
				NotAfter:  parsed.NotAfter.Format(time.RFC3339),
				Mode:      "self-signed",
			})
		}

		log.Printf("[TLS] Using self-signed certificate for %q", domain)
	} else {
		// ACME / Let's Encrypt
		if err := os.MkdirAll(certDir, 0700); err != nil {
			return fmt.Errorf("create cert cache dir %s: %w", certDir, err)
		}

		mgr := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(domain),
			Cache:      autocert.DirCache(certDir),
			Email:      acmeEmail,
		}

		tlsConfig = mgr.TLSConfig()
		tlsConfig.MinVersion = tls.VersionTLS12

		// Wrap GetCertificate to capture cert info on each fetch
		origGetCert := tlsConfig.GetCertificate
		tlsConfig.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert, err := origGetCert(hello)
			if err != nil {
				log.Printf("[TLS] ACME GetCertificate error for %q: %v", hello.ServerName, err)
				return nil, err
			}
			if cert != nil && len(cert.Certificate) > 0 {
				if parsed, parseErr := x509.ParseCertificate(cert.Certificate[0]); parseErr == nil {
					setCertInfo(&CertInfo{
						Domain:    domain,
						Issuer:    parsed.Issuer.CommonName,
						NotBefore: parsed.NotBefore.Format(time.RFC3339),
						NotAfter:  parsed.NotAfter.Format(time.RFC3339),
						Mode:      "acme",
					})
				}
			}
			return cert, nil
		}

		log.Printf("[TLS] Using Let's Encrypt ACME for domain %q (email: %s)", domain, acmeEmail)
		log.Printf("[TLS] Certificate cache: %s", certDir)

		setCertInfo(&CertInfo{
			Domain: domain,
			Mode:   "acme",
			Issuer: "pending (Let's Encrypt)",
		})

		// Start HTTP server for ACME HTTP-01 challenges + redirect to HTTPS.
		// mgr.HTTPHandler wraps the fallback: it intercepts challenge requests
		// and passes everything else to the redirect handler.
		redirectHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		})

		httpServer := &http.Server{
			Addr:    httpAddr,
			Handler: mgr.HTTPHandler(redirectHandler),
		}
		go func() {
			log.Printf("[HTTP] Listening on %s (ACME challenges + HTTPS redirect)", httpAddr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("[HTTP] Server error: %v", err)
				if s.onError != nil {
					s.onError(err, "http_redirect_server")
				}
			}
		}()
		go func() {
			<-ctx.Done()
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			httpServer.Shutdown(shutCtx)
		}()
	}

	httpsServer := &http.Server{
		Addr:      httpsAddr,
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	ln, err := tls.Listen("tcp", httpsAddr, tlsConfig)
	if err != nil {
		return fmt.Errorf("listen HTTPS on %s: %w", httpsAddr, err)
	}

	log.Printf("[HTTPS] Listening on %s", httpsAddr)

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpsServer.Shutdown(shutCtx)
	}()

	return httpsServer.Serve(ln)
}

func (s *Server) handleMessage(data []byte) *JSONRPCResponse {
	var request JSONRPCRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			Error: &JSONRPCError{
				Code:    ParseError,
				Message: "Parse error",
				Data:    err.Error(),
			},
		}
	}

	// Notifications have no ID
	if request.ID == nil {
		s.handleNotification(&request)
		return nil
	}

	return s.handleRequest(&request)
}

func (s *Server) handleNotification(request *JSONRPCRequest) {
	// Handle known notifications silently
}

func (s *Server) handleRequest(request *JSONRPCRequest) *JSONRPCResponse {
	response := &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      request.ID,
	}

	switch request.Method {
	case "initialize":
		response.Result = s.handleInitialize()
	case "tools/list":
		response.Result = s.handleListTools()
	case "tools/call":
		result, err := s.handleCallTool(request.Params)
		if err != nil {
			response.Error = &JSONRPCError{
				Code:    InternalError,
				Message: err.Error(),
			}
		} else {
			response.Result = result
		}
	case "ping":
		response.Result = map[string]interface{}{}
	default:
		response.Error = &JSONRPCError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("Method not found: %s", request.Method),
		}
	}

	return response
}

func (s *Server) handleInitialize() *InitializeResult {
	return &InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{ListChanged: false},
		},
		ServerInfo: ServerInfo{
			Name:    s.name,
			Version: s.version,
		},
	}
}

func (s *Server) handleListTools() *ListToolsResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &ListToolsResult{Tools: s.tools}
}

func (s *Server) handleCallTool(params interface{}) (*CallToolResult, error) {
	paramsMap, ok := params.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid params type")
	}

	name, ok := paramsMap["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing tool name")
	}

	arguments, _ := paramsMap["arguments"].(map[string]interface{})

	if s.checkRateLimit() {
		return &CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("Rate limit exceeded: Maximum %d tool calls per %s. Please try again later.", s.rateLimitCalls, s.rateLimitWindow),
			}},
			IsError: true,
		}, nil
	}

	s.mu.RLock()
	handler, exists := s.handlers[name]
	s.mu.RUnlock()

	if !exists {
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", name)}},
			IsError: true,
		}, nil
	}

	startTime := time.Now()
	result, err := handler(arguments)
	duration := time.Since(startTime)

	success := err == nil && (result == nil || !result.IsError)

	if s.onToolCall != nil {
		s.onToolCall(name, arguments, duration, success)
	}

	if err != nil {
		if s.onError != nil {
			s.onError(err, fmt.Sprintf("tool_%s", name))
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("Error: %s", err.Error())}},
			IsError: true,
		}, nil
	}

	return result, nil
}

// generateSelfSignedCert creates a self-signed TLS certificate.
func generateSelfSignedCert(domain string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Go MCP Printer"},
			CommonName:   domain,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	if domain == "" || domain == "localhost" {
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		template.DNSNames = []string{"localhost"}
	} else {
		template.DNSNames = []string{domain}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}, nil
}

// Version returns the server version.
func (s *Server) Version() string {
	return s.version
}

// Name returns the server name.
func (s *Server) Name() string {
	return s.name
}

// ToolCount returns the number of registered tools.
func (s *Server) ToolCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tools)
}

// Tools returns a copy of the registered tools.
func (s *Server) Tools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Tool, len(s.tools))
	copy(result, s.tools)
	return result
}

// FormatSSEEvent formats a JSON-RPC response as an SSE event (for future streaming support).
func FormatSSEEvent(response *JSONRPCResponse) ([]byte, error) {
	data, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return append([]byte("data: "), append(data, '\n', '\n')...), nil
}

// TextResult is a convenience constructor for a text CallToolResult.
func TextResult(text string) *CallToolResult {
	return &CallToolResult{
		Content: []ContentItem{{Type: "text", Text: text}},
	}
}

// ErrorResult is a convenience constructor for an error CallToolResult.
func ErrorResult(format string, args ...interface{}) *CallToolResult {
	return &CallToolResult{
		Content: []ContentItem{{Type: "text", Text: fmt.Sprintf(format, args...)}},
		IsError: true,
	}
}

// JSONResult marshals a value to JSON and returns it as a text result.
func JSONResult(v interface{}) (*CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return TextResult(string(data)), nil
}

// GetStringArg extracts a string argument with a default value.
func GetStringArg(args map[string]interface{}, key, defaultVal string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return defaultVal
}

// GetIntArg extracts an integer argument with a default value.
func GetIntArg(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

// GetBoolArg extracts a boolean argument with a default value.
func GetBoolArg(args map[string]interface{}, key string, defaultVal bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultVal
}

// GetStringSliceArg extracts a string slice argument.
func GetStringSliceArg(args map[string]interface{}, key string) []string {
	if v, ok := args[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

// RequireStringArg extracts a required string argument, returning an error if missing.
func RequireStringArg(args map[string]interface{}, key string) (string, error) {
	s := GetStringArg(args, key, "")
	if s == "" {
		return "", fmt.Errorf("missing required argument: %s", key)
	}
	return s, nil
}

// Pointer helpers for JSON schema
func Float64Ptr(v float64) *float64 { return &v }
func IntPtr(v int) *int             { return &v }

// ParseToolCall extracts tool name and arguments from raw params.
func ParseToolCall(params interface{}) (string, map[string]interface{}, error) {
	paramsMap, ok := params.(map[string]interface{})
	if !ok {
		return "", nil, fmt.Errorf("invalid params type")
	}
	name, ok := paramsMap["name"].(string)
	if !ok || name == "" {
		return "", nil, fmt.Errorf("missing tool name")
	}
	arguments, _ := paramsMap["arguments"].(map[string]interface{})
	if arguments == nil {
		arguments = make(map[string]interface{})
	}
	return name, arguments, nil
}

// ListenAddr returns the appropriate listen address string.
func ListenAddr(port int) string {
	return fmt.Sprintf(":%d", port)
}

// IsLocalRequest checks if a request originates from localhost.
func IsLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.TrimSpace(host)
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
