package oauth

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/logging"
)

// Server handles all OAuth 2.1 endpoints.
type Server struct {
	store   *Store
	keyPath string
	issuer  string
	logger  *logging.Logger
}

// NewServer creates a new OAuth server.
func NewServer(store *Store, keyPath, issuer string, logger *logging.Logger) *Server {
	return &Server{
		store:   store,
		keyPath: keyPath,
		issuer:  issuer,
		logger:  logger,
	}
}

// RouteRegistrar is an interface for registering HTTP routes.
type RouteRegistrar interface {
	HandleFunc(pattern string, handler http.HandlerFunc)
}

// RegisterRoutes registers all OAuth routes.
func (s *Server) RegisterRoutes(r RouteRegistrar) {
	r.HandleFunc("/.well-known/oauth-protected-resource", ProtectedResourceMetadata(s.issuer))
	r.HandleFunc("/.well-known/oauth-authorization-server", AuthorizationServerMetadata(s.issuer))
	r.HandleFunc("/authorize", s.handleAuthorize)
	r.HandleFunc("/token", s.handleToken)
	r.HandleFunc("/register", s.handleRegister)
	r.HandleFunc("/jwks", s.handleJWKS)
	r.HandleFunc("/revoke", s.handleRevoke)
}

// ValidateRequest checks the Bearer token on an HTTP request.
func (s *Server) ValidateRequest(r *http.Request) (bool, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false, nil
	}

	if !strings.HasPrefix(auth, "Bearer ") {
		return false, nil
	}

	tokenStr := strings.TrimPrefix(auth, "Bearer ")

	key := GetSigningKey()
	if key == nil {
		return false, fmt.Errorf("signing key not loaded")
	}

	claims, err := VerifyJWT(tokenStr, &key.PublicKey)
	if err != nil {
		return false, err
	}

	if s.store.IsRevoked(claims.JWTID) {
		return false, fmt.Errorf("token revoked")
	}

	return true, nil
}

func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.showConsentPage(w, r)
	case http.MethodPost:
		s.processConsent(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) showConsentPage(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	state := r.URL.Query().Get("state")
	scope := r.URL.Query().Get("scope")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")

	if clientID == "" || redirectURI == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	client := s.store.GetClient(clientID)
	if client == nil {
		http.Error(w, "Unknown client", http.StatusBadRequest)
		return
	}

	// Validate redirect URI
	validURI := false
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			validURI = true
			break
		}
	}
	if !validURI {
		http.Error(w, "Invalid redirect_uri", http.StatusBadRequest)
		return
	}

	if scope == "" {
		scope = "mcp:tools"
	}

	clientName := client.ClientName
	if clientName == "" {
		clientName = clientID
	}

	RenderConsent(w, ConsentData{
		ClientID:            clientID,
		ClientName:          clientName,
		RedirectURI:         redirectURI,
		State:               state,
		Scope:               scope,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
	})
}

func (s *Server) processConsent(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	action := r.FormValue("action")
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")
	scope := r.FormValue("scope")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")

	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, "Invalid redirect_uri", http.StatusBadRequest)
		return
	}

	q := redirectURL.Query()
	if state != "" {
		q.Set("state", state)
	}

	if action != "approve" {
		q.Set("error", "access_denied")
		q.Set("error_description", "User denied the request")
		redirectURL.RawQuery = q.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
		s.logger.OAuthEvent("CONSENT_DENIED", clientID, nil)
		return
	}

	code, err := s.store.CreateAuthCode(clientID, redirectURI, scope, codeChallenge, codeChallengeMethod)
	if err != nil {
		q.Set("error", "server_error")
		redirectURL.RawQuery = q.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
		s.logger.OAuthEvent("CODE_CREATE_FAILED", clientID, err)
		return
	}

	q.Set("code", code)
	redirectURL.RawQuery = q.Encode()
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
	s.logger.OAuthEvent("CODE_ISSUED", clientID, nil)
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		jsonError(w, "invalid_request", "Invalid form data", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")
	if grantType != "authorization_code" {
		jsonError(w, "unsupported_grant_type", "Only authorization_code is supported", http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")

	ac, err := s.store.ExchangeCode(code, clientID, redirectURI, codeVerifier)
	if err != nil {
		s.logger.OAuthEvent("TOKEN_EXCHANGE_FAILED", clientID, err)
		jsonError(w, "invalid_grant", err.Error(), http.StatusBadRequest)
		return
	}

	key := GetSigningKey()
	if key == nil {
		jsonError(w, "server_error", "Signing key not available", http.StatusInternalServerError)
		return
	}

	token, expiresIn, err := CreateAccessToken(s.issuer, ac.ClientID, ac.Scope, key)
	if err != nil {
		jsonError(w, "server_error", "Failed to create token", http.StatusInternalServerError)
		return
	}

	s.logger.OAuthEvent("TOKEN_ISSUED", clientID, nil)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
		"scope":        ac.Scope,
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
		GrantTypes   []string `json:"grant_types"`
		Scope        string   `json:"scope"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid_request", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if len(req.RedirectURIs) == 0 {
		jsonError(w, "invalid_request", "redirect_uris is required", http.StatusBadRequest)
		return
	}

	client, err := s.store.RegisterClient(req.ClientName, req.RedirectURIs)
	if err != nil {
		jsonError(w, "server_error", "Failed to register client", http.StatusInternalServerError)
		return
	}

	s.logger.OAuthEvent("CLIENT_REGISTERED", client.ClientID, nil)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"client_id":      client.ClientID,
		"client_name":    client.ClientName,
		"redirect_uris":  client.RedirectURIs,
		"grant_types":    client.GrantTypes,
		"scope":          client.Scope,
	})
}

func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	key := GetSigningKey()
	if key == nil {
		http.Error(w, "Key not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(JWKS(&key.PublicKey))
}

func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK) // Revocation always returns 200
		return
	}

	tokenStr := r.FormValue("token")
	if tokenStr == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	key := GetSigningKey()
	if key == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	claims, err := VerifyJWT(tokenStr, &key.PublicKey)
	if err != nil {
		// Token might already be invalid, that's OK for revocation
		w.WriteHeader(http.StatusOK)
		return
	}

	s.store.RevokeToken(claims.JWTID)
	s.logger.OAuthEvent("TOKEN_REVOKED", claims.ClientID, nil)
	w.WriteHeader(http.StatusOK)
}

// Store returns the underlying OAuth store (for admin API access).
func (s *Server) Store() *Store {
	return s.store
}

// GetPublicKey returns the current public key (for admin API).
func (s *Server) GetPublicKey() *rsa.PublicKey {
	key := GetSigningKey()
	if key == nil {
		return nil
	}
	return &key.PublicKey
}

func jsonError(w http.ResponseWriter, errorType, description string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errorType,
		"error_description": description,
	})
}
