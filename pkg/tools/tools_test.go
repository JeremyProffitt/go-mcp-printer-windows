package tools

import (
	"testing"
	"time"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/config"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/mcp"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/printer"
)

func TestRegisterAll(t *testing.T) {
	cfg := config.DefaultConfig()
	s := mcp.NewServer("test", "1.0.0", 10, 20*time.Second)
	r := NewRegistry(cfg, nil, "1.0.0")
	r.RegisterAll(s)

	if s.ToolCount() != 14 {
		t.Errorf("Expected 14 tools, got %d", s.ToolCount())
	}

	tools := s.Tools()
	expectedNames := []string{
		"list_printers",
		"get_printer_details",
		"get_default_printer",
		"print_file",
		"print_text",
		"print_image",
		"get_print_queue",
		"get_print_job_status",
		"cancel_print_job",
		"pause_printer",
		"resume_printer",
		"set_default_printer",
		"print_test_page",
		"get_printer_server_status",
	}

	nameSet := make(map[string]bool)
	for _, tool := range tools {
		nameSet[tool.Name] = true
	}

	for _, name := range expectedNames {
		if !nameSet[name] {
			t.Errorf("Missing tool: %s", name)
		}
	}
}

func TestFilterPrinters(t *testing.T) {
	printers := []printer.PrinterInfo{
		{Name: "HP LaserJet"},
		{Name: "DNP DS620"},
		{Name: "Microsoft Print to PDF"},
	}

	// No filters
	r := &Registry{cfg: &config.Config{}}
	result := r.filterPrinters(printers)
	if len(result) != 3 {
		t.Errorf("No filters: expected 3, got %d", len(result))
	}

	// Blocked
	r = &Registry{cfg: &config.Config{BlockedPrinters: []string{"Microsoft Print to PDF"}}}
	result = r.filterPrinters(printers)
	if len(result) != 2 {
		t.Errorf("Blocked: expected 2, got %d", len(result))
	}

	// Allowed only
	r = &Registry{cfg: &config.Config{AllowedPrinters: []string{"HP LaserJet"}}}
	result = r.filterPrinters(printers)
	if len(result) != 1 {
		t.Errorf("Allowed: expected 1, got %d", len(result))
	}
}

func TestIsPrinterAllowed(t *testing.T) {
	r := &Registry{cfg: &config.Config{
		BlockedPrinters: []string{"blocked-printer"},
		AllowedPrinters: []string{"allowed-printer"},
	}}

	if r.isPrinterAllowed("blocked-printer") {
		t.Error("blocked-printer should not be allowed")
	}
	if !r.isPrinterAllowed("allowed-printer") {
		t.Error("allowed-printer should be allowed")
	}
	if r.isPrinterAllowed("other-printer") {
		t.Error("other-printer should not be allowed when AllowedPrinters is set")
	}
}

func TestIsPathAllowed(t *testing.T) {
	// No restrictions
	r := &Registry{cfg: &config.Config{}}
	if !r.isPathAllowed("C:\\any\\path") {
		t.Error("Should allow any path when no restrictions")
	}

	// With restrictions
	r = &Registry{cfg: &config.Config{AllowedPaths: []string{"C:\\Users\\test\\Documents"}}}
	if !r.isPathAllowed("C:\\Users\\test\\Documents\\file.pdf") {
		t.Error("Should allow path within allowed directory")
	}
}
