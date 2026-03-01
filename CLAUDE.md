# go-mcp-printer-windows

## Overview

Remote HTTPS-only MCP server for Windows printer management. Runs as a Windows service with a system tray management UI. Provides 14 MCP tools for discovering, configuring, and managing print jobs via the Model Context Protocol.

## Architecture

Single binary with subcommands:
- `serve` — HTTPS server (Windows service or foreground)
- `tray` — System tray icon (opens admin UI in browser)
- `install` / `uninstall` — Windows service management
- `version` — Print version

## External Dependencies (3 only)

- `golang.org/x/sys` — Windows service (svc, mgr packages)
- `golang.org/x/crypto` — ACME/autocert for Let's Encrypt
- `fyne.io/systray` — System tray icon (pure Go, no CGo)

Everything else uses Go standard library (JWT, JSON-RPC, HTTP, TLS, HTML templates, embed).

## Key Directories

- `pkg/mcp/` — MCP protocol types and HTTPS JSON-RPC server
- `pkg/oauth/` — OAuth 2.1 (authorization server + resource server)
- `pkg/printer/` — PowerShell printer backend
- `pkg/tools/` — 14 MCP tool registrations
- `pkg/admin/` — Admin web UI handlers
- `pkg/service/` — Windows service (svc.Handler)
- `pkg/tray/` — System tray
- `pkg/config/` — Config struct + JSON persistence
- `pkg/logging/` — Structured logger with PII filtering
- `web/` — Embedded admin UI (HTML/CSS/JS)
- `wix/` — MSI installer definition

## Config Location

`C:\ProgramData\go-mcp-printer-windows\config.json`

## Build

```bash
bash build.sh
```

## Test

```bash
go test -v -race ./...
```

## Code Style

- Go standard formatting (`go fmt`)
- No CGo (pure Go)
- Minimal dependencies (3 external)
- All printer operations via PowerShell
- JWT implementation is manual (crypto/rsa + encoding/base64)
