# go-mcp-printer-windows

A remote HTTPS-only MCP (Model Context Protocol) server for Windows printer management. Runs as a Windows service with system tray management UI, full OAuth 2.1 authentication, and automatic Let's Encrypt certificates.

## Features

- **14 MCP Tools** for discovering, configuring, and managing print jobs
- **OAuth 2.1** with PKCE, dynamic client registration, JWT tokens
- **HTTPS-only** with ACME/Let's Encrypt or self-signed certificates
- **Windows Service** with auto-start and failure recovery
- **System Tray** icon for easy management
- **Admin Web UI** for configuration, printer status, OAuth clients, and logs
- **MSI Installer** with service, tray auto-start, and firewall rules
- **3 external dependencies** only (golang.org/x/sys, golang.org/x/crypto, fyne.io/systray)

## Quick Start

```bash
# Build
bash build.sh

# Run in foreground (self-signed cert)
./dist/go-mcp-printer-windows-amd64.exe serve

# Open admin UI
https://localhost/admin/

# Health check
curl -k https://localhost/health
```

## Commands

```
go-mcp-printer-windows.exe serve      # HTTPS server (as service or foreground)
go-mcp-printer-windows.exe tray       # System tray icon
go-mcp-printer-windows.exe install    # Install as Windows service (admin required)
go-mcp-printer-windows.exe uninstall  # Remove Windows service (admin required)
go-mcp-printer-windows.exe version    # Print version
```

## MCP Tools

| # | Tool | Type | Description |
|---|------|------|-------------|
| 1 | `list_printers` | read-only | List all installed printers |
| 2 | `get_printer_details` | read-only | Get printer capabilities and details |
| 3 | `get_default_printer` | read-only | Get the default printer name |
| 4 | `print_file` | write | Print PDF, images, text, Office docs |
| 5 | `print_text` | write | Print raw text content |
| 6 | `print_image` | write | Print images with photo-optimized settings |
| 7 | `get_print_queue` | read-only | Get print jobs in queue |
| 8 | `get_print_job_status` | read-only | Get single job status |
| 9 | `cancel_print_job` | destructive | Cancel a print job |
| 10 | `pause_printer` | write | Pause a printer |
| 11 | `resume_printer` | write | Resume a paused printer |
| 12 | `set_default_printer` | write | Set the default printer |
| 13 | `print_test_page` | write | Print a Windows test page |
| 14 | `get_printer_server_status` | read-only | Server version, uptime, config |

## OAuth 2.1 Flow

The server acts as both Authorization Server and Resource Server:

1. Client sends unauthenticated request to `/mcp` → gets `401` with `WWW-Authenticate` header
2. Client discovers OAuth endpoints via `/.well-known/oauth-protected-resource`
3. Client registers via `POST /register` → gets `client_id`
4. User authorizes via browser at `/authorize` with PKCE
5. Client exchanges code for JWT at `POST /token`
6. Client calls `POST /mcp` with `Authorization: Bearer <JWT>`

## Configuration

Config file: `C:\ProgramData\go-mcp-printer-windows\config.json`

```json
{
  "domain": "printer.example.com",
  "httpsPort": 443,
  "httpPort": 80,
  "useSelfSigned": false,
  "acmeEmail": "admin@example.com",
  "logLevel": "info",
  "defaultPrinter": "HP LaserJet",
  "allowedPrinters": [],
  "blockedPrinters": [],
  "allowedPaths": [],
  "rateLimitCalls": 10,
  "rateLimitWindow": 20
}
```

## API Routes

```
OAuth 2.1:
  GET  /.well-known/oauth-protected-resource
  GET  /.well-known/oauth-authorization-server
  GET  /authorize
  POST /token
  POST /register
  GET  /jwks
  POST /revoke

MCP (Bearer token required):
  POST /mcp

Admin (localhost or session auth):
  GET  /admin/*
  GET  /admin/api/config
  POST /admin/api/config
  GET  /admin/api/printers
  GET  /admin/api/logs
  GET  /admin/api/status
  GET  /admin/api/oauth/clients
  DELETE /admin/api/oauth/clients/:id
  POST /admin/api/oauth/keys/regenerate

Health (no auth):
  GET  /health
```

## Building

```bash
# Build for Windows
bash build.sh

# Run tests
go test -v ./...

# Build MSI (requires WiX v4)
dotnet tool install --global wix
wix build wix/Product.wxs -o dist/go-mcp-printer-windows.msi -bindpath:BuildDir=dist
```

## Installation via MSI

The MSI installer:
- Installs binary to `C:\Program Files\Go MCP Printer\`
- Creates data directory at `C:\ProgramData\go-mcp-printer-windows\`
- Installs Windows service `GoMCPPrinter` (auto-start)
- Adds system tray to Windows startup
- Configures firewall rules for ports 80 and 443

## License

MIT
