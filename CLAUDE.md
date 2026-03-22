# go-mcp-printer-windows

## Overview

Remote HTTP MCP server for Windows printer management (designed for VPN environments). Runs as a Windows service with a system tray management UI. Provides MCP tools for discovering, configuring, and managing print jobs via the Model Context Protocol.

## Architecture

Single binary with subcommands:
- `serve` — HTTP server (Windows service or foreground with tray icon)
- `install` / `uninstall` — Windows service management
- `version` — Print version

## External Dependencies (2 only)

- `golang.org/x/sys` — Windows service (svc, mgr packages)
- `fyne.io/systray` — System tray icon in foreground mode (pure Go, no CGo)

Everything else uses Go standard library (JSON-RPC, HTTP, HTML templates, embed).

## Key Directories

- `pkg/mcp/` — MCP protocol types and HTTP JSON-RPC server
- `pkg/printer/` — PowerShell printer backend
- `pkg/tools/` — 14 MCP tool registrations
- `pkg/admin/` — Admin web UI handlers
- `pkg/service/` — Windows service (svc.Handler)
- `pkg/tray/` — System tray (shown in foreground serve mode)
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
- Minimal dependencies (2 external)
- All printer operations via PowerShell
