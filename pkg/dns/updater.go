package dns

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Status holds the current state of the DNS updater.
type Status struct {
	Enabled       bool   `json:"enabled"`
	Domain        string `json:"domain"`
	PublicIP      string `json:"publicIp"`
	HostedZoneID  string `json:"hostedZoneId"`
	LastUpdate    string `json:"lastUpdate"`
	LastError     string `json:"lastError"`
	UpdateCount   int    `json:"updateCount"`
	NextUpdate    string `json:"nextUpdate"`
	IntervalSecs  int    `json:"intervalSecs"`
}

// Updater periodically updates a Route 53 A record with the machine's public IP.
type Updater struct {
	mu         sync.RWMutex
	client     *Route53Client
	domain     string
	zoneID     string
	interval   time.Duration
	publicIP   string
	lastUpdate time.Time
	lastError  string
	updateCount int
	cancel     context.CancelFunc
	running    bool
	onLog      func(msg string)
}

// NewUpdater creates a new DNS updater.
func NewUpdater(onLog func(msg string)) *Updater {
	return &Updater{
		onLog: onLog,
	}
}

// Start begins periodic DNS updates. Stops any existing updater first.
func (u *Updater) Start(ctx context.Context, accessKey, secretKey, domain string, intervalSecs int) error {
	u.Stop()

	if accessKey == "" || secretKey == "" {
		return fmt.Errorf("AWS credentials required")
	}
	if domain == "" {
		return fmt.Errorf("domain required")
	}
	if intervalSecs < 60 {
		intervalSecs = 300
	}

	client := NewRoute53Client(accessKey, secretKey)

	// Look up hosted zone ID
	u.log("Looking up hosted zone for %s...", domain)
	zoneID, err := client.FindHostedZoneID(domain)
	if err != nil {
		return fmt.Errorf("find hosted zone: %w", err)
	}
	u.log("Found hosted zone: %s", zoneID)

	u.mu.Lock()
	u.client = client
	u.domain = domain
	u.zoneID = zoneID
	u.interval = time.Duration(intervalSecs) * time.Second
	u.lastError = ""
	u.running = true
	u.mu.Unlock()

	// Run first update immediately
	u.runUpdate()

	// Start periodic updates
	childCtx, cancel := context.WithCancel(ctx)
	u.mu.Lock()
	u.cancel = cancel
	u.mu.Unlock()

	go u.loop(childCtx)

	return nil
}

// Stop stops the periodic updater.
func (u *Updater) Stop() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.cancel != nil {
		u.cancel()
		u.cancel = nil
	}
	u.running = false
}

// RunOnce performs a single DNS update immediately.
func (u *Updater) RunOnce(accessKey, secretKey, domain string) (string, error) {
	if accessKey == "" || secretKey == "" {
		return "", fmt.Errorf("AWS credentials required")
	}
	if domain == "" {
		return "", fmt.Errorf("domain required")
	}

	client := NewRoute53Client(accessKey, secretKey)

	ip, err := GetPublicIP()
	if err != nil {
		return "", fmt.Errorf("get public IP: %w", err)
	}

	zoneID, err := client.FindHostedZoneID(domain)
	if err != nil {
		return "", fmt.Errorf("find hosted zone: %w", err)
	}

	status, err := client.UpsertARecord(zoneID, domain, ip, 300)
	if err != nil {
		return "", fmt.Errorf("upsert record: %w", err)
	}

	return fmt.Sprintf("Updated %s -> %s (zone: %s, status: %s)", domain, ip, zoneID, status), nil
}

// GetStatus returns the current updater status.
func (u *Updater) GetStatus() *Status {
	u.mu.RLock()
	defer u.mu.RUnlock()

	s := &Status{
		Enabled:      u.running,
		Domain:       u.domain,
		PublicIP:     u.publicIP,
		HostedZoneID: u.zoneID,
		LastError:    u.lastError,
		UpdateCount:  u.updateCount,
		IntervalSecs: int(u.interval.Seconds()),
	}
	if !u.lastUpdate.IsZero() {
		s.LastUpdate = u.lastUpdate.Format(time.RFC3339)
		s.NextUpdate = u.lastUpdate.Add(u.interval).Format(time.RFC3339)
	}
	return s
}

func (u *Updater) loop(ctx context.Context) {
	u.mu.RLock()
	interval := u.interval
	u.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.runUpdate()
		}
	}
}

func (u *Updater) runUpdate() {
	ip, err := GetPublicIP()
	if err != nil {
		u.mu.Lock()
		u.lastError = fmt.Sprintf("get public IP: %v", err)
		u.mu.Unlock()
		u.log("DNS update failed: %s", u.lastError)
		return
	}

	u.mu.RLock()
	oldIP := u.publicIP
	client := u.client
	domain := u.domain
	zoneID := u.zoneID
	u.mu.RUnlock()

	// Only update if IP changed or first run
	if ip == oldIP && oldIP != "" {
		u.mu.Lock()
		u.lastUpdate = time.Now()
		u.lastError = ""
		u.mu.Unlock()
		u.log("DNS check: IP unchanged (%s)", ip)
		return
	}

	u.log("DNS update: %s -> %s (was: %s)", domain, ip, oldIP)

	status, err := client.UpsertARecord(zoneID, domain, ip, 300)
	if err != nil {
		u.mu.Lock()
		u.lastError = fmt.Sprintf("upsert: %v", err)
		u.mu.Unlock()
		u.log("DNS update failed: %s", u.lastError)
		return
	}

	u.mu.Lock()
	u.publicIP = ip
	u.lastUpdate = time.Now()
	u.lastError = ""
	u.updateCount++
	u.mu.Unlock()

	u.log("DNS updated: %s -> %s (status: %s)", domain, ip, status)
}

func (u *Updater) log(format string, args ...interface{}) {
	if u.onLog != nil {
		u.onLog(fmt.Sprintf(format, args...))
	}
}

// GetPublicIP fetches the machine's public IP address from external services.
func GetPublicIP() (string, error) {
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
		"https://checkip.amazonaws.com",
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for _, url := range services {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if ip != "" && len(ip) <= 45 { // max IPv6 length
			return ip, nil
		}
	}

	return "", fmt.Errorf("all IP lookup services failed")
}
