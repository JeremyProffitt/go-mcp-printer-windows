package tools

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/config"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/logging"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/mcp"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/printer"
)

// Registry manages tool registration.
type Registry struct {
	cfg       *config.Config
	logger    *logging.Logger
	version   string
	startTime time.Time
}

// NewRegistry creates a new tool registry.
func NewRegistry(cfg *config.Config, logger *logging.Logger, version string) *Registry {
	return &Registry{
		cfg:       cfg,
		logger:    logger,
		version:   version,
		startTime: time.Now(),
	}
}

// RegisterAll registers all 14 printer tools on the MCP server.
func (r *Registry) RegisterAll(s *mcp.Server) {
	r.registerListPrinters(s)
	r.registerGetPrinterDetails(s)
	r.registerGetDefaultPrinter(s)
	r.registerPrintFile(s)
	r.registerPrintText(s)
	r.registerPrintImage(s)
	r.registerGetPrintQueue(s)
	r.registerGetPrintJobStatus(s)
	r.registerCancelPrintJob(s)
	r.registerPausePrinter(s)
	r.registerResumePrinter(s)
	r.registerSetDefaultPrinter(s)
	r.registerPrintTestPage(s)
	r.registerGetPrinterServerStatus(s)
}

func (r *Registry) filterPrinters(printers []printer.PrinterInfo) []printer.PrinterInfo {
	if len(r.cfg.AllowedPrinters) == 0 && len(r.cfg.BlockedPrinters) == 0 {
		return printers
	}

	allowed := make(map[string]bool)
	for _, name := range r.cfg.AllowedPrinters {
		allowed[strings.ToLower(name)] = true
	}
	blocked := make(map[string]bool)
	for _, name := range r.cfg.BlockedPrinters {
		blocked[strings.ToLower(name)] = true
	}

	var result []printer.PrinterInfo
	for _, p := range printers {
		lower := strings.ToLower(p.Name)
		if blocked[lower] {
			continue
		}
		if len(allowed) > 0 && !allowed[lower] {
			continue
		}
		result = append(result, p)
	}
	return result
}

func (r *Registry) isPrinterAllowed(name string) bool {
	lower := strings.ToLower(name)
	for _, b := range r.cfg.BlockedPrinters {
		if strings.ToLower(b) == lower {
			return false
		}
	}
	if len(r.cfg.AllowedPrinters) > 0 {
		for _, a := range r.cfg.AllowedPrinters {
			if strings.ToLower(a) == lower {
				return true
			}
		}
		return false
	}
	return true
}

func (r *Registry) isPathAllowed(path string) bool {
	if len(r.cfg.AllowedPaths) == 0 {
		return true
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absPath = strings.ToLower(absPath)
	for _, allowed := range r.cfg.AllowedPaths {
		allowedAbs, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if strings.HasPrefix(absPath, strings.ToLower(allowedAbs)) {
			return true
		}
	}
	return false
}

// --- Tool 1: list_printers ---
func (r *Registry) registerListPrinters(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "list_printers",
		Description: "List all installed printers with status and type",
		InputSchema: mcp.JSONSchema{Type: "object"},
		Annotations: &mcp.ToolAnnotation{
			Title:        "List Printers",
			ReadOnlyHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		printers, err := printer.ListPrinters()
		if err != nil {
			return mcp.ErrorResult("Failed to list printers: %s", err), nil
		}
		printers = r.filterPrinters(printers)
		return mcp.JSONResult(printers)
	})
}

// --- Tool 2: get_printer_details ---
func (r *Registry) registerGetPrinterDetails(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "get_printer_details",
		Description: "Get detailed info and capabilities for a printer",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
			},
			Required: []string{"printer_name"},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:        "Printer Details",
			ReadOnlyHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := mcp.RequireStringArg(args, "printer_name")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		if !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		info, err := printer.GetPrinterDetails(name)
		if err != nil {
			return mcp.ErrorResult("Failed to get printer details: %s", err), nil
		}
		return mcp.JSONResult(info)
	})
}

// --- Tool 3: get_default_printer ---
func (r *Registry) registerGetDefaultPrinter(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "get_default_printer",
		Description: "Get the name of the default printer",
		InputSchema: mcp.JSONSchema{Type: "object"},
		Annotations: &mcp.ToolAnnotation{
			Title:        "Default Printer",
			ReadOnlyHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := printer.GetDefaultPrinter()
		if err != nil {
			return mcp.ErrorResult("Failed to get default printer: %s", err), nil
		}
		if name == "" {
			return mcp.TextResult("No default printer is set"), nil
		}
		return mcp.TextResult(name), nil
	})
}

// --- Tool 4: print_file ---
func (r *Registry) registerPrintFile(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "print_file",
		Description: "Print a file (PDF, images, text, Office docs)",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"file_path":    {Type: "string", Description: "Path to the file to print"},
				"printer_name": {Type: "string", Description: "Target printer (default: system default)"},
				"copies":       {Type: "integer", Description: "Number of copies", Default: 1, Minimum: mcp.Float64Ptr(1), Maximum: mcp.Float64Ptr(99)},
				"duplex":       {Type: "string", Description: "Duplex mode", Enum: []string{"None", "TwoSidedLongEdge", "TwoSidedShortEdge"}},
				"orientation":  {Type: "string", Description: "Page orientation", Enum: []string{"Portrait", "Landscape"}},
			},
			Required: []string{"file_path"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		filePath, err := mcp.RequireStringArg(args, "file_path")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		if !r.isPathAllowed(filePath) {
			return mcp.ErrorResult("Path %q is not in the allowed paths list", filePath), nil
		}

		opts := printer.PrintOptions{
			PrinterName: mcp.GetStringArg(args, "printer_name", r.cfg.DefaultPrinter),
			Copies:      mcp.GetIntArg(args, "copies", 1),
			Duplex:      mcp.GetStringArg(args, "duplex", ""),
			Orientation: mcp.GetStringArg(args, "orientation", ""),
		}
		if opts.PrinterName != "" && !r.isPrinterAllowed(opts.PrinterName) {
			return mcp.ErrorResult("Printer %q is not accessible", opts.PrinterName), nil
		}

		if err := printer.PrintFile(filePath, opts); err != nil {
			r.logger.PrintJob(opts.PrinterName, filePath, opts.Copies, err)
			return mcp.ErrorResult("Print failed: %s", err), nil
		}
		r.logger.PrintJob(opts.PrinterName, filePath, opts.Copies, nil)
		return mcp.TextResult(fmt.Sprintf("Sent %s to printer %s (%d copies)", filepath.Base(filePath), opts.PrinterName, opts.Copies)), nil
	})
}

// --- Tool 5: print_text ---
func (r *Registry) registerPrintText(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "print_text",
		Description: "Print raw text content",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"text":         {Type: "string", Description: "Text content to print"},
				"printer_name": {Type: "string", Description: "Target printer (default: system default)"},
			},
			Required: []string{"text"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		text, err := mcp.RequireStringArg(args, "text")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}

		opts := printer.PrintOptions{
			PrinterName: mcp.GetStringArg(args, "printer_name", r.cfg.DefaultPrinter),
		}
		if opts.PrinterName != "" && !r.isPrinterAllowed(opts.PrinterName) {
			return mcp.ErrorResult("Printer %q is not accessible", opts.PrinterName), nil
		}

		if err := printer.PrintText(text, opts); err != nil {
			return mcp.ErrorResult("Print failed: %s", err), nil
		}
		return mcp.TextResult(fmt.Sprintf("Sent text (%d chars) to printer %s", len(text), opts.PrinterName)), nil
	})
}

// --- Tool 6: print_image ---
func (r *Registry) registerPrintImage(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "print_image",
		Description: "Print an image with photo-optimized settings",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"image_path":   {Type: "string", Description: "Path to image file (JPEG, PNG, BMP, TIFF)"},
				"printer_name": {Type: "string", Description: "Target printer (default: system default)"},
				"copies":       {Type: "integer", Description: "Number of copies", Default: 1, Minimum: mcp.Float64Ptr(1), Maximum: mcp.Float64Ptr(99)},
			},
			Required: []string{"image_path"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		imagePath, err := mcp.RequireStringArg(args, "image_path")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		if !r.isPathAllowed(imagePath) {
			return mcp.ErrorResult("Path %q is not in the allowed paths list", imagePath), nil
		}

		opts := printer.PrintOptions{
			PrinterName: mcp.GetStringArg(args, "printer_name", r.cfg.DefaultPrinter),
			Copies:      mcp.GetIntArg(args, "copies", 1),
		}
		if opts.PrinterName != "" && !r.isPrinterAllowed(opts.PrinterName) {
			return mcp.ErrorResult("Printer %q is not accessible", opts.PrinterName), nil
		}

		if err := printer.PrintImage(imagePath, opts); err != nil {
			r.logger.PrintJob(opts.PrinterName, imagePath, opts.Copies, err)
			return mcp.ErrorResult("Print failed: %s", err), nil
		}
		r.logger.PrintJob(opts.PrinterName, imagePath, opts.Copies, nil)
		return mcp.TextResult(fmt.Sprintf("Sent image %s to printer %s", filepath.Base(imagePath), opts.PrinterName)), nil
	})
}

// --- Tool 7: get_print_queue ---
func (r *Registry) registerGetPrintQueue(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "get_print_queue",
		Description: "Get print jobs in the queue for a printer or all printers",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name (omit for all printers)"},
			},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:        "Print Queue",
			ReadOnlyHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name := mcp.GetStringArg(args, "printer_name", "")
		if name != "" && !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		jobs, err := printer.GetPrintQueue(name)
		if err != nil {
			return mcp.ErrorResult("Failed to get print queue: %s", err), nil
		}
		if len(jobs) == 0 {
			return mcp.TextResult("No jobs in queue"), nil
		}
		return mcp.JSONResult(jobs)
	})
}

// --- Tool 8: get_print_job_status ---
func (r *Registry) registerGetPrintJobStatus(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "get_print_job_status",
		Description: "Get status of a specific print job",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
				"job_id":       {Type: "integer", Description: "Job ID"},
			},
			Required: []string{"printer_name", "job_id"},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:        "Job Status",
			ReadOnlyHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := mcp.RequireStringArg(args, "printer_name")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		jobID := mcp.GetIntArg(args, "job_id", 0)
		if jobID <= 0 {
			return mcp.ErrorResult("missing required argument: job_id"), nil
		}
		if !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		job, err := printer.GetPrintJobStatus(name, jobID)
		if err != nil {
			return mcp.ErrorResult("Failed to get job status: %s", err), nil
		}
		return mcp.JSONResult(job)
	})
}

// --- Tool 9: cancel_print_job ---
func (r *Registry) registerCancelPrintJob(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "cancel_print_job",
		Description: "Cancel a print job",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
				"job_id":       {Type: "integer", Description: "Job ID to cancel"},
			},
			Required: []string{"printer_name", "job_id"},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:           "Cancel Job",
			DestructiveHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := mcp.RequireStringArg(args, "printer_name")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		jobID := mcp.GetIntArg(args, "job_id", 0)
		if jobID <= 0 {
			return mcp.ErrorResult("missing required argument: job_id"), nil
		}
		if !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		if err := printer.CancelPrintJob(name, jobID); err != nil {
			return mcp.ErrorResult("Failed to cancel job: %s", err), nil
		}
		return mcp.TextResult(fmt.Sprintf("Cancelled job %d on %s", jobID, name)), nil
	})
}

// --- Tool 10: pause_printer ---
func (r *Registry) registerPausePrinter(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "pause_printer",
		Description: "Pause a printer (requires admin)",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
			},
			Required: []string{"printer_name"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := mcp.RequireStringArg(args, "printer_name")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		if !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		if err := printer.PausePrinter(name); err != nil {
			return mcp.ErrorResult("Failed to pause printer: %s", err), nil
		}
		return mcp.TextResult(fmt.Sprintf("Paused printer %s", name)), nil
	})
}

// --- Tool 11: resume_printer ---
func (r *Registry) registerResumePrinter(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "resume_printer",
		Description: "Resume a paused printer",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
			},
			Required: []string{"printer_name"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := mcp.RequireStringArg(args, "printer_name")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		if !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		if err := printer.ResumePrinter(name); err != nil {
			return mcp.ErrorResult("Failed to resume printer: %s", err), nil
		}
		return mcp.TextResult(fmt.Sprintf("Resumed printer %s", name)), nil
	})
}

// --- Tool 12: set_default_printer ---
func (r *Registry) registerSetDefaultPrinter(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "set_default_printer",
		Description: "Set the default printer",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name to set as default"},
			},
			Required: []string{"printer_name"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := mcp.RequireStringArg(args, "printer_name")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		if !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		if err := printer.SetDefaultPrinter(name); err != nil {
			return mcp.ErrorResult("Failed to set default printer: %s", err), nil
		}
		return mcp.TextResult(fmt.Sprintf("Default printer set to %s", name)), nil
	})
}

// --- Tool 13: print_test_page ---
func (r *Registry) registerPrintTestPage(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "print_test_page",
		Description: "Print a Windows test page",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
			},
			Required: []string{"printer_name"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := mcp.RequireStringArg(args, "printer_name")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		if !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		if err := printer.PrintTestPage(name); err != nil {
			return mcp.ErrorResult("Failed to print test page: %s", err), nil
		}
		return mcp.TextResult(fmt.Sprintf("Test page sent to %s", name)), nil
	})
}

// --- Tool 14: get_printer_server_status ---
func (r *Registry) registerGetPrinterServerStatus(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "get_printer_server_status",
		Description: "Get server status: version, uptime, config, OAuth clients",
		InputSchema: mcp.JSONSchema{Type: "object"},
		Annotations: &mcp.ToolAnnotation{
			Title:        "Server Status",
			ReadOnlyHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		uptime := time.Since(r.startTime)
		status := map[string]interface{}{
			"version":       r.version,
			"uptime":        uptime.String(),
			"domain":        r.cfg.Domain,
			"httpsPort":     r.cfg.HTTPSPort,
			"useSelfSigned": r.cfg.UseSelfSigned,
			"logLevel":      r.cfg.LogLevel,
			"rateLimitCalls": r.cfg.RateLimitCalls,
			"rateLimitWindow": fmt.Sprintf("%ds", r.cfg.RateLimitWindow),
		}
		return mcp.JSONResult(status)
	})
}
