package tray

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"fyne.io/systray"
)

// Run starts the system tray icon. It blocks on the main goroutine (required by
// systray for Windows GUI thread affinity). Use ctx/cancel for coordinated
// shutdown with background HTTP servers.
func Run(ctx context.Context, cancel context.CancelFunc, port, adminPort int) {
	systray.Run(func() { onReady(ctx, cancel, port, adminPort) }, func() { onExit(cancel) })
}

func onReady(ctx context.Context, cancel context.CancelFunc, port, adminPort int) {
	systray.SetIcon(iconData)
	systray.SetTitle("MCP Printer")
	systray.SetTooltip("Go MCP Printer Server")

	mConfig := systray.AddMenuItem("Open Config", "Open admin UI in browser")
	mLogs := systray.AddMenuItem("View Logs", "Open logs in browser")
	systray.AddSeparator()
	mStatus := systray.AddMenuItem("Status: Checking...", "Server status")
	mStatus.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Exit", "Shut down server and exit")

	// Health check URL (MCP HTTP server)
	healthURL := fmt.Sprintf("http://localhost:%d", port)
	if port == 80 {
		healthURL = "http://localhost"
	}

	// Admin UI URL (separate HTTP server)
	adminURL := fmt.Sprintf("http://localhost:%d", adminPort)

	// Watch for external shutdown (Ctrl+C or server error) → unblock main thread
	go func() {
		<-ctx.Done()
		systray.Quit()
	}()

	// Poll server status
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// Immediate first check
		checkHealth(healthURL, mStatus)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkHealth(healthURL, mStatus)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-mConfig.ClickedCh:
				openBrowser(adminURL + "/admin/")
			case <-mLogs.ClickedCh:
				openBrowser(adminURL + "/admin/#logs")
			case <-mQuit.ClickedCh:
				cancel()
			}
		}
	}()
}

func onExit(cancel context.CancelFunc) {
	cancel()
}

func checkHealth(healthURL string, mStatus *systray.MenuItem) {
	resp, err := http.DefaultClient.Get(healthURL + "/health")
	if err == nil {
		resp.Body.Close()
		mStatus.SetTitle("Status: Running")
	} else {
		mStatus.SetTitle("Status: Not responding")
	}
}

// Notify shows a Windows balloon/toast notification.
func Notify(title, message string) {
	// Use PowerShell to show a Windows toast notification
	ps := fmt.Sprintf(
		`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null; `+
			`[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom, ContentType = WindowsRuntime] | Out-Null; `+
			`$xml = [Windows.Data.Xml.Dom.XmlDocument]::new(); `+
			`$xml.LoadXml('<toast><visual><binding template="ToastText02"><text id="1">%s</text><text id="2">%s</text></binding></visual></toast>'); `+
			`$toast = [Windows.UI.Notifications.ToastNotification]::new($xml); `+
			`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Go MCP Printer').Show($toast)`,
		title, message,
	)
	exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).Start()
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
