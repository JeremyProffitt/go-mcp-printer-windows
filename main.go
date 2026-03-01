package main

import (
	"context"
	"fmt"
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

	// Register admin UI
	adminHandler := admin.NewHandler(cfg, logger, oauthServer, version)
	adminHandler.RegisterRoutes(server)

	fmt.Fprintf(os.Stderr, "Go MCP Printer Server v%s\n", version)
	fmt.Fprintf(os.Stderr, "HTTPS: %s:%d\n", cfg.Domain, cfg.HTTPSPort)
	fmt.Fprintf(os.Stderr, "Tools: %d registered\n", server.ToolCount())
	fmt.Fprintf(os.Stderr, "Admin: https://%s/admin/\n", cfg.Domain)

	// Run as service or foreground
	runFunc := func(ctx context.Context) error {
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
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	tray.Run(cfg.HTTPSPort)
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
