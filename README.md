# agentctl

One small command to check, update, and manage local AI agent tools like Hermes, OpenClaw, Claude Code, and Codex.

## Status

Early MVP. Current goal: read-only local detection and status first; updates come after the basics are boring and reliable.

## Install

Linux/macOS, once releases exist:

```bash
curl -fsSL https://raw.githubusercontent.com/keithpettit/agentctl/main/install.sh | bash
```

Windows PowerShell, once releases exist:

```powershell
irm https://raw.githubusercontent.com/keithpettit/agentctl/main/install.ps1 | iex
```

## Build from source

```bash
go build -o bin/agentctl ./cmd/agentctl
./bin/agentctl status
```

## Commands

```bash
agentctl list
agentctl status
agentctl update <agent>   # planned
agentctl doctor           # planned
```

## Design

- Cross-platform Go binary.
- Family-friendly output by default.
- Delegates updates to official agent CLIs instead of replacing them.
- Redacts sensitive output; no credentials printed.
- Starts read-only: list/status before update/restart/logs.
