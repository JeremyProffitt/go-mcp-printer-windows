package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jeremyje/go-mcp-printer-windows/pkg/mcp"
	"github.com/jeremyje/go-mcp-printer-windows/pkg/printer"
)

// RegisterResources registers all MCP resources on the server.
func (r *Registry) RegisterResources(s *mcp.Server) {
	r.registerPrinterListResource(s)
	r.registerConfigResource(s)
	r.registerHelpResource(s)
}

func (r *Registry) registerPrinterListResource(s *mcp.Server) {
	s.RegisterResource(mcp.Resource{
		URI:         "printer://list",
		Name:        "Printer Inventory",
		Description: "All installed printers with drivers, types, capabilities, and paper sizes",
		MimeType:    "application/json",
	}, func() (string, error) {
		printers, err := printer.ListPrintersWithPaperSizes()
		if err != nil {
			return "", err
		}
		data, err := json.MarshalIndent(printers, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
}

func (r *Registry) registerConfigResource(s *mcp.Server) {
	s.RegisterResource(mcp.Resource{
		URI:         "printer://config",
		Name:        "Server Configuration",
		Description: "Current server configuration: allowed/blocked printers, photo printers, rate limits",
		MimeType:    "application/json",
	}, func() (string, error) {
		// Expose config without sensitive fields
		safe := map[string]interface{}{
			"domain":          r.cfg.Domain,
			"port":            r.cfg.Port,
			"logLevel":        r.cfg.LogLevel,
			"defaultPrinter":  r.cfg.DefaultPrinter,
			"allowedPrinters": r.cfg.AllowedPrinters,
			"blockedPrinters": r.cfg.BlockedPrinters,
			"photoPrinters":   r.cfg.PhotoPrinters,
			"allowedPaths":    r.cfg.AllowedPaths,
			"rateLimitCalls":  r.cfg.RateLimitCalls,
			"rateLimitWindow": r.cfg.RateLimitWindow,
		}
		data, err := json.MarshalIndent(safe, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
}

func (r *Registry) registerHelpResource(s *mcp.Server) {
	s.RegisterResource(mcp.Resource{
		URI:         "printer://help",
		Name:        "Printing Guide",
		Description: "Supported file formats, print options, photo vs document tips, and duplex guidance",
		MimeType:    "text/markdown",
	}, func() (string, error) {
		var b strings.Builder
		b.WriteString("# Go MCP Printer — Quick Reference\n\n")

		b.WriteString("## Supported File Formats\n")
		b.WriteString("- **Documents**: PDF, TXT, DOC, DOCX, XLS, XLSX, PPT, PPTX\n")
		b.WriteString("- **Images**: JPG, PNG, BMP, GIF, TIFF\n")
		b.WriteString("- **Web content**: HTML (print_html), URL (print_url), Markdown (print_md)\n")
		b.WriteString("- **Raw text**: Any text content via print_text\n\n")

		b.WriteString("## Print Options\n")
		b.WriteString("| Option | Values | Notes |\n")
		b.WriteString("|--------|--------|-------|\n")
		b.WriteString("| copies | 1–999 | Default 1 |\n")
		b.WriteString("| duplex | None, TwoSidedLongEdge, TwoSidedShortEdge | Printer must support duplex |\n")
		b.WriteString("| orientation | Portrait, Landscape | Default Portrait |\n")
		b.WriteString("| color | Color, Grayscale | Default Color |\n")
		b.WriteString("| quality | Draft, Normal, High | Default Normal |\n")
		b.WriteString("| paperSize | Letter, Legal, A4, 4x6, etc. | Use list_printer_paper_sizes for full list |\n")
		b.WriteString("| fitToPage | true/false | Scale content to fit paper |\n\n")

		b.WriteString("## Photo Printing Tips\n")
		b.WriteString("- Use `print_image` for photos — it applies photo-optimized settings automatically\n")
		b.WriteString("- Photo printers (dye-sub) are tagged with `category: photo` in the printer list\n")
		b.WriteString("- For best results: set quality=High, use appropriate paper size (4x6 for photos)\n")
		b.WriteString("- Photo printers typically support: Glossy, Matte, and Luster media types\n\n")

		b.WriteString("## Troubleshooting\n")
		b.WriteString("1. **Printer not responding**: Run `test_printer_connectivity` to check WMI, ping, and port 9100\n")
		b.WriteString("2. **Low ink/toner**: Run `get_ink_toner_levels` (uses SNMP for network printers)\n")
		b.WriteString("3. **Stuck jobs**: Run `get_print_queue` then `cancel_print_job` or `purge_print_queue`\n")
		b.WriteString("4. **Wrong output**: Check `get_printer_details` for capabilities and `set_print_defaults`\n")

		b.WriteString(fmt.Sprintf("\n---\nServer version: %s\n", r.version))
		return b.String(), nil
	})
}
