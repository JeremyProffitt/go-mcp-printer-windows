package oauth

import (
	"encoding/json"
	"net/http"
)

// ProtectedResourceMetadata returns the RFC 9728 protected resource metadata.
func ProtectedResourceMetadata(issuer string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resource":               issuer,
			"authorization_servers":  []string{issuer},
			"bearer_methods_supported": []string{"header"},
			"scopes_supported":       []string{"mcp:tools"},
		})
	}
}

// AuthorizationServerMetadata returns the RFC 8414 authorization server metadata.
func AuthorizationServerMetadata(issuer string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/authorize",
			"token_endpoint":                        issuer + "/token",
			"registration_endpoint":                 issuer + "/register",
			"jwks_uri":                              issuer + "/jwks",
			"revocation_endpoint":                   issuer + "/revoke",
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code"},
			"token_endpoint_auth_methods_supported": []string{"none"},
			"code_challenge_methods_supported":      []string{"S256"},
			"scopes_supported":                      []string{"mcp:tools"},
		})
	}
}
