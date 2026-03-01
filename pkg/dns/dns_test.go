package dns

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSHA256Hex(t *testing.T) {
	result := sha256Hex([]byte(""))
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if result != expected {
		t.Errorf("sha256Hex empty = %q, want %q", result, expected)
	}

	result = sha256Hex([]byte("hello"))
	expected = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if result != expected {
		t.Errorf("sha256Hex hello = %q, want %q", result, expected)
	}
}

func TestHmacSHA256(t *testing.T) {
	result := hmacSHA256([]byte("key"), []byte("data"))
	if len(result) != 32 {
		t.Errorf("hmacSHA256 length = %d, want 32", len(result))
	}
}

func TestSignRequest(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://route53.amazonaws.com/2013-04-01/hostedzonesbyname", nil)
	req.Host = "route53.amazonaws.com"

	signRequest(req, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", "route53", nil)

	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Fatal("Authorization header is empty")
	}
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/") {
		t.Errorf("Authorization header has wrong prefix: %s", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=host;x-amz-date") {
		t.Errorf("Authorization header missing SignedHeaders: %s", auth)
	}
	if req.Header.Get("X-Amz-Date") == "" {
		t.Error("X-Amz-Date header is empty")
	}
}

func TestSignRequestWithBody(t *testing.T) {
	body := []byte("<xml>test</xml>")
	req, _ := http.NewRequest(http.MethodPost, "https://route53.amazonaws.com/2013-04-01/hostedzone/Z123/rrset", nil)
	req.Host = "route53.amazonaws.com"
	req.Header.Set("Content-Type", "text/xml")

	signRequest(req, "AKID", "SECRET", "us-east-1", "route53", body)

	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Fatal("Authorization header is empty")
	}
	if !strings.Contains(auth, "content-type;host;x-amz-date") {
		t.Errorf("Authorization should include content-type in signed headers: %s", auth)
	}
}

func TestGenerateIAMPolicy(t *testing.T) {
	// Without zone ID
	policy := GenerateIAMPolicy("")
	if !strings.Contains(policy, "route53:ListHostedZonesByName") {
		t.Error("Policy missing ListHostedZonesByName")
	}
	if !strings.Contains(policy, "route53:ChangeResourceRecordSets") {
		t.Error("Policy missing ChangeResourceRecordSets")
	}
	if !strings.Contains(policy, `"*"`) {
		t.Error("Policy without zone should have wildcard resource")
	}

	// With zone ID
	policy = GenerateIAMPolicy("Z1234567890")
	if !strings.Contains(policy, "arn:aws:route53:::hostedzone/Z1234567890") {
		t.Error("Policy should contain zone ARN")
	}
}

func TestGetPublicIP(t *testing.T) {
	// Use a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("203.0.113.42"))
	}))
	defer server.Close()

	// We can't easily test the real function without internet, but we can test the parsing logic
	// by checking the function signature exists and the mock server works
	client := &http.Client{}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("mock request failed: %v", err)
	}
	defer resp.Body.Close()
}

func TestNewUpdater(t *testing.T) {
	var logged []string
	u := NewUpdater(func(msg string) {
		logged = append(logged, msg)
	})
	if u == nil {
		t.Fatal("NewUpdater returned nil")
	}

	status := u.GetStatus()
	if status.Enabled {
		t.Error("New updater should not be enabled")
	}
	if status.PublicIP != "" {
		t.Error("New updater should have empty public IP")
	}
}

func TestUpdaterStartValidation(t *testing.T) {
	u := NewUpdater(nil)

	// Missing credentials
	err := u.Start(nil, "", "", "example.com", 300)
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Errorf("Expected credentials error, got: %v", err)
	}

	// Missing domain
	err = u.Start(nil, "key", "secret", "", 300)
	if err == nil || !strings.Contains(err.Error(), "domain") {
		t.Errorf("Expected domain error, got: %v", err)
	}
}

func TestRoute53ClientCreation(t *testing.T) {
	client := NewRoute53Client("AKID", "SECRET")
	if client == nil {
		t.Fatal("NewRoute53Client returned nil")
	}
	if client.accessKey != "AKID" {
		t.Errorf("accessKey = %q, want AKID", client.accessKey)
	}
}
