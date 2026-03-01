package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// OAuthClient represents a dynamically registered OAuth client.
type OAuthClient struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
	GrantTypes   []string `json:"grant_types"`
	Scope        string   `json:"scope,omitempty"`
	CreatedAt    int64    `json:"created_at"`
}

// AuthCode represents an authorization code.
type AuthCode struct {
	Code                string `json:"code"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	Scope               string `json:"scope"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	ExpiresAt           int64  `json:"expires_at"`
	Used                bool   `json:"used"`
}

// Store holds OAuth state (clients, codes, revoked tokens).
type Store struct {
	mu       sync.RWMutex
	path     string
	Clients  map[string]*OAuthClient `json:"clients"`
	Codes    map[string]*AuthCode    `json:"codes"`
	Revoked  map[string]int64        `json:"revoked"` // jti -> revoked_at
}

// NewStore creates or loads an OAuth store.
func NewStore(path string) (*Store, error) {
	s := &Store{
		path:    path,
		Clients: make(map[string]*OAuthClient),
		Codes:   make(map[string]*AuthCode),
		Revoked: make(map[string]int64),
	}

	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, s)
	}

	return s, nil
}

func (s *Store) save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

// RegisterClient dynamically registers a new OAuth client.
func (s *Store) RegisterClient(name string, redirectURIs []string) (*OAuthClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	client := &OAuthClient{
		ClientID:     generateID(),
		ClientName:   name,
		RedirectURIs: redirectURIs,
		GrantTypes:   []string{"authorization_code"},
		Scope:        "mcp:tools",
		CreatedAt:    time.Now().Unix(),
	}

	s.Clients[client.ClientID] = client

	if err := s.save(); err != nil {
		return nil, fmt.Errorf("save store: %w", err)
	}

	return client, nil
}

// GetClient returns a client by ID.
func (s *Store) GetClient(clientID string) *OAuthClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Clients[clientID]
}

// DeleteClient removes a client.
func (s *Store) DeleteClient(clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.Clients, clientID)
	return s.save()
}

// ListClients returns all registered clients.
func (s *Store) ListClients() []*OAuthClient {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*OAuthClient, 0, len(s.Clients))
	for _, c := range s.Clients {
		result = append(result, c)
	}
	return result
}

// CreateAuthCode creates a new authorization code.
func (s *Store) CreateAuthCode(clientID, redirectURI, scope, codeChallenge, codeChallengeMethod string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	code := generateID()
	s.Codes[code] = &AuthCode{
		Code:                code,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scope:               scope,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           time.Now().Add(10 * time.Minute).Unix(),
	}

	if err := s.save(); err != nil {
		return "", err
	}

	return code, nil
}

// ExchangeCode validates and consumes an authorization code.
func (s *Store) ExchangeCode(code, clientID, redirectURI, codeVerifier string) (*AuthCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ac, ok := s.Codes[code]
	if !ok {
		return nil, fmt.Errorf("invalid authorization code")
	}

	if ac.Used {
		return nil, fmt.Errorf("authorization code already used")
	}

	if time.Now().Unix() > ac.ExpiresAt {
		delete(s.Codes, code)
		s.save()
		return nil, fmt.Errorf("authorization code expired")
	}

	if ac.ClientID != clientID {
		return nil, fmt.Errorf("client_id mismatch")
	}

	if ac.RedirectURI != redirectURI {
		return nil, fmt.Errorf("redirect_uri mismatch")
	}

	// Verify PKCE
	if ac.CodeChallengeMethod == "S256" {
		h := sha256.Sum256([]byte(codeVerifier))
		challenge := base64.RawURLEncoding.EncodeToString(h[:])
		if challenge != ac.CodeChallenge {
			return nil, fmt.Errorf("PKCE verification failed")
		}
	} else if ac.CodeChallenge != "" {
		return nil, fmt.Errorf("unsupported code_challenge_method")
	}

	ac.Used = true
	s.save()

	return ac, nil
}

// RevokeToken marks a token (by JTI) as revoked.
func (s *Store) RevokeToken(jti string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Revoked[jti] = time.Now().Unix()
	return s.save()
}

// IsRevoked checks if a token JTI has been revoked.
func (s *Store) IsRevoked(jti string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, revoked := s.Revoked[jti]
	return revoked
}

// Cleanup removes expired codes and old revocations.
func (s *Store) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	for code, ac := range s.Codes {
		if now > ac.ExpiresAt+3600 { // keep 1 hour past expiry
			delete(s.Codes, code)
		}
	}

	// Clean up revocations older than 24 hours
	cutoff := now - 86400
	for jti, revokedAt := range s.Revoked {
		if revokedAt < cutoff {
			delete(s.Revoked, jti)
		}
	}

	s.save()
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
