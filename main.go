package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/admin"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/config"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/logging"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/mcp"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/oauth"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/service"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/tools"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/tray"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "serve":
		runServe()
	case "tray":
		runTray()
	case "install":
		runInstall()
	case "uninstall":
		runUninstall()
	case "version":
		fmt.Printf("go-mcp-printer-windows %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`go-mcp-printer-windows %s

Usage:
  go-mcp-printer-windows <command>

Commands:
  serve      Start HTTPS server (as Windows service or foreground)
  tray       Start system tray icon
  install    Install as Windows service (requires admin)
  uninstall  Remove Windows service (requires admin)
  version    Print version
  help       Show this help
`, version)
}

func runServe() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.NewLogger(logging.Config{
		LogDir:  cfg.LogDir,
		AppName: config.AppName,
		Level:   logging.ParseLogLevel(cfg.LogLevel),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	startupInfo := logging.GetStartupInfo(version, cfg.LogDir, cfg.LogLevel, cfg.Domain, cfg.HTTPSPort)
	logger.LogStartup(startupInfo)

	// Initialize OAuth
	oauthStore, err := oauth.NewStore(config.OAuthStorePath())
	if err != nil {
		logger.Error("Failed to initialize OAuth store: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to initialize OAuth store: %v\n", err)
		os.Exit(1)
	}

	_, err = oauth.LoadOrGenerateKey(config.OAuthKeyPath())
	if err != nil {
		logger.Error("Failed to load/generate OAuth key: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to load/generate OAuth key: %v\n", err)
		os.Exit(1)
	}

	issuer := fmt.Sprintf("https://%s", cfg.Domain)
	if cfg.HTTPSPort != 443 {
		issuer = fmt.Sprintf("https://%s:%d", cfg.Domain, cfg.HTTPSPort)
	}

	oauthServer := oauth.NewServer(oauthStore, config.OAuthKeyPath(), issuer, logger)

	// Create MCP server
	server := mcp.NewServer(
		"Go MCP Printer",
		version,
		cfg.RateLimitCalls,
		time.Duration(cfg.RateLimitWindow)*time.Second,
	)

	// Set OAuth validator
	server.AuthValidator = oauthServer.ValidateRequest

	// Set telemetry callback
	server.SetToolCallCallback(func(name string, args map[string]interface{}, duration time.Duration, success bool) {
		logger.ToolCall(name, args, duration, success)
	})

	// Register OAuth routes
	oauthServer.RegisterRoutes(server)

	// Register tools
	registry := tools.NewRegistry(cfg, logger, version)
	registry.RegisterAll(server)

	// Create context for background tasks
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	// Create admin mux on separate port
	adminMux := http.NewServeMux()
	adminHandler := admin.NewHandler(cfg, logger, oauthServer, version, bgCtx)
	adminHandler.RegisterRoutes(adminMux)

	fmt.Fprintf(os.Stderr, "Go MCP Printer Server v%s\n", version)
	fmt.Fprintf(os.Stderr, "HTTPS: %s:%d\n", cfg.Domain, cfg.HTTPSPort)
	fmt.Fprintf(os.Stderr, "Tools: %d registered\n", server.ToolCount())
	fmt.Fprintf(os.Stderr, "Admin: http://localhost:%d/admin/\n", cfg.AdminPort)

	// Run as service or foreground
	runFunc := func(ctx context.Context) error {
		// Start admin HTTP server
		adminServer := &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.AdminPort),
			Handler: adminMux,
		}
		go func() {
			if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("Admin server error: %v", err)
			}
		}()
		go func() {
			<-ctx.Done()
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			adminServer.Shutdown(shutCtx)
		}()

		return server.RunHTTPS(
			ctx,
			cfg.Domain,
			cfg.HTTPSPort,
			cfg.HTTPPort,
			cfg.UseSelfSigned,
			cfg.ACMEEmail,
			config.CertDir(),
		)
	}

	if service.IsWindowsService() {
		logger.Info("Running as Windows service")
		if err := service.Run(runFunc, logger); err != nil {
			logger.Error("Service error: %v", err)
			logger.LogShutdown(fmt.Sprintf("error: %v", err))
			os.Exit(1)
		}
	} else {
		logger.Info("Running in foreground mode")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			sig := <-sigChan
			logger.LogShutdown(fmt.Sprintf("signal: %v", sig))
			fmt.Fprintf(os.Stderr, "\nShutting down...\n")
			cancel()
		}()

		if err := runFunc(ctx); err != nil {
			logger.Error("Server error: %v", err)
			logger.LogShutdown(fmt.Sprintf("error: %v", err))
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
		logger.LogShutdown("normal exit")
	}
}

func runTray() {
	// Hide the console window (Windows only)
	hideConsoleWindow()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	tray.Run(cfg.HTTPSPort, cfg.AdminPort)
}

func runInstall() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
		os.Exit(1)
	}

	if err := service.Install(exePath); err != nil {
		fmt.Fprintf(os.Stderr, "Install failed: %v\n", err)
		os.Exit(1)
	}
}

func runUninstall() {
	if err := service.Uninstall(); err != nil {
		fmt.Fprintf(os.Stderr, "Uninstall failed: %v\n", err)
		os.Exit(1)
	}
}

// hideConsoleWindow hides the console window on Windows.
// This is a no-op on other platforms.
func hideConsoleWindow() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")

	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")

	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, 0) // SW_HIDE = 0
	}
}
