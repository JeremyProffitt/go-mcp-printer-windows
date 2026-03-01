package tray

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"fyne.io/systray"
)

// Run starts the system tray icon.
func Run(httpsPort, adminPort int) {
	systray.Run(func() { onReady(httpsPort, adminPort) }, onExit)
}

func onReady(httpsPort, adminPort int) {
	systray.SetIcon(iconData)
	systray.SetTitle("MCP Printer")
	systray.SetTooltip("Go MCP Printer Server")

	mConfig := systray.AddMenuItem("Open Config", "Open admin UI in browser")
	mLogs := systray.AddMenuItem("View Logs", "Open logs in browser")
	systray.AddSeparator()
	mStatus := systray.AddMenuItem("Status: Checking...", "Server status")
	mStatus.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Exit Tray", "Close tray icon (service keeps running)")

	// Health check URL (MCP HTTPS server)
	healthURL := fmt.Sprintf("https://localhost:%d", httpsPort)
	if httpsPort == 443 {
		healthURL = "https://localhost"
	}

	// Admin UI URL (separate HTTP server)
	adminURL := fmt.Sprintf("http://localhost:%d", adminPort)

	// HTTPS client that skips certificate verification for localhost health checks
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// Poll server status
	go func() {
		for {
			resp, err := httpClient.Get(healthURL + "/health")
			if err == nil {
				resp.Body.Close()
				mStatus.SetTitle("Status: Running")
			} else {
				mStatus.SetTitle("Status: Not responding")
			}
			time.Sleep(30 * time.Second)
		}
	}()

	go func() {
		for {
			select {
			case <-mConfig.ClickedCh:
				openBrowser(adminURL + "/admin/")
			case <-mLogs.ClickedCh:
				openBrowser(adminURL + "/admin/#logs")
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	// Cleanup if needed
}

func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	exec.Command(cmd, args...).Start()
}
