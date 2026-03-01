package oauth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	signingKey   *rsa.PrivateKey
	signingKeyMu sync.RWMutex
)

// LoadOrGenerateKey loads the RSA private key from disk or generates a new one.
func LoadOrGenerateKey(keyPath string) (*rsa.PrivateKey, error) {
	signingKeyMu.Lock()
	defer signingKeyMu.Unlock()

	data, err := os.ReadFile(keyPath)
	if err == nil {
		block, _ := pem.Decode(data)
		if block != nil && block.Type == "RSA PRIVATE KEY" {
			key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err == nil {
				signingKey = key
				return key, nil
			}
		}
	}

	// Generate new key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}

	// Save to disk
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return nil, fmt.Errorf("create key dir: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}

	signingKey = key
	return key, nil
}

// RegenerateKey generates a new RSA key and saves it.
func RegenerateKey(keyPath string) (*rsa.PrivateKey, error) {
	signingKeyMu.Lock()
	defer signingKeyMu.Unlock()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return nil, fmt.Errorf("create key dir: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}

	signingKey = key
	return key, nil
}

// GetSigningKey returns the current signing key.
func GetSigningKey() *rsa.PrivateKey {
	signingKeyMu.RLock()
	defer signingKeyMu.RUnlock()
	return signingKey
}

// --- JWT Implementation (no external library) ---

// JWTClaims represents JWT claims.
type JWTClaims struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"`
	Audience  string `json:"aud"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	JWTID     string `json:"jti"`
	Scope     string `json:"scope,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
}

// SignJWT creates a signed JWT token using RS256.
func SignJWT(claims *JWTClaims, key *rsa.PrivateKey) (string, error) {
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": "default",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerB64 := base64URLEncode(headerJSON)
	claimsB64 := base64URLEncode(claimsJSON)
	signingInput := headerB64 + "." + claimsB64

	hash := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64URLEncode(signature), nil
}

// VerifyJWT verifies a JWT token and returns its claims.
func VerifyJWT(tokenStr string, key *rsa.PublicKey) (*JWTClaims, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	hash := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature); err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}

	claimsJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	if claims.ExpiresAt > 0 && time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

// JWKS returns the JSON Web Key Set for the public key.
func JWKS(key *rsa.PublicKey) map[string]interface{} {
	return map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": "default",
				"n":   base64URLEncode(key.N.Bytes()),
				"e":   base64URLEncode(big.NewInt(int64(key.E)).Bytes()),
			},
		},
	}
}

// CreateAccessToken creates a new JWT access token for the given client.
func CreateAccessToken(issuer, clientID, scope string, key *rsa.PrivateKey) (string, int64, error) {
	now := time.Now()
	expiresIn := int64(3600) // 1 hour
	jti := generateID()

	claims := &JWTClaims{
		Issuer:    issuer,
		Subject:   clientID,
		Audience:  issuer,
		ExpiresAt: now.Unix() + expiresIn,
		IssuedAt:  now.Unix(),
		JWTID:     jti,
		Scope:     scope,
		ClientID:  clientID,
	}

	token, err := SignJWT(claims, key)
	if err != nil {
		return "", 0, err
	}

	return token, expiresIn, nil
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
