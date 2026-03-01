package dns

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

const (
	route53Endpoint = "https://route53.amazonaws.com"
	route53Region   = "us-east-1"
	route53Service  = "route53"
)

// Route53Client is a minimal client for AWS Route 53.
type Route53Client struct {
	accessKey string
	secretKey string
	http      *http.Client
}

// NewRoute53Client creates a new Route 53 client.
func NewRoute53Client(accessKey, secretKey string) *Route53Client {
	return &Route53Client{
		accessKey: accessKey,
		secretKey: secretKey,
		http:      &http.Client{},
	}
}

// --- XML types for Route 53 API ---

type listHostedZonesResponse struct {
	XMLName    xml.Name     `xml:"ListHostedZonesByNameResponse"`
	HostedZones []hostedZone `xml:"HostedZones>HostedZone"`
}

type hostedZone struct {
	ID   string `xml:"Id"`
	Name string `xml:"Name"`
}

type changeResourceRecordSetsRequest struct {
	XMLName     xml.Name    `xml:"ChangeResourceRecordSetsRequest"`
	XMLNS       string      `xml:"xmlns,attr"`
	ChangeBatch changeBatch `xml:"ChangeBatch"`
}

type changeBatch struct {
	Changes []change `xml:"Changes>Change"`
}

type change struct {
	Action            string            `xml:"Action"`
	ResourceRecordSet resourceRecordSet `xml:"ResourceRecordSet"`
}

type resourceRecordSet struct {
	Name            string           `xml:"Name"`
	Type            string           `xml:"Type"`
	TTL             int              `xml:"TTL"`
	ResourceRecords []resourceRecord `xml:"ResourceRecords>ResourceRecord"`
}

type resourceRecord struct {
	Value string `xml:"Value"`
}

type changeResourceRecordSetsResponse struct {
	XMLName    xml.Name   `xml:"ChangeResourceRecordSetsResponse"`
	ChangeInfo changeInfo `xml:"ChangeInfo"`
}

type changeInfo struct {
	ID     string `xml:"Id"`
	Status string `xml:"Status"`
}

type errorResponse struct {
	XMLName xml.Name `xml:"ErrorResponse"`
	Error   struct {
		Type    string `xml:"Type"`
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	} `xml:"Error"`
}

// FindHostedZoneID finds the hosted zone ID for a domain.
func (c *Route53Client) FindHostedZoneID(domain string) (string, error) {
	// Ensure domain has trailing dot for Route 53 lookup
	lookupDomain := domain
	if !strings.HasSuffix(lookupDomain, ".") {
		lookupDomain += "."
	}

	// Try the full domain first, then walk up to parent zones
	parts := strings.Split(strings.TrimSuffix(domain, "."), ".")
	for i := 0; i < len(parts)-1; i++ {
		searchDomain := strings.Join(parts[i:], ".")
		if !strings.HasSuffix(searchDomain, ".") {
			searchDomain += "."
		}

		url := fmt.Sprintf("%s/2013-04-01/hostedzonesbyname?dnsname=%s&maxitems=1", route53Endpoint, searchDomain)

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}
		req.Host = "route53.amazonaws.com"
		signRequest(req, c.accessKey, c.secretKey, route53Region, route53Service, nil)

		resp, err := c.http.Do(req)
		if err != nil {
			return "", fmt.Errorf("request: %w", err)
		}

		body, err := readAndClose(resp.Body)
		if err != nil {
			return "", fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			var errResp errorResponse
			if xml.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
				return "", fmt.Errorf("AWS error: %s - %s", errResp.Error.Code, errResp.Error.Message)
			}
			return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		}

		var result listHostedZonesResponse
		if err := xml.Unmarshal(body, &result); err != nil {
			return "", fmt.Errorf("parse response: %w", err)
		}

		for _, zone := range result.HostedZones {
			if zone.Name == searchDomain {
				// Zone ID comes as "/hostedzone/ZXXXXX" — strip prefix
				id := zone.ID
				if strings.HasPrefix(id, "/hostedzone/") {
					id = strings.TrimPrefix(id, "/hostedzone/")
				}
				return id, nil
			}
		}
	}

	return "", fmt.Errorf("no hosted zone found for domain %q", domain)
}

// UpsertARecord creates or updates an A record for the given domain.
func (c *Route53Client) UpsertARecord(hostedZoneID, domain, ip string, ttl int) (string, error) {
	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}
	if ttl <= 0 {
		ttl = 300
	}

	reqBody := changeResourceRecordSetsRequest{
		XMLNS: "https://route53.amazonaws.com/doc/2013-04-01/",
		ChangeBatch: changeBatch{
			Changes: []change{
				{
					Action: "UPSERT",
					ResourceRecordSet: resourceRecordSet{
						Name: domain,
						Type: "A",
						TTL:  ttl,
						ResourceRecords: []resourceRecord{
							{Value: ip},
						},
					},
				},
			},
		},
	}

	xmlData, err := xml.MarshalIndent(reqBody, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal XML: %w", err)
	}
	xmlPayload := []byte(xml.Header + string(xmlData))

	url := fmt.Sprintf("%s/2013-04-01/hostedzone/%s/rrset", route53Endpoint, hostedZoneID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(xmlPayload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Host = "route53.amazonaws.com"
	req.Header.Set("Content-Type", "text/xml")
	signRequest(req, c.accessKey, c.secretKey, route53Region, route53Service, xmlPayload)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}

	body, err := readAndClose(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if xml.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return "", fmt.Errorf("AWS error: %s - %s", errResp.Error.Code, errResp.Error.Message)
		}
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result changeResourceRecordSetsResponse
	if err := xml.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return result.ChangeInfo.Status, nil
}

// GenerateIAMPolicy returns a minimal IAM policy JSON for the required Route 53 permissions.
// If hostedZoneID is provided, the policy is scoped to that specific zone.
func GenerateIAMPolicy(hostedZoneID string) string {
	resource := `"*"`
	if hostedZoneID != "" {
		resource = fmt.Sprintf(`[
                "arn:aws:route53:::hostedzone/%s",
                "arn:aws:route53:::hostedzone/%s"
            ]`, hostedZoneID, hostedZoneID)
	}

	return fmt.Sprintf(`{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "MCPPrinterDNSUpdate",
            "Effect": "Allow",
            "Action": [
                "route53:ListHostedZonesByName",
                "route53:GetHostedZone"
            ],
            "Resource": "*"
        },
        {
            "Sid": "MCPPrinterRecordUpdate",
            "Effect": "Allow",
            "Action": [
                "route53:ChangeResourceRecordSets"
            ],
            "Resource": %s
        }
    ]
}`, resource)
}
