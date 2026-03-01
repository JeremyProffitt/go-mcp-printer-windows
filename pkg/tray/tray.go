package tray

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"fyne.io/systray"
)

// Run starts the system tray icon.
func Run(httpsPort int) {
	systray.Run(func() { onReady(httpsPort) }, onExit)
}

func onReady(httpsPort int) {
	systray.SetTitle("MCP Printer")
	systray.SetTooltip("Go MCP Printer Server")

	mConfig := systray.AddMenuItem("Open Config", "Open admin UI in browser")
	mLogs := systray.AddMenuItem("View Logs", "Open logs in browser")
	systray.AddSeparator()
	mStatus := systray.AddMenuItem("Status: Checking...", "Server status")
	mStatus.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Exit Tray", "Close tray icon (service keeps running)")

	baseURL := fmt.Sprintf("https://localhost:%d", httpsPort)
	if httpsPort == 443 {
		baseURL = "https://localhost"
	}

	// Poll server status
	go func() {
		for {
			resp, err := http.Get(baseURL + "/health")
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
				openBrowser(baseURL + "/admin/")
			case <-mLogs.ClickedCh:
				openBrowser(baseURL + "/admin/#logs")
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
