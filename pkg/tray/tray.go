package tray

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"fyne.io/systray"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/startup"
)

// Run starts the system tray icon. It blocks on the main goroutine (required by
// systray for Windows GUI thread affinity). Use ctx/cancel for coordinated
// shutdown with background HTTP servers.
func Run(ctx context.Context, cancel context.CancelFunc, port, adminPort int) {
	notifyAdminPort = adminPort
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
	mAutoStart := systray.AddMenuItemCheckbox("Start on Login", "Start MCP Printer when you log in", startup.AutoStartEnabled())
	mWatchdog := systray.AddMenuItemCheckbox("Watchdog (10min)", "Check server health every 10 minutes, restart if down", startup.WatchdogEnabled())
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
			case <-mAutoStart.ClickedCh:
				enabled := !mAutoStart.Checked()
				if err := startup.SetAutoStart(enabled); err != nil {
					Notify("Error", fmt.Sprintf("Failed to set auto-start: %v", err))
				} else if enabled {
					mAutoStart.Check()
				} else {
					mAutoStart.Uncheck()
				}
			case <-mWatchdog.ClickedCh:
				enabled := !mWatchdog.Checked()
				if err := startup.SetWatchdog(enabled, port); err != nil {
					Notify("Error", fmt.Sprintf("Failed to set watchdog: %v", err))
				} else if enabled {
					mWatchdog.Check()
				} else {
					mWatchdog.Uncheck()
				}
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

// notifyAdminPort is set by Run so Notify can build the logs URL.
var notifyAdminPort int

// Notify shows a Windows toast notification. Clicking it opens the admin logs page.
func Notify(title, message string) {
	logsURL := fmt.Sprintf("http://localhost:%d/admin/#logs", notifyAdminPort)
	ps := fmt.Sprintf(
		`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null; `+
			`[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom, ContentType = WindowsRuntime] | Out-Null; `+
			`$xml = [Windows.Data.Xml.Dom.XmlDocument]::new(); `+
			`$xml.LoadXml('<toast activationType="protocol" launch="%s"><visual><binding template="ToastText02"><text id="1">%s</text><text id="2">%s</text></binding></visual></toast>'); `+
			`$toast = [Windows.UI.Notifications.ToastNotification]::new($xml); `+
			`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Go MCP Printer').Show($toast)`,
		logsURL, title, message,
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
