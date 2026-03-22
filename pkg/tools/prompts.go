package tools

import (
	"fmt"
	"strings"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/mcp"
)

// RegisterPrompts registers all MCP prompt templates on the server.
func (r *Registry) RegisterPrompts(s *mcp.Server) {
	r.registerDiagnosePrinterPrompt(s)
	r.registerPrintDocumentPrompt(s)
	r.registerSupplyCheckPrompt(s)
	r.registerSetupPrinterPrompt(s)
	r.registerQueueCleanupPrompt(s)
}

func (r *Registry) registerDiagnosePrinterPrompt(s *mcp.Server) {
	s.RegisterPrompt(mcp.Prompt{
		Name:        "diagnose-printer",
		Description: "Troubleshoot a printer: connectivity, errors, ink levels, and queue status",
		Arguments: []mcp.PromptArgument{
			{Name: "printer", Description: "Printer name to diagnose", Required: true},
		},
	}, func(args map[string]string) (*mcp.GetPromptResult, error) {
		name := args["printer"]
		if name == "" {
			return nil, fmt.Errorf("printer name is required")
		}
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Diagnose printer: %s", name),
			Messages: []mcp.PromptMessage{{
				Role: "user",
				Content: mcp.ContentItem{
					Type: "text",
					Text: fmt.Sprintf(
						"Please diagnose the printer %q by running these steps in order:\n\n"+
							"1. Run `test_printer_connectivity` for %q to check if it's reachable (WMI status, ping, port 9100)\n"+
							"2. Run `get_printer_errors` for %q to check for error states\n"+
							"3. Run `get_ink_toner_levels` for %q to check supply levels\n"+
							"4. Run `get_print_queue` for %q to check for stuck or errored jobs\n"+
							"5. Run `get_printer_details` for %q to review its current configuration\n\n"+
							"Summarize all findings and recommend specific actions to fix any issues found.",
						name, name, name, name, name, name),
				},
			}},
		}, nil
	})
}

func (r *Registry) registerPrintDocumentPrompt(s *mcp.Server) {
	s.RegisterPrompt(mcp.Prompt{
		Name:        "print-document",
		Description: "Smart print: auto-detect format, pick best printer and settings",
		Arguments: []mcp.PromptArgument{
			{Name: "file", Description: "File path or content to print", Required: true},
			{Name: "printer", Description: "Target printer (optional, auto-selects if omitted)", Required: false},
		},
	}, func(args map[string]string) (*mcp.GetPromptResult, error) {
		file := args["file"]
		if file == "" {
			return nil, fmt.Errorf("file is required")
		}
		printerHint := ""
		if p := args["printer"]; p != "" {
			printerHint = fmt.Sprintf(" on printer %q", p)
		}

		// Detect file type for guidance
		lower := strings.ToLower(file)
		var guidance string
		switch {
		case strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
			strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".tiff"):
			guidance = "This is an image file. Use `print_image` which applies photo-optimized settings. " +
				"If a photo printer (category=photo) is available, prefer it. Set quality=High."
		case strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm"):
			guidance = "This is an HTML file. Use `print_html` to render it properly."
		case strings.HasSuffix(lower, ".md"):
			guidance = "This is a Markdown file. Use `print_md` to convert to styled HTML and print."
		default:
			guidance = "Use `print_file` for this file type."
		}

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Print %s%s", file, printerHint),
			Messages: []mcp.PromptMessage{{
				Role: "user",
				Content: mcp.ContentItem{
					Type: "text",
					Text: fmt.Sprintf(
						"Print the file %q%s.\n\n"+
							"Steps:\n"+
							"1. Run `list_printers` to see available printers and their capabilities\n"+
							"2. %s\n"+
							"3. Choose appropriate settings (paper size, orientation, quality) based on the content\n"+
							"4. Print and confirm the job was submitted successfully\n\n"+
							"If no printer is specified, pick the best match based on the file type and available printers.",
						file, printerHint, guidance),
				},
			}},
		}, nil
	})
}

func (r *Registry) registerSupplyCheckPrompt(s *mcp.Server) {
	s.RegisterPrompt(mcp.Prompt{
		Name:        "supply-check",
		Description: "Check ink/toner levels across all printers and flag low supplies",
	}, func(args map[string]string) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Check supplies across all printers",
			Messages: []mcp.PromptMessage{{
				Role: "user",
				Content: mcp.ContentItem{
					Type: "text",
					Text: "Check ink and toner levels for all installed printers:\n\n" +
						"1. Run `list_printers` to get all printer names\n" +
						"2. Run `get_ink_toner_levels` for each printer\n" +
						"3. Summarize the results in a table: Printer | Supply | Level | Status\n" +
						"4. Flag any supplies below 20% as needing replacement soon\n" +
						"5. Flag any supplies below 10% as critical\n\n" +
						"Note: SNMP-based levels are only available for network printers.",
				},
			}},
		}, nil
	})
}

func (r *Registry) registerSetupPrinterPrompt(s *mcp.Server) {
	s.RegisterPrompt(mcp.Prompt{
		Name:        "setup-printer",
		Description: "Guided network printer setup: add, test, configure defaults",
		Arguments: []mcp.PromptArgument{
			{Name: "address", Description: "Printer IP address or UNC path (e.g. 192.168.1.100 or \\\\server\\printer)", Required: true},
		},
	}, func(args map[string]string) (*mcp.GetPromptResult, error) {
		addr := args["address"]
		if addr == "" {
			return nil, fmt.Errorf("address is required")
		}
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Set up printer at %s", addr),
			Messages: []mcp.PromptMessage{{
				Role: "user",
				Content: mcp.ContentItem{
					Type: "text",
					Text: fmt.Sprintf(
						"Set up a new network printer at %q:\n\n"+
							"1. Run `add_network_printer` with the address %q\n"+
							"   - If it's an IP address, a driver name may be needed — check existing printers for common drivers\n"+
							"2. Run `test_printer_connectivity` to verify it's reachable\n"+
							"3. Run `get_printer_details` to see its capabilities\n"+
							"4. Run `print_test_page` to verify it prints correctly\n"+
							"5. Run `set_print_defaults` to configure sensible defaults (paper size, color mode)\n"+
							"6. Ask if this should be set as the default printer\n\n"+
							"Report the result of each step and stop if any step fails.",
						addr, addr),
				},
			}},
		}, nil
	})
}

func (r *Registry) registerQueueCleanupPrompt(s *mcp.Server) {
	s.RegisterPrompt(mcp.Prompt{
		Name:        "queue-cleanup",
		Description: "Find and clean up stuck or errored print jobs",
		Arguments: []mcp.PromptArgument{
			{Name: "printer", Description: "Specific printer to clean (optional, checks all if omitted)", Required: false},
		},
	}, func(args map[string]string) (*mcp.GetPromptResult, error) {
		target := "all printers"
		extra := ""
		if p := args["printer"]; p != "" {
			target = fmt.Sprintf("printer %q", p)
			extra = fmt.Sprintf(" for %q", p)
		}
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Clean up print queue for %s", target),
			Messages: []mcp.PromptMessage{{
				Role: "user",
				Content: mcp.ContentItem{
					Type: "text",
					Text: fmt.Sprintf(
						"Clean up the print queue for %s:\n\n"+
							"1. Run `get_print_queue`%s to see all pending jobs\n"+
							"2. Identify any jobs with error or stuck status\n"+
							"3. For each stuck job, try `restart_print_job` first\n"+
							"4. If restart fails, use `cancel_print_job` to remove it\n"+
							"5. If many jobs are stuck, ask before using `purge_print_queue`\n"+
							"6. Report: how many jobs were found, restarted, cancelled, and remaining\n\n"+
							"Be careful with purge — confirm with the user first as it removes ALL jobs.",
						target, extra),
				},
			}},
		}, nil
	})
}
