package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/logging"
)

// RunFunc is the function signature for the server's main run loop.
type RunFunc func(ctx context.Context) error

// handler implements svc.Handler for the Windows service.
type handler struct {
	runFunc RunFunc
	logger  *logging.Logger
}

// Execute implements svc.Handler.
func (h *handler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.runFunc(ctx)
	}()

	changes <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	if h.logger != nil {
		h.logger.Info("Windows service started")
	}

	for {
		select {
		case err := <-errCh:
			if err != nil {
				if h.logger != nil {
					h.logger.Error("Service error: %v", err)
				}
				changes <- svc.Status{State: svc.StopPending}
				return false, 1
			}
			changes <- svc.Status{State: svc.StopPending}
			return false, 0

		case c := <-r:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				if h.logger != nil {
					h.logger.Info("Windows service stop requested")
				}
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				// Wait for runFunc to finish
				select {
				case <-errCh:
				case <-time.After(10 * time.Second):
				}
				return false, 0

			case svc.Interrogate:
				changes <- c.CurrentStatus
			}
		}
	}
}

// Run runs the server as a Windows service or in foreground mode.
func Run(runFunc RunFunc, logger *logging.Logger) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("detect service mode: %w", err)
	}

	if isService {
		return svc.Run("GoMCPPrinter", &handler{
			runFunc: runFunc,
			logger:  logger,
		})
	}

	// Foreground mode
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	go func() {
		sigCh := make(chan os.Signal, 1)
		// signal.Notify is not used here because the caller handles it
		<-sigCh
		cancel()
	}()

	return runFunc(ctx)
}

// IsWindowsService returns true if running as a Windows service.
func IsWindowsService() bool {
	is, _ := svc.IsWindowsService()
	return is
}
