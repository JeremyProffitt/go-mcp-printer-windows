package startup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/config"
)

const (
	registryKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	registryValueName = "GoMCPPrinter"
	taskName          = "GoMCPPrinterWatchdog"
)

// AutoStartEnabled checks if the auto-start registry entry exists.
func AutoStartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, registryKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	_, _, err = k.GetStringValue(registryValueName)
	return err == nil
}

// SetAutoStart enables or disables auto-start on login via the HKCU Run key.
func SetAutoStart(enabled bool) error {
	if enabled {
		k, err := registry.OpenKey(registry.CURRENT_USER, registryKeyPath, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("open registry key: %w", err)
		}
		defer k.Close()

		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path: %w", err)
		}
		value := fmt.Sprintf(`"%s" serve`, exePath)
		return k.SetStringValue(registryValueName, value)
	}

	k, err := registry.OpenKey(registry.CURRENT_USER, registryKeyPath, registry.SET_VALUE)
	if err != nil {
		return nil // key doesn't exist, nothing to remove
	}
	defer k.Close()

	err = k.DeleteValue(registryValueName)
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("delete registry value: %w", err)
	}
	return nil
}

// WatchdogEnabled checks if the watchdog scheduled task exists.
func WatchdogEnabled() bool {
	cmd := exec.Command("schtasks", "/Query", "/TN", taskName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// SetWatchdog enables or disables the watchdog scheduled task.
// When enabled, it creates a PowerShell script and a scheduled task that runs
// every 10 minutes to check the server health endpoint and restart if needed.
func SetWatchdog(enabled bool, port int) error {
	scriptPath := filepath.Join(config.DefaultDataDir(), "watchdog.ps1")

	if !enabled {
		exec.Command("schtasks", "/Delete", "/TN", taskName, "/F").Run()
		os.Remove(scriptPath)
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// Write watchdog script that reads config for current port
	script := fmt.Sprintf(`# GoMCPPrinter Watchdog — auto-generated, do not edit
$configPath = '%s'
$exePath = '%s'
$port = %d

if (Test-Path $configPath) {
    try {
        $cfg = Get-Content $configPath -Raw | ConvertFrom-Json
        if ($cfg.port -and $cfg.port -gt 0) { $port = $cfg.port }
    } catch {}
}

$url = "http://localhost:$port/health"
try {
    $resp = Invoke-WebRequest -Uri $url -TimeoutSec 5 -UseBasicParsing
    if ($resp.StatusCode -eq 200) { exit 0 }
} catch {}

# Server not responding — start it (use WScript.Shell to truly hide console window)
$wsh = New-Object -ComObject WScript.Shell
$wsh.Run("""$exePath"" serve", 0, $false)
`, config.ConfigPath(), exePath, port)

	if err := os.MkdirAll(filepath.Dir(scriptPath), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("write watchdog script: %w", err)
	}

	// Create scheduled task that runs every 10 minutes
	taskCmd := fmt.Sprintf(
		`powershell.exe -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File "%s"`,
		scriptPath,
	)
	out, err := exec.Command("schtasks", "/Create",
		"/TN", taskName,
		"/TR", taskCmd,
		"/SC", "MINUTE",
		"/MO", "10",
		"/F",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create scheduled task: %s: %w", string(out), err)
	}

	return nil
}
