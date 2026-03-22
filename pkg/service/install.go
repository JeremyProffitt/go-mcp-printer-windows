package service

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/svc/mgr"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/config"
)

// Install creates the Windows service.
func Install(exePath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w (are you running as administrator?)", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(config.ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", config.ServiceName)
	}

	s, err = m.CreateService(config.ServiceName, exePath, mgr.Config{
		DisplayName:      "Go MCP Printer Server",
		Description:      "MCP server for Windows printer management over HTTP",
		StartType:        mgr.StartAutomatic,
		ServiceStartName: "NT AUTHORITY\\LocalService",
	}, "serve")
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()

	// Configure recovery: restart after 5 seconds on failure
	err = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5000},
		{Type: mgr.ServiceRestart, Delay: 5000},
		{Type: mgr.ServiceRestart, Delay: 5000},
	}, 86400) // Reset failure count after 1 day
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to set recovery actions: %v\n", err)
	}

	fmt.Printf("Service %s installed successfully\n", config.ServiceName)
	fmt.Println("Start with: sc start", config.ServiceName)
	return nil
}

// Uninstall removes the Windows service.
func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w (are you running as administrator?)", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(config.ServiceName)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", config.ServiceName, err)
	}
	defer s.Close()

	err = s.Delete()
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}

	fmt.Printf("Service %s uninstalled successfully\n", config.ServiceName)
	return nil
}
