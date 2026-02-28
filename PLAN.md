# go-mcp-printer-windows — Implementation Plan

## Overview

A Go-based MCP (Model Context Protocol) server for **Windows printer management**. Enables AI assistants (Claude Desktop, Claude Code, Cursor, etc.) to discover, configure, and print to local printers via the Windows printing subsystem. Supports both **stdio** and **HTTP** transports, runs as a **Windows service**, and includes verbose structured logging.

**Target printers on this machine:**
| Printer | Driver | Port | Type |
|---------|--------|------|------|
| DP-DS820 | DP-DS820 | USB001 | DNP dye-sub photo printer |
| DP-QW410 | DP-QW410 | USB002 | DNP dye-sub photo printer |
| HP ColorLaserJet MFP M282-M285 PCL-6 (V4) | HP PCL-6 V4 | 192.168.1.118 (network) | HP Color LaserJet |
| NPI82EA3D (HP Color LaserJet MFP M283fdw) | HP PCL-6 V4 | WSD | HP Color LaserJet (WSD) |
| Microsoft Print to PDF | Microsoft Print To PDF | PORTPROMPT: | Virtual PDF printer |
| OneNote (Desktop) | OneNote 16 Driver | nul: | Virtual OneNote printer |

**Design goal:** Broad printer support via Windows native APIs — not hardcoded to any vendor. The HP and DNP dye-sub printers above are validation targets, but any Windows-installed printer should work.

---

## Architecture

```
go-mcp-printer-windows/
├── main.go                          # Entry point: CLI flags, transport selection, startup
├── go.mod                           # Module definition (zero external deps)
├── build.sh                         # Cross-compile build script (Windows amd64/arm64)
├── install-service.ps1              # PowerShell: install as Windows service
├── uninstall-service.ps1            # PowerShell: uninstall Windows service
├── README.md                        # Full documentation
├── CLAUDE.md                        # LLM project guidelines
├── PLAN.md                          # This file
│
├── pkg/
│   ├── logging/
│   │   ├── logging.go               # Structured file logger with levels, PII filtering
│   │   └── logging_test.go          # Logger tests
│   │
│   ├── mcp/
│   │   ├── server.go                # MCP JSON-RPC server (stdio + HTTP transports)
│   │   ├── types.go                 # MCP protocol types (Tool, Resource, JSON-RPC, etc.)
│   │   └── server_test.go           # Server tests
│   │
│   ├── printer/
│   │   ├── windows.go               # Windows printing backend (PowerShell + Win32 API via syscall)
│   │   ├── types.go                 # Printer data types (PrinterInfo, PrintJob, PrintOptions)
│   │   └── windows_test.go          # Printer backend tests
│   │
│   └── tools/
│       ├── tools.go                 # MCP tool registry + all tool definitions/handlers
│       └── tools_test.go            # Tool handler tests
│
└── testdata/
    ├── sample.txt                   # Test file for plain text printing
    ├── sample.pdf                   # Test file for PDF printing
    └── sample.jpg                   # Test file for image printing (dye-sub validation)
```

---

## Phase 1: Project Scaffolding

### 1.1 — go.mod
- Module: `github.com/JeremyProffitt/go-mcp-printer-windows`
- Go version: `1.21` (matches template, zero external deps)
- No external dependencies — pure standard library + Windows syscall

### 1.2 — CLAUDE.md
- Project description and conventions
- Build/test/run commands
- Architecture overview for LLM context

### 1.3 — build.sh
Based on the `go-mcp-dynatrace` build script pattern:
- Build targets: `windows/amd64`, `windows/arm64`
- Inject version via `-X main.Version=$VERSION`
- Strip with `-ldflags="-s -w"`
- Output to `dist/` directory
- Named: `go-mcp-printer-windows-amd64.exe`, `go-mcp-printer-windows-arm64.exe`

---

## Phase 2: Core Infrastructure

### 2.1 — Logging (`pkg/logging/logging.go`)
Ported from `go-mcp-dynatrace` logging package with adaptations:

**Features:**
- **Log levels:** OFF, ERROR, WARN, INFO, ACCESS, DEBUG
- **File-based logging:** `{logDir}/go-mcp-printer-windows-YYYY-MM-DD.log`
- **Default log dir:** `~/.go-mcp-printer-windows/logs/`
- **Structured format:** `[2026-02-28T14:30:00.123-06:00] [INFO] message`
- **PII filtering:** Mask sensitive data in log output
- **Startup/shutdown logging:** Config source tracking (flag vs env vs default)
- **Verbose printer operation logging:**
  - Log every PowerShell command executed (DEBUG level)
  - Log every printer enumeration result (DEBUG level)
  - Log print job submission details (INFO level)
  - Log print job status changes (INFO level)
  - Log driver/capability queries (DEBUG level)
  - Log errors with full context (ERROR level)

**Environment variables:**
- `MCP_LOG_DIR` — Custom log directory
- `MCP_LOG_LEVEL` — Log level override

**Methods:**
```go
Info(format, args...)
Debug(format, args...)
Warn(format, args...)
Error(format, args...)
PrinterOp(operation, printerName, details)   // Printer-specific structured log
PrintJob(jobID, printerName, status, details) // Print job tracking
PowerShell(command, stdout, stderr, err)       // PS command execution log
Startup() / Shutdown()
```

### 2.2 — MCP Protocol (`pkg/mcp/`)

#### `pkg/mcp/types.go`
Ported from template. MCP protocol types:
- `JSONRPCRequest`, `JSONRPCResponse`, `JSONRPCError`
- `Tool`, `ToolAnnotation`, `JSONSchema`, `Property`
- `CallToolResult`, `TextContent`, `ImageContent`
- `Resource`, `ResourceTemplate`, `ResourceProvider`
- `Prompt`, `PromptMessage`, `PromptProvider`
- `ServerInfo`, `ServerCapabilities`
- `InitializeResult`, `ToolsListResult`, `ResourcesListResult`

#### `pkg/mcp/server.go`
Ported from template with enhancements:

**Stdio transport (default):**
- Line-buffered JSON-RPC over stdin/stdout
- 30-second initial connection timeout
- EOF detection for graceful shutdown
- All logging goes to file (never stdout — that's the MCP channel)

**HTTP transport (`--http` flag):**
- `POST /` — MCP JSON-RPC endpoint
- `GET /health` — Health check (returns printer count + status)
- Optional auth via `MCP_AUTH_TOKEN` env var
- CORS headers for browser-based clients
- Configurable host/port

**Server capabilities advertised:**
```json
{
  "tools": { "listChanged": true },
  "resources": { "subscribe": false, "listChanged": false }
}
```

**Rate limiting:** 10 calls per 20 seconds (more generous than template since printing is inherently slow)

### 2.3 — `main.go`
Entry point following the `go-mcp-dynatrace` pattern:

**CLI flags:**
```
-log-dir <path>       Log directory (default: ~/.go-mcp-printer-windows/logs/)
-log-level <level>    Log level: off|error|warn|info|access|debug (default: info)
--http                Run in HTTP mode (default: stdio)
-p, --port <int>      HTTP port (default: 3001)
-H, --host <string>   HTTP host (default: 127.0.0.1)
-version              Show version
-help                 Show help
```

**Startup sequence:**
1. Load `~/.mcp_env` if present
2. Parse CLI flags
3. Initialize logger (resolve log dir from flag > env > default)
4. Log startup info (version, platform, transport, config sources)
5. Initialize printer backend (enumerate printers, log discoveries)
6. Create MCP server (stdio or HTTP)
7. Register all tools
8. Start server
9. Graceful shutdown on SIGINT/SIGTERM (or stdin EOF for stdio)

**Environment variables:**
| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `MCP_LOG_DIR` | No | `~/.go-mcp-printer-windows/logs/` | Log directory |
| `MCP_LOG_LEVEL` | No | `info` | Log level |
| `MCP_AUTH_TOKEN` | No | — | HTTP mode auth token |
| `MCP_PRINTER_DEFAULT` | No | — | Default printer name |
| `MCP_PRINTER_ALLOWED` | No | `*` (all) | Comma-separated allowed printer names |
| `MCP_PRINTER_BLOCKED` | No | — | Comma-separated blocked printer names |
| `MCP_PRINT_ALLOWED_PATHS` | No | `~/Documents,~/Downloads,~/Desktop` | Allowed file paths for printing |

---

## Phase 3: Windows Printing Backend

### 3.1 — Printer Types (`pkg/printer/types.go`)

```go
// PrinterInfo represents a discovered Windows printer
type PrinterInfo struct {
    Name         string            `json:"name"`
    DriverName   string            `json:"driverName"`
    PortName     string            `json:"portName"`
    Location     string            `json:"location"`
    Comment      string            `json:"comment"`
    Status       string            `json:"status"`       // Idle, Printing, Error, Offline, etc.
    StatusCode   int               `json:"statusCode"`
    IsDefault    bool              `json:"isDefault"`
    IsNetwork    bool              `json:"isNetwork"`
    IsShared     bool              `json:"isShared"`
    PrinterType  string            `json:"printerType"`  // Local, Network, Virtual
    Capabilities PrinterCapabilities `json:"capabilities"`
}

// PrinterCapabilities describes what the printer supports
type PrinterCapabilities struct {
    SupportsColor     bool     `json:"supportsColor"`
    SupportsDuplex    bool     `json:"supportsDuplex"`
    SupportsCollation bool     `json:"supportsCollation"`
    SupportedPapers   []string `json:"supportedPapers"`   // Letter, A4, 4x6, 5x7, 6x8, etc.
    SupportedMedias   []string `json:"supportedMedias"`   // Plain, Photo, Glossy, etc.
    MaxResolutionDPI  int      `json:"maxResolutionDpi"`
    SupportedOrientations []string `json:"supportedOrientations"` // Portrait, Landscape
}

// PrintJob represents a submitted or tracked print job
type PrintJob struct {
    JobID       int    `json:"jobId"`
    PrinterName string `json:"printerName"`
    Document    string `json:"document"`
    Status      string `json:"status"`      // Spooling, Printing, Printed, Error, Cancelled, etc.
    Owner       string `json:"owner"`
    Pages       int    `json:"pages"`
    Size        int64  `json:"size"`
    SubmittedAt string `json:"submittedAt"`
}

// PrintOptions configures a print operation
type PrintOptions struct {
    PrinterName  string `json:"printerName"`  // Target printer (empty = default)
    Copies       int    `json:"copies"`       // Number of copies (default: 1)
    Color        bool   `json:"color"`        // Color mode (default: true)
    Duplex       string `json:"duplex"`       // None, TwoSidedLongEdge, TwoSidedShortEdge
    Orientation  string `json:"orientation"`  // Portrait, Landscape
    PaperSize    string `json:"paperSize"`    // Letter, A4, 4x6, 5x7, 6x8, etc.
    MediaType    string `json:"mediaType"`    // Plain, Photo, Glossy, etc.
    Quality      string `json:"quality"`      // Draft, Normal, High
    PageRange    string `json:"pageRange"`    // e.g., "1-3,5,7-10"
    FitToPage    bool   `json:"fitToPage"`    // Scale content to fit page
}
```

### 3.2 — Windows Backend (`pkg/printer/windows.go`)

**Implementation strategy: PowerShell-first with robust error handling.**

Windows printing can be accessed via:
1. **PowerShell cmdlets** (`Get-Printer`, `Get-PrintJob`, `Out-Printer`, `Get-PrinterProperty`) — most reliable, broadest compatibility
2. **Win32 API via syscall** (`winspool.drv`) — lower level, for capabilities not exposed by PowerShell
3. **`print` command** — simple but limited

We use **PowerShell as primary** with **syscall fallback** for advanced features.

#### Functions:

```go
// Backend is the Windows printing interface
type Backend struct {
    logger *logging.Logger
}

// ListPrinters enumerates all installed printers with full details
// Uses: Get-Printer | Select-Object *
// Logs: Every printer found at DEBUG, count at INFO
func (b *Backend) ListPrinters() ([]PrinterInfo, error)

// GetPrinter gets detailed info for a specific printer by name
// Uses: Get-Printer -Name "..." | Select-Object *
func (b *Backend) GetPrinter(name string) (*PrinterInfo, error)

// GetDefaultPrinter returns the system default printer
// Uses: Get-CimInstance -ClassName Win32_Printer -Filter "Default=True"
func (b *Backend) GetDefaultPrinter() (*PrinterInfo, error)

// GetPrinterCapabilities queries printer capabilities
// Uses: Get-PrinterProperty -PrinterName "..." + Get-CimInstance Win32_Printer
// This is critical for dye-sub printers which support specific paper sizes
func (b *Backend) GetPrinterCapabilities(name string) (*PrinterCapabilities, error)

// PrintFile sends a file to a printer
// Strategy by file type:
//   - .txt          → Out-Printer cmdlet
//   - .pdf          → Start-Process with -Verb Print (uses system PDF handler)
//   - .jpg/.png/.bmp → Start-Process with -Verb Print (uses Windows Photo Viewer/handler)
//   - .docx/.xlsx   → Start-Process with -Verb Print (uses associated Office app)
// Logs: File path, printer name, options, job ID at INFO
// Logs: Full PowerShell command at DEBUG
func (b *Backend) PrintFile(filePath string, opts PrintOptions) (*PrintJob, error)

// PrintText sends raw text to a printer
// Uses: Out-Printer -Name "..."
// Supports multi-line text content
func (b *Backend) PrintText(text string, opts PrintOptions) (*PrintJob, error)

// GetPrintQueue lists jobs in a printer's queue
// Uses: Get-PrintJob -PrinterName "..."
func (b *Backend) GetPrintQueue(printerName string) ([]PrintJob, error)

// GetPrintJob gets status of a specific print job
// Uses: Get-PrintJob -PrinterName "..." -ID <jobID>
func (b *Backend) GetPrintJob(printerName string, jobID int) (*PrintJob, error)

// CancelPrintJob cancels a specific print job
// Uses: Remove-PrintJob -PrinterName "..." -ID <jobID>
func (b *Backend) CancelPrintJob(printerName string, jobID int) error

// PausePrinter pauses a printer's queue
// Uses: Set-Printer -Name "..." -Paused $true (requires admin)
func (b *Backend) PausePrinter(name string) error

// ResumePrinter resumes a paused printer's queue
// Uses: Set-Printer -Name "..." -Paused $false (requires admin)
func (b *Backend) ResumePrinter(name string) error

// TestPage prints a test page to verify printer connectivity
// Uses: rundll32 printui.dll,PrintUIEntry /k /n "..."
func (b *Backend) TestPage(printerName string) error

// SetDefaultPrinter sets the system default printer
// Uses: (New-Object -ComObject WScript.Network).SetDefaultPrinter("...")
func (b *Backend) SetDefaultPrinter(name string) error
```

#### PowerShell execution helper:

```go
// execPowerShell runs a PowerShell command and returns stdout, stderr
// - Uses powershell.exe -NoProfile -NonInteractive -Command "..."
// - Timeout: 30 seconds per command
// - Logs the full command at DEBUG level
// - Logs stdout/stderr at DEBUG level
// - Logs errors at ERROR level with full context
func (b *Backend) execPowerShell(command string) (string, string, error)
```

#### Dye-sub printer considerations:
- DNP DS820 and QW410 support specific paper sizes: 4x6, 5x7, 6x8, 6x9, 8x10, 8x12
- They may expose these via `Get-PrinterProperty` or the driver's devmode
- Image printing is the primary use case — JPG/PNG files
- Color is always enabled (thermal dye-sub is inherently color)
- `MediaType` matters — these printers use different ribbon/media combinations

#### HP LaserJet considerations:
- Supports standard paper sizes (Letter, Legal, A4, envelopes)
- Supports duplex printing
- Supports color and B&W modes
- Network printer — may have WSD discovery delays
- PCL-6 driver — broad document format support

---

## Phase 4: MCP Tool Definitions

### 4.1 — Tool Registry (`pkg/tools/tools.go`)

Following the `go-mcp-dynatrace` registry pattern:

```go
type Registry struct {
    backend *printer.Backend
    logger  *logging.Logger
    config  *Config
}

type Config struct {
    DefaultPrinter string
    AllowedPrinters []string  // Empty = all allowed
    BlockedPrinters []string
    AllowedPaths    []string
}

func (r *Registry) RegisterAll(server *mcp.Server)
```

### 4.2 — Tool Catalog (14 tools)

#### Discovery Tools

**1. `list_printers`**
- Description: "List all installed printers on this Windows machine with their status, type, and capabilities"
- Input: none
- Output: Formatted table of all printers with name, driver, status, type (local/network/virtual), default marker
- Annotations: `ReadOnlyHint: true`
- Logging: Logs printer count, each printer at DEBUG

**2. `get_printer_details`**
- Description: "Get detailed information about a specific printer including capabilities, supported paper sizes, media types, and current status"
- Input: `printerName` (string, required)
- Output: Full printer info + capabilities JSON
- Annotations: `ReadOnlyHint: true`
- Logging: Logs query and result

**3. `get_default_printer`**
- Description: "Get the current default printer"
- Input: none
- Output: Default printer details
- Annotations: `ReadOnlyHint: true`

#### Printing Tools

**4. `print_file`**
- Description: "Print a file to a specified printer. Supports PDF, images (JPG, PNG, BMP, TIFF), text files, and Office documents. Automatically uses the appropriate Windows print handler."
- Input:
  - `filePath` (string, required) — Absolute path to the file
  - `printerName` (string, optional) — Target printer (default: system default or `MCP_PRINTER_DEFAULT`)
  - `copies` (integer, optional, default: 1)
  - `color` (boolean, optional, default: true)
  - `duplex` (string, optional, enum: None/TwoSidedLongEdge/TwoSidedShortEdge)
  - `orientation` (string, optional, enum: Portrait/Landscape)
  - `paperSize` (string, optional) — e.g., "Letter", "A4", "4x6", "5x7"
  - `mediaType` (string, optional) — e.g., "Plain", "Photo", "Glossy"
  - `quality` (string, optional, enum: Draft/Normal/High)
  - `pageRange` (string, optional) — e.g., "1-3,5"
  - `fitToPage` (boolean, optional, default: false)
- Output: Print job ID, printer name, status, confirmation message
- Annotations: `ReadOnlyHint: false`
- **Security:** Validates `filePath` against `MCP_PRINT_ALLOWED_PATHS`. Rejects paths containing `..`, hidden directories, or paths outside allowed directories.
- Logging: Logs file path, printer, all options at INFO; full PS command at DEBUG

**5. `print_text`**
- Description: "Print text content directly to a printer. Useful for quick notes, code snippets, or generated content."
- Input:
  - `text` (string, required) — Text content to print
  - `printerName` (string, optional)
  - `copies` (integer, optional, default: 1)
  - `orientation` (string, optional, enum: Portrait/Landscape)
  - `paperSize` (string, optional)
- Output: Print job confirmation
- Annotations: `ReadOnlyHint: false`
- Logging: Logs text length (not content), printer, options at INFO

**6. `print_image`**
- Description: "Print an image file with photo-optimized settings. Ideal for dye-sub printers (DNP DS820, QW410) and photo printing. Automatically sets high quality and photo media type."
- Input:
  - `filePath` (string, required) — Path to image (JPG, PNG, BMP, TIFF)
  - `printerName` (string, optional)
  - `copies` (integer, optional, default: 1)
  - `paperSize` (string, optional) — e.g., "4x6", "5x7", "6x8", "8x10"
  - `fitToPage` (boolean, optional, default: true)
  - `orientation` (string, optional, enum: Portrait/Landscape/Auto)
- Output: Print job confirmation
- Annotations: `ReadOnlyHint: false`
- Logging: Logs image dimensions, file size, printer at INFO
- **Note:** This is a convenience wrapper around `print_file` with photo-optimized defaults (High quality, Photo media, Color on, FitToPage on)

#### Queue Management Tools

**7. `get_print_queue`**
- Description: "View the print queue for a specific printer, showing all pending, printing, and completed jobs"
- Input: `printerName` (string, required)
- Output: List of print jobs with ID, document name, status, pages, size, submitted time
- Annotations: `ReadOnlyHint: true`
- Logging: Logs printer name, job count

**8. `get_print_job_status`**
- Description: "Check the status of a specific print job"
- Input:
  - `printerName` (string, required)
  - `jobId` (integer, required)
- Output: Job details (status, pages printed, error info)
- Annotations: `ReadOnlyHint: true`

**9. `cancel_print_job`**
- Description: "Cancel a specific print job in a printer's queue"
- Input:
  - `printerName` (string, required)
  - `jobId` (integer, required)
- Output: Confirmation of cancellation
- Annotations: `ReadOnlyHint: false, DestructiveHint: true`
- Logging: Logs cancellation at WARN level

#### Printer Control Tools

**10. `pause_printer`**
- Description: "Pause a printer's queue (stops processing new jobs). Requires administrator privileges."
- Input: `printerName` (string, required)
- Output: Confirmation
- Annotations: `ReadOnlyHint: false`
- Logging: Logs at WARN

**11. `resume_printer`**
- Description: "Resume a paused printer's queue. Requires administrator privileges."
- Input: `printerName` (string, required)
- Output: Confirmation
- Annotations: `ReadOnlyHint: false`
- Logging: Logs at INFO

**12. `set_default_printer`**
- Description: "Set the system default printer"
- Input: `printerName` (string, required)
- Output: Confirmation with previous and new default printer
- Annotations: `ReadOnlyHint: false`
- Logging: Logs old and new default at INFO

**13. `print_test_page`**
- Description: "Print a Windows test page to verify printer connectivity and configuration"
- Input: `printerName` (string, required)
- Output: Confirmation
- Annotations: `ReadOnlyHint: false`
- Logging: Logs at INFO

#### Utility Tools

**14. `get_printer_server_status`**
- Description: "Get the status of the MCP printer server itself — version, uptime, log location, configured defaults, and printer summary"
- Input: none
- Output: Server version, uptime, transport mode, log dir, default printer, allowed/blocked printers, total printers found
- Annotations: `ReadOnlyHint: true`

---

## Phase 5: Windows Service Installation

### 5.1 — `install-service.ps1`

PowerShell script to install as a Windows service using `sc.exe` or NSSM (Non-Sucking Service Manager):

```powershell
# install-service.ps1
# Installs go-mcp-printer-windows as a Windows service
#
# Usage: .\install-service.ps1 [-HttpMode] [-Port 3001] [-LogLevel info]
# Requires: Administrator privileges
#
# Strategy: Use NSSM if available (preferred), fallback to sc.exe
#
# Steps:
# 1. Check for admin privileges
# 2. Copy binary to C:\Program Files\go-mcp-printer-windows\
# 3. Create log directory at C:\ProgramData\go-mcp-printer-windows\logs\
# 4. Install service with appropriate arguments
# 5. Configure service for automatic startup
# 6. Configure service recovery (restart on failure)
# 7. Start the service
# 8. Verify service is running
```

**Service configuration:**
- Service Name: `GoMCPPrinter`
- Display Name: `Go MCP Printer Server`
- Description: `MCP server for Windows printer management`
- Startup: Automatic (Delayed Start)
- Recovery: Restart after 5 seconds (first/second/subsequent failures)
- Log on as: Local Service account
- Working directory: `C:\Program Files\go-mcp-printer-windows\`

### 5.2 — `uninstall-service.ps1`

```powershell
# uninstall-service.ps1
# Removes the go-mcp-printer-windows Windows service
#
# Steps:
# 1. Check for admin privileges
# 2. Stop service if running
# 3. Remove service registration
# 4. Optionally remove program files and logs
```

---

## Phase 6: Documentation

### 6.1 — README.md

Sections:
1. **Overview** — What it does, supported printers
2. **Features** — Tool list, transport modes, service mode
3. **Quick Start** — Download binary, configure, run
4. **Configuration** — Environment variables table, `~/.mcp_env` support
5. **Usage with Claude Desktop** — `claude_desktop_config.json` example (with `cmd /c` wrapper on Windows)
6. **Usage with Claude Code** — `.mcp.json` and `claude_code_config.json` examples
7. **Usage with Cursor** — `mcp.json` example
8. **Tool Reference** — All 14 tools with descriptions and examples
9. **Running as a Windows Service** — Service installation/management guide
10. **HTTP Mode** — API documentation, health endpoint, auth
11. **Printer-Specific Notes** — DNP dye-sub tips, HP LaserJet tips
12. **Command Line Options** — Flag reference
13. **Logging** — Log levels, locations, verbose mode, troubleshooting
14. **Security** — Allowed paths, printer filtering, auth tokens
15. **Building from Source** — Go build instructions
16. **Troubleshooting** — Common issues and solutions

### 6.2 — CLAUDE.md
- Project conventions, architecture overview
- Build command: `bash build.sh`
- Test command: `go test ./...`
- Run command: `go run . [flags]`
- Key patterns: registry, handler, PowerShell executor

---

## Phase 7: Testing

### 7.1 — Unit Tests
- `pkg/logging/logging_test.go` — Log level filtering, PII masking, file rotation
- `pkg/mcp/server_test.go` — JSON-RPC parsing, tool dispatch, HTTP endpoints
- `pkg/printer/windows_test.go` — PowerShell command construction, output parsing, error handling
- `pkg/tools/tools_test.go` — Tool registration, input validation, path security checks

### 7.2 — Integration Test Strategy (manual)
- List printers → verify all 6 printers appear
- Print test page to HP LaserJet → verify physical output
- Print 4x6 photo to DNP DS820 → verify photo output
- Print text to Microsoft Print to PDF → verify PDF created
- Queue management → submit job, check status, cancel
- HTTP mode → `curl` against health and MCP endpoints
- Service mode → install, verify running, uninstall

---

## Phase 8: Build & Release

### 8.1 — Build Script (`build.sh`)
```bash
#!/usr/bin/env bash
VERSION="${1:-dev}"
BUILD_DIR="dist"
rm -rf "$BUILD_DIR" && mkdir -p "$BUILD_DIR"

# Windows amd64
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$VERSION" \
  -o "$BUILD_DIR/go-mcp-printer-windows-amd64.exe" .

# Windows arm64
GOOS=windows GOARCH=arm64 go build -ldflags="-s -w -X main.Version=$VERSION" \
  -o "$BUILD_DIR/go-mcp-printer-windows-arm64.exe" .

echo "Build complete: $BUILD_DIR/"
ls -la "$BUILD_DIR/"
```

### 8.2 — Client Configuration Examples

**Claude Desktop** (`%APPDATA%\Claude\claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "printer": {
      "command": "C:\\Program Files\\go-mcp-printer-windows\\go-mcp-printer-windows-amd64.exe",
      "env": {
        "MCP_LOG_LEVEL": "debug",
        "MCP_PRINTER_DEFAULT": "HP ColorLaserJet MFP M282-M285 PCL-6 (V4)"
      }
    }
  }
}
```

**Claude Code** (`~/.claude/settings.json` or project `.mcp.json`):
```json
{
  "mcpServers": {
    "printer": {
      "command": "C:\\Program Files\\go-mcp-printer-windows\\go-mcp-printer-windows-amd64.exe",
      "args": ["-log-level", "debug"],
      "env": {
        "MCP_PRINTER_DEFAULT": "HP ColorLaserJet MFP M282-M285 PCL-6 (V4)"
      }
    }
  }
}
```

**HTTP Mode (any client):**
```bash
# Start server
go-mcp-printer-windows-amd64.exe --http -p 3001 -log-level debug

# Health check
curl http://127.0.0.1:3001/health

# MCP call
curl -X POST http://127.0.0.1:3001/ -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_printers"},"id":1}'
```

---

## Implementation Order

| Step | Description | Est. Files |
|------|-------------|-----------|
| 1 | Scaffolding: `go.mod`, `CLAUDE.md`, `build.sh` | 3 |
| 2 | Logging package | 2 |
| 3 | MCP protocol types | 1 |
| 4 | MCP server (stdio + HTTP) | 1 |
| 5 | Printer types | 1 |
| 6 | Windows printing backend | 1 |
| 7 | Tool registry + all 14 tools | 1 |
| 8 | `main.go` entry point | 1 |
| 9 | Service installation scripts | 2 |
| 10 | Tests | 4 |
| 11 | README + documentation | 2 |
| 12 | Test data files | 3 |
| **Total** | | **~22 files** |

---

## Security Considerations

1. **Path traversal prevention:** All file paths validated against allowed directories, `..` rejected
2. **Printer allowlist/blocklist:** Configurable which printers are accessible
3. **No command injection:** PowerShell commands constructed with proper escaping, never string interpolation of user input into commands
4. **Auth token for HTTP mode:** Optional bearer token authentication
5. **Logging PII filtering:** Sensitive data masked in logs
6. **No hidden file access:** Dotfiles and hidden directories blocked
7. **Admin operations gated:** Pause/resume/set-default clearly documented as requiring admin

---

## Open Questions / Future Enhancements

- **Spooler monitoring:** Could add a long-running watcher for print job status changes (WebSocket in HTTP mode)
- **Print preview:** Generate PDF preview before printing (would require additional tooling)
- **Scan support:** HP M283fdw has a scanner — could add scan-to-file MCP tools in a future version
- **Network printer discovery:** Auto-discover printers via mDNS/WSD beyond locally installed
- **IPP direct support:** Print via IPP protocol directly without going through Windows spooler (for advanced use cases)
