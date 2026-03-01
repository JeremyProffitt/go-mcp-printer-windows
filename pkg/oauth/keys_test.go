package oauth

import (
	"crypto/rsa"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadOrGenerateKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key.pem")

	// First call should generate a new key
	key1, err := LoadOrGenerateKey(keyPath)
	if err != nil {
		t.Fatalf("LoadOrGenerateKey() error = %v", err)
	}
	if key1 == nil {
		t.Fatal("Expected non-nil key")
	}

	// File should exist
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatal("Key file should exist")
	}

	// Reset signingKey to force reload
	signingKeyMu.Lock()
	signingKey = nil
	signingKeyMu.Unlock()

	// Second call should load the same key
	key2, err := LoadOrGenerateKey(keyPath)
	if err != nil {
		t.Fatalf("LoadOrGenerateKey() second call error = %v", err)
	}

	if key1.N.Cmp(key2.N) != 0 {
		t.Error("Keys should be the same after reload")
	}
}

func TestRegenerateKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key.pem")

	key1, err := LoadOrGenerateKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}

	key2, err := RegenerateKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}

	if key1.N.Cmp(key2.N) == 0 {
		t.Error("Regenerated key should be different")
	}
}

func TestSignAndVerifyJWT(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key.pem")

	key, err := LoadOrGenerateKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}

	claims := &JWTClaims{
		Issuer:    "https://test.example.com",
		Subject:   "test-client",
		Audience:  "https://test.example.com",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		IssuedAt:  time.Now().Unix(),
		JWTID:     "test-jti",
		Scope:     "mcp:tools",
		ClientID:  "test-client",
	}

	token, err := SignJWT(claims, key)
	if err != nil {
		t.Fatalf("SignJWT() error = %v", err)
	}

	if token == "" {
		t.Fatal("Token should not be empty")
	}

	// Verify
	verified, err := VerifyJWT(token, &key.PublicKey)
	if err != nil {
		t.Fatalf("VerifyJWT() error = %v", err)
	}

	if verified.Issuer != claims.Issuer {
		t.Errorf("Issuer = %q, want %q", verified.Issuer, claims.Issuer)
	}
	if verified.Subject != claims.Subject {
		t.Errorf("Subject = %q, want %q", verified.Subject, claims.Subject)
	}
	if verified.Scope != claims.Scope {
		t.Errorf("Scope = %q, want %q", verified.Scope, claims.Scope)
	}
}

func TestVerifyExpiredJWT(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key.pem")

	key, err := LoadOrGenerateKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}

	claims := &JWTClaims{
		Issuer:    "https://test.example.com",
		ExpiresAt: time.Now().Add(-time.Hour).Unix(), // Already expired
		IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
		JWTID:     "expired-jti",
	}

	token, err := SignJWT(claims, key)
	if err != nil {
		t.Fatal(err)
	}

	_, err = VerifyJWT(token, &key.PublicKey)
	if err == nil {
		t.Error("Should reject expired token")
	}
}

func TestVerifyJWTWrongKey(t *testing.T) {
	tmpDir := t.TempDir()

	key1, err := LoadOrGenerateKey(filepath.Join(tmpDir, "key1.pem"))
	if err != nil {
		t.Fatal(err)
	}

	// Reset for second key
	signingKeyMu.Lock()
	signingKey = nil
	signingKeyMu.Unlock()

	key2, err := LoadOrGenerateKey(filepath.Join(tmpDir, "key2.pem"))
	if err != nil {
		t.Fatal(err)
	}

	claims := &JWTClaims{
		Issuer:    "https://test.example.com",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		IssuedAt:  time.Now().Unix(),
		JWTID:     "test-jti",
	}

	// Sign with key1
	token, err := SignJWT(claims, key1)
	if err != nil {
		t.Fatal(err)
	}

	// Verify with key2 should fail
	_, err = VerifyJWT(token, &key2.PublicKey)
	if err == nil {
		t.Error("Should reject token signed with different key")
	}
}

func TestJWKS(t *testing.T) {
	tmpDir := t.TempDir()
	key, err := LoadOrGenerateKey(filepath.Join(tmpDir, "test_key.pem"))
	if err != nil {
		t.Fatal(err)
	}

	jwks := JWKS(&key.PublicKey)
	keys, ok := jwks["keys"].([]map[string]interface{})
	if !ok || len(keys) != 1 {
		t.Fatal("Expected one key in JWKS")
	}

	k := keys[0]
	if k["kty"] != "RSA" {
		t.Errorf("kty = %v, want RSA", k["kty"])
	}
	if k["alg"] != "RS256" {
		t.Errorf("alg = %v, want RS256", k["alg"])
	}
}

func TestCreateAccessToken(t *testing.T) {
	tmpDir := t.TempDir()
	key, err := LoadOrGenerateKey(filepath.Join(tmpDir, "test_key.pem"))
	if err != nil {
		t.Fatal(err)
	}

	token, expiresIn, err := CreateAccessToken("https://test.example.com", "client123", "mcp:tools", key)
	if err != nil {
		t.Fatalf("CreateAccessToken() error = %v", err)
	}

	if token == "" {
		t.Error("Token should not be empty")
	}
	if expiresIn != 3600 {
		t.Errorf("expiresIn = %d, want 3600", expiresIn)
	}

	// Verify the token
	claims, err := VerifyJWT(token, &key.PublicKey)
	if err != nil {
		t.Fatalf("Token verification failed: %v", err)
	}
	if claims.ClientID != "client123" {
		t.Errorf("ClientID = %q, want %q", claims.ClientID, "client123")
	}
}

// Ensure rsa import is used
var _ *rsa.PrivateKey
