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

// RegisterAll registers all 30 printer tools on the MCP server.
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
	r.registerListPrinterPaperSizes(s)
	r.registerPrintAllTestPages(s)
	r.registerGetInkTonerLevels(s)
	r.registerGetPrintHistory(s)
	r.registerTestPrinterConnectivity(s)
	r.registerPurgePrintQueue(s)
	r.registerRestartPrintJob(s)
	r.registerAddNetworkPrinter(s)
	r.registerRemovePrinter(s)
	r.registerSetPrintDefaults(s)
	r.registerSharePrinter(s)
	r.registerPrintHTML(s)
	r.registerPrintURL(s)
	r.registerPrintMarkdown(s)
	r.registerPrintMultipleFiles(s)
	r.registerGetPrinterErrors(s)
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

// --- Tool 15: list_printer_paper_sizes ---
func (r *Registry) registerListPrinterPaperSizes(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "list_printer_paper_sizes",
		Description: "List all printers with their supported paper sizes (dimensions in mm and inches)",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Specific printer name (omit for all printers)"},
			},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:        "Printer Paper Sizes",
			ReadOnlyHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name := mcp.GetStringArg(args, "printer_name", "")

		if name != "" {
			if !r.isPrinterAllowed(name) {
				return mcp.ErrorResult("Printer %q is not accessible", name), nil
			}
			sizes, err := printer.GetPaperSizes(name)
			if err != nil {
				return mcp.ErrorResult("Failed to get paper sizes: %s", err), nil
			}
			result := printer.PrinterWithPaperSizes{
				Name:       name,
				PaperSizes: sizes,
			}
			return mcp.JSONResult(result)
		}

		allPrinters, err := printer.ListPrintersWithPaperSizes()
		if err != nil {
			return mcp.ErrorResult("Failed to list printers: %s", err), nil
		}

		var filtered []printer.PrinterWithPaperSizes
		for _, p := range allPrinters {
			if r.isPrinterAllowed(p.Name) {
				filtered = append(filtered, p)
			}
		}

		return mcp.JSONResult(filtered)
	})
}

// --- Tool 16: print_all_test_pages ---
func (r *Registry) registerPrintAllTestPages(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "print_all_test_pages",
		Description: "Print a Windows test page on every installed printer. Note: printers may have different page widths.",
		InputSchema: mcp.JSONSchema{Type: "object"},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		printers, err := printer.ListPrinters()
		if err != nil {
			return mcp.ErrorResult("Failed to list printers: %s", err), nil
		}
		printers = r.filterPrinters(printers)

		if len(printers) == 0 {
			return mcp.TextResult("No accessible printers found"), nil
		}

		results := make(map[string]string)
		for _, p := range printers {
			if err := printer.PrintTestPage(p.Name); err != nil {
				results[p.Name] = fmt.Sprintf("error: %s", err)
				r.logger.PrintJob(p.Name, "test_page", 1, err)
			} else {
				results[p.Name] = "test page sent"
				r.logger.PrintJob(p.Name, "test_page", 1, nil)
			}
		}
		return mcp.JSONResult(results)
	})
}

// --- Tool 17: get_ink_toner_levels ---
func (r *Registry) registerGetInkTonerLevels(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "get_ink_toner_levels",
		Description: "Get ink or toner supply levels for a printer (uses SNMP for network printers)",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
			},
			Required: []string{"printer_name"},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:        "Ink/Toner Levels",
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
		status, err := printer.GetInkTonerLevels(name)
		if err != nil {
			return mcp.ErrorResult("Failed to get ink/toner levels: %s", err), nil
		}
		return mcp.JSONResult(status)
	})
}

// --- Tool 18: get_print_history ---
func (r *Registry) registerGetPrintHistory(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "get_print_history",
		Description: "Get print history from Windows event log (requires PrintService/Operational log enabled)",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"days":         {Type: "integer", Description: "Number of days of history (default: 7)", Default: 7, Minimum: mcp.Float64Ptr(1), Maximum: mcp.Float64Ptr(90)},
				"printer_name": {Type: "string", Description: "Filter by printer name (optional)"},
			},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:        "Print History",
			ReadOnlyHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		days := mcp.GetIntArg(args, "days", 7)
		printerName := mcp.GetStringArg(args, "printer_name", "")
		if printerName != "" && !r.isPrinterAllowed(printerName) {
			return mcp.ErrorResult("Printer %q is not accessible", printerName), nil
		}
		entries, err := printer.GetPrintHistory(days, printerName)
		if err != nil {
			return mcp.ErrorResult("Failed to get print history: %s", err), nil
		}
		if len(entries) == 0 {
			return mcp.TextResult("No print history found"), nil
		}
		return mcp.JSONResult(entries)
	})
}

// --- Tool 19: test_printer_connectivity ---
func (r *Registry) registerTestPrinterConnectivity(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "test_printer_connectivity",
		Description: "Test printer connectivity: WMI status, ping, and port 9100",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
			},
			Required: []string{"printer_name"},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:        "Printer Connectivity",
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
		result, err := printer.TestPrinterConnectivity(name)
		if err != nil {
			return mcp.ErrorResult("Failed to test connectivity: %s", err), nil
		}
		return mcp.JSONResult(result)
	})
}

// --- Tool 20: purge_print_queue ---
func (r *Registry) registerPurgePrintQueue(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "purge_print_queue",
		Description: "Remove all jobs from a printer's queue",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
			},
			Required: []string{"printer_name"},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:           "Purge Queue",
			DestructiveHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := mcp.RequireStringArg(args, "printer_name")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		if !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		count, err := printer.PurgePrintQueue(name)
		if err != nil {
			return mcp.ErrorResult("Failed to purge queue: %s", err), nil
		}
		return mcp.TextResult(fmt.Sprintf("Purged %d jobs from %s", count, name)), nil
	})
}

// --- Tool 21: restart_print_job ---
func (r *Registry) registerRestartPrintJob(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "restart_print_job",
		Description: "Restart a specific print job",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
				"job_id":       {Type: "integer", Description: "Job ID to restart"},
			},
			Required: []string{"printer_name", "job_id"},
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
		if err := printer.RestartPrintJob(name, jobID); err != nil {
			return mcp.ErrorResult("Failed to restart job: %s", err), nil
		}
		return mcp.TextResult(fmt.Sprintf("Restarted job %d on %s", jobID, name)), nil
	})
}

// --- Tool 22: add_network_printer ---
func (r *Registry) registerAddNetworkPrinter(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "add_network_printer",
		Description: "Add a network printer by UNC path (e.g. \\\\server\\printer) or IP address",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"connection_name": {Type: "string", Description: "UNC path (e.g. \\\\server\\printer)"},
				"ip_address":     {Type: "string", Description: "Printer IP address (alternative to UNC)"},
				"driver_name":    {Type: "string", Description: "Driver name (required for IP mode)"},
				"printer_name":   {Type: "string", Description: "Display name (optional, for IP mode)"},
			},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		connName := mcp.GetStringArg(args, "connection_name", "")
		ipAddr := mcp.GetStringArg(args, "ip_address", "")
		driverName := mcp.GetStringArg(args, "driver_name", "")
		printerName := mcp.GetStringArg(args, "printer_name", "")

		if connName == "" && ipAddr == "" {
			return mcp.ErrorResult("Either connection_name (UNC path) or ip_address must be provided"), nil
		}

		if err := printer.AddNetworkPrinter(connName, ipAddr, driverName, printerName); err != nil {
			return mcp.ErrorResult("Failed to add printer: %s", err), nil
		}
		if connName != "" {
			return mcp.TextResult(fmt.Sprintf("Added network printer: %s", connName)), nil
		}
		if printerName == "" {
			printerName = fmt.Sprintf("Printer_%s", ipAddr)
		}
		return mcp.TextResult(fmt.Sprintf("Added printer %s at %s", printerName, ipAddr)), nil
	})
}

// --- Tool 23: remove_printer ---
func (r *Registry) registerRemovePrinter(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "remove_printer",
		Description: "Remove an installed printer",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name to remove"},
			},
			Required: []string{"printer_name"},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:           "Remove Printer",
			DestructiveHint: true,
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := mcp.RequireStringArg(args, "printer_name")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		if !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		if err := printer.RemovePrinter(name); err != nil {
			return mcp.ErrorResult("Failed to remove printer: %s", err), nil
		}
		return mcp.TextResult(fmt.Sprintf("Removed printer %s", name)), nil
	})
}

// --- Tool 24: set_print_defaults ---
func (r *Registry) registerSetPrintDefaults(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "set_print_defaults",
		Description: "Set default print configuration (paper size, color, duplex, collate)",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
				"paper_size":   {Type: "string", Description: "Paper size (e.g. Letter, A4)"},
				"color":        {Type: "boolean", Description: "Enable color printing"},
				"duplex_mode":  {Type: "string", Description: "Duplex mode", Enum: []string{"OneSided", "TwoSidedLongEdge", "TwoSidedShortEdge"}},
				"collate":      {Type: "boolean", Description: "Enable collation"},
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

		cfg := printer.PrintDefaultsConfig{
			PrinterName: name,
			PaperSize:   mcp.GetStringArg(args, "paper_size", ""),
			DuplexMode:  mcp.GetStringArg(args, "duplex_mode", ""),
		}
		if v, ok := args["color"]; ok {
			if b, ok := v.(bool); ok {
				cfg.Color = &b
			}
		}
		if v, ok := args["collate"]; ok {
			if b, ok := v.(bool); ok {
				cfg.Collate = &b
			}
		}

		if err := printer.SetPrintDefaults(cfg); err != nil {
			return mcp.ErrorResult("Failed to set defaults: %s", err), nil
		}
		return mcp.TextResult(fmt.Sprintf("Updated print defaults for %s", name)), nil
	})
}

// --- Tool 25: share_printer ---
func (r *Registry) registerSharePrinter(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "share_printer",
		Description: "Enable or disable printer sharing on the network",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
				"shared":       {Type: "boolean", Description: "Enable (true) or disable (false) sharing"},
				"share_name":   {Type: "string", Description: "Network share name (optional, used when sharing)"},
			},
			Required: []string{"printer_name", "shared"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		name, err := mcp.RequireStringArg(args, "printer_name")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		shared := mcp.GetBoolArg(args, "shared", false)
		shareName := mcp.GetStringArg(args, "share_name", "")

		if !r.isPrinterAllowed(name) {
			return mcp.ErrorResult("Printer %q is not accessible", name), nil
		}
		if err := printer.SharePrinter(name, shared, shareName); err != nil {
			return mcp.ErrorResult("Failed to update sharing: %s", err), nil
		}
		if shared {
			msg := fmt.Sprintf("Enabled sharing for %s", name)
			if shareName != "" {
				msg += fmt.Sprintf(" as \\\\%s", shareName)
			}
			return mcp.TextResult(msg), nil
		}
		return mcp.TextResult(fmt.Sprintf("Disabled sharing for %s", name)), nil
	})
}

// --- Tool 26: print_html ---
func (r *Registry) registerPrintHTML(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "print_html",
		Description: "Print HTML content using Windows built-in HTML renderer",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"html":         {Type: "string", Description: "HTML content to print"},
				"printer_name": {Type: "string", Description: "Target printer (default: system default)"},
			},
			Required: []string{"html"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		html, err := mcp.RequireStringArg(args, "html")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		printerName := mcp.GetStringArg(args, "printer_name", r.cfg.DefaultPrinter)
		if printerName != "" && !r.isPrinterAllowed(printerName) {
			return mcp.ErrorResult("Printer %q is not accessible", printerName), nil
		}
		if err := printer.PrintHTML(html, printerName); err != nil {
			return mcp.ErrorResult("Print HTML failed: %s", err), nil
		}
		target := printerName
		if target == "" {
			target = "default printer"
		}
		return mcp.TextResult(fmt.Sprintf("Sent HTML (%d chars) to %s", len(html), target)), nil
	})
}

// --- Tool 27: print_url ---
func (r *Registry) registerPrintURL(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "print_url",
		Description: "Download and print a web page URL",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"url":          {Type: "string", Description: "URL to download and print"},
				"printer_name": {Type: "string", Description: "Target printer (default: system default)"},
			},
			Required: []string{"url"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		url, err := mcp.RequireStringArg(args, "url")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		printerName := mcp.GetStringArg(args, "printer_name", r.cfg.DefaultPrinter)
		if printerName != "" && !r.isPrinterAllowed(printerName) {
			return mcp.ErrorResult("Printer %q is not accessible", printerName), nil
		}
		if err := printer.PrintURL(url, printerName); err != nil {
			return mcp.ErrorResult("Print URL failed: %s", err), nil
		}
		target := printerName
		if target == "" {
			target = "default printer"
		}
		return mcp.TextResult(fmt.Sprintf("Printed %s to %s", url, target)), nil
	})
}

// --- Tool 28: print_md ---
func (r *Registry) registerPrintMarkdown(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "print_md",
		Description: "Print Markdown content (converted to styled HTML)",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"markdown":     {Type: "string", Description: "Markdown content to print"},
				"printer_name": {Type: "string", Description: "Target printer (default: system default)"},
			},
			Required: []string{"markdown"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		md, err := mcp.RequireStringArg(args, "markdown")
		if err != nil {
			return mcp.ErrorResult("%s", err), nil
		}
		printerName := mcp.GetStringArg(args, "printer_name", r.cfg.DefaultPrinter)
		if printerName != "" && !r.isPrinterAllowed(printerName) {
			return mcp.ErrorResult("Printer %q is not accessible", printerName), nil
		}
		if err := printer.PrintMarkdown(md, printerName); err != nil {
			return mcp.ErrorResult("Print Markdown failed: %s", err), nil
		}
		target := printerName
		if target == "" {
			target = "default printer"
		}
		return mcp.TextResult(fmt.Sprintf("Sent Markdown (%d chars) to %s", len(md), target)), nil
	})
}

// --- Tool 29: print_multiple_files ---
func (r *Registry) registerPrintMultipleFiles(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "print_multiple_files",
		Description: "Print multiple files in batch (max 50 files)",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"file_paths": {
					Type:        "array",
					Description: "Array of file paths to print",
					Items:       &mcp.Property{Type: "string"},
				},
				"printer_name": {Type: "string", Description: "Target printer (default: system default)"},
			},
			Required: []string{"file_paths"},
		},
	}, func(args map[string]interface{}) (*mcp.CallToolResult, error) {
		filePaths := mcp.GetStringSliceArg(args, "file_paths")
		if len(filePaths) == 0 {
			return mcp.ErrorResult("file_paths must contain at least one file"), nil
		}
		// Validate all paths
		for _, fp := range filePaths {
			if !r.isPathAllowed(fp) {
				return mcp.ErrorResult("Path %q is not in the allowed paths list", fp), nil
			}
		}

		printerName := mcp.GetStringArg(args, "printer_name", r.cfg.DefaultPrinter)
		if printerName != "" && !r.isPrinterAllowed(printerName) {
			return mcp.ErrorResult("Printer %q is not accessible", printerName), nil
		}

		results := printer.PrintMultipleFiles(filePaths, printerName)
		return mcp.JSONResult(results)
	})
}

// --- Tool 30: get_printer_errors ---
func (r *Registry) registerGetPrinterErrors(s *mcp.Server) {
	s.RegisterTool(mcp.Tool{
		Name:        "get_printer_errors",
		Description: "Get error state and recent error events for a printer",
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"printer_name": {Type: "string", Description: "Printer name"},
			},
			Required: []string{"printer_name"},
		},
		Annotations: &mcp.ToolAnnotation{
			Title:        "Printer Errors",
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
		result, err := printer.GetPrinterErrors(name)
		if err != nil {
			return mcp.ErrorResult("Failed to get printer errors: %s", err), nil
		}
		return mcp.JSONResult(result)
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
