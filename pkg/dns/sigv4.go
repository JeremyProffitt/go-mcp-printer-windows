package dns

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// signRequest signs an HTTP request using AWS Signature V4.
func signRequest(req *http.Request, accessKey, secretKey, region, service string, body []byte) {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	req.Header.Set("X-Amz-Date", amzDate)
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.Host)
	}

	// Step 1: Create canonical request
	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	canonicalQueryString := req.URL.Query().Encode()

	// Canonical headers - must be sorted
	signedHeadersList := []string{"host", "x-amz-date"}
	if req.Header.Get("Content-Type") != "" {
		signedHeadersList = append(signedHeadersList, "content-type")
	}
	sort.Strings(signedHeadersList)
	signedHeaders := strings.Join(signedHeadersList, ";")

	var canonicalHeaders string
	for _, h := range signedHeadersList {
		var val string
		switch h {
		case "host":
			val = req.Host
		default:
			val = strings.TrimSpace(req.Header.Get(http.CanonicalHeaderKey(h)))
		}
		canonicalHeaders += h + ":" + val + "\n"
	}

	payloadHash := sha256Hex(body)

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	// Step 2: Create string to sign
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// Step 3: Calculate signing key
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))

	// Step 4: Create signature
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	// Step 5: Add authorization header
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// readAndClose reads the full body and returns it.
func readAndClose(r io.ReadCloser) ([]byte, error) {
	defer r.Close()
	return io.ReadAll(r)
}
