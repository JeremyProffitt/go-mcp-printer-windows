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
  serve      Start server (tray icon when run interactively)
  install    Install as Windows service (requires admin)
  uninstall  Remove Windows service (requires admin)
  version    Print version
  help       Show this help
`, version)
}

func runServe() {
	logger, err := logging.NewLogger(logging.Config{
		LogDir:  config.DefaultConfig().LogDir,
		AppName: config.AppName,
		Level:   logging.LevelInfo,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Restart channel — any goroutine can request a restart by sending on this
	restartCh := make(chan struct{}, 1)

	if service.IsWindowsService() {
		logger.Info("Running as Windows service")
		if err := service.Run(func(ctx context.Context) error {
			return runServerLoop(ctx, logger, restartCh)
		}, logger); err != nil {
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
			select {
			case sig := <-sigChan:
				logger.LogShutdown(fmt.Sprintf("signal: %v", sig))
				fmt.Fprintf(os.Stderr, "\nShutting down...\n")
				cancel()
			case <-ctx.Done():
			}
		}()

		// Run server loop in background goroutine
		go func() {
			if err := runServerLoop(ctx, logger, restartCh); err != nil && err != http.ErrServerClosed {
				logger.Error("Server error: %v", err)
				logger.LogShutdown(fmt.Sprintf("error: %v", err))
				fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
				cancel()
			}
		}()

		// tray.Run blocks on main thread (Windows GUI thread affinity)
		cfg, _ := config.Load()
		tray.Run(ctx, cancel, cfg.Port, cfg.AdminPort)
		logger.LogShutdown("normal exit")
	}
}

// runServerLoop loads config, starts servers, and restarts when signalled.
func runServerLoop(ctx context.Context, logger *logging.Logger, restartCh chan struct{}) error {
	for {
		if err := runServerOnce(ctx, logger, restartCh); err != nil {
			return err
		}

		// Wait for restart signal or parent context cancellation
		select {
		case <-ctx.Done():
			return nil
		case <-restartCh:
			logger.Info("Restarting server...")
			fmt.Fprintf(os.Stderr, "Restarting server...\n")
			// Clear config cache so we reload from disk
			config.ClearCache()
			continue
		}
	}
}

// runServerOnce runs one lifecycle of the MCP + admin servers until ctx is
// cancelled or the iteration context is cancelled (for restart).
func runServerOnce(parentCtx context.Context, logger *logging.Logger, restartCh chan struct{}) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger.SetLevel(logging.ParseLogLevel(cfg.LogLevel))
	startupInfo := logging.GetStartupInfo(version, cfg.LogDir, cfg.LogLevel, cfg.Domain, cfg.Port)
	logger.LogStartup(startupInfo)

	// Create MCP server
	server := mcp.NewServer(
		"Go MCP Printer",
		version,
		cfg.RateLimitCalls,
		time.Duration(cfg.RateLimitWindow)*time.Second,
	)

	// Log every MCP JSON-RPC request
	server.SetRequestCallback(func(method string) {
		logger.Access("MCP_REQUEST method=%q", method)
	})

	// Log tool calls + show Windows notification for print operations
	printTools := map[string]bool{
		"print_file": true, "print_text": true, "print_image": true,
		"print_html": true, "print_test_page": true, "print_all_test_pages": true,
		"print_url": true, "print_markdown": true, "print_multiple_files": true,
	}
	server.SetToolCallCallback(func(name string, args map[string]interface{}, duration time.Duration, success bool) {
		logger.ToolCall(name, args, duration, success)
		if printTools[name] && success {
			printerArg, _ := args["printer"].(string)
			msg := fmt.Sprintf("Tool: %s", name)
			if printerArg != "" {
				msg = fmt.Sprintf("Tool: %s → %s", name, printerArg)
			}
			tray.Notify("Print Request", msg)
		}
	})

	// Register tools, resources, and prompts
	registry := tools.NewRegistry(cfg, logger, version)
	registry.RegisterAll(server)
	registry.RegisterResources(server)
	registry.RegisterPrompts(server)

	// Create admin mux on separate port
	adminMux := http.NewServeMux()
	adminHandler := admin.NewHandler(cfg, logger, version, restartCh)
	adminHandler.RegisterRoutes(adminMux)

	fmt.Fprintf(os.Stderr, "Go MCP Printer Server v%s\n", version)
	fmt.Fprintf(os.Stderr, "HTTP: %s:%d\n", cfg.Domain, cfg.Port)
	fmt.Fprintf(os.Stderr, "Tools: %d | Resources: %d | Prompts: %d\n", server.ToolCount(), server.ResourceCount(), server.PromptCount())
	fmt.Fprintf(os.Stderr, "Admin: http://localhost:%d/admin/\n", cfg.AdminPort)

	// Per-iteration context — cancel this to shut down servers for restart
	iterCtx, iterCancel := context.WithCancel(parentCtx)
	defer iterCancel()

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
		<-iterCtx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		adminServer.Shutdown(shutCtx)
	}()

	// When restartCh fires, cancel this iteration so servers shut down
	go func() {
		select {
		case <-iterCtx.Done():
		case <-restartCh:
			// Put the signal back so the loop in runServerLoop sees it
			select {
			case restartCh <- struct{}{}:
			default:
			}
			iterCancel()
		}
	}()

	err = server.RunHTTP(iterCtx, fmt.Sprintf(":%d", cfg.Port))
	if err == http.ErrServerClosed {
		return nil
	}
	return err
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

