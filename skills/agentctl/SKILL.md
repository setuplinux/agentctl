---
name: agentctl
description: Use when detecting, installing, updating, uninstalling, or troubleshooting local AI agent CLIs managed by agentctl
---

# agentctl

## Overview

`agentctl` is a cross-platform CLI for managing local AI agent tools such as Hermes, OpenClaw, Claude Code, Codex, Gemini CLI, and Multica.

Use it as the safe first stop for agent inventory, health checks, installs, updates, and uninstalls.

## Install agentctl

If `agentctl` is missing, install it for the current platform before running management commands.

### Linux or macOS

```bash
curl -fsSL https://raw.githubusercontent.com/setuplinux/agentctl/main/install.sh | bash
```

Then refresh the shell if needed:

```bash
export PATH="$HOME/.local/bin:$HOME/bin:$PATH"
command -v agentctl
agentctl version
```

### Windows PowerShell

```powershell
irm https://raw.githubusercontent.com/setuplinux/agentctl/main/install.ps1 | iex
```

Then verify:

```powershell
agentctl.exe version
agentctl.exe status
```

If the current PowerShell session still cannot find `agentctl.exe`, open a new PowerShell window or run it from the expected user-local install path:

```powershell
$env:LOCALAPPDATA\agentctl\agentctl.exe version
```

## Safe First Commands

Always begin with read-only checks:

```bash
agentctl version
agentctl status
agentctl list
```

Use `doctor` before changing anything when it is available for the target agent:

```bash
agentctl doctor <agent>
```

Examples:

```bash
agentctl doctor openclaw
agentctl doctor hermes
```

## Common Commands

```bash
agentctl list
agentctl status
agentctl install <agent|all>
agentctl setup
agentctl doctor <agent|all>
agentctl update <agent|all> [--exclude agent1,agent2]
agentctl uninstall <agent|all>
agentctl version
agentctl help
```

Common agent names:

```text
hermes
openclaw
claude
codex
gemini
multica
```

## Installing Agents

Check current state first:

```bash
agentctl status
```

Install one missing agent:

```bash
agentctl install multica
agentctl install openclaw
agentctl install claude
agentctl install codex
agentctl install gemini
agentctl install hermes
```

Install all supported missing agents only when the user explicitly wants that broad action:

```bash
agentctl install all
```

`agentctl setup` is the friendly bootstrap path; it installs missing supported agents and skips installed ones:

```bash
agentctl setup
```

## Updates and Uninstalls

Prefer targeted updates:

```bash
agentctl update openclaw
agentctl update multica
```

Avoid broad updates unless the user asked for them and the risk is acceptable:

```bash
agentctl update all
```

If the user wants "everything except" one or two agents, use `--exclude`:

```bash
agentctl update all --exclude codex
agentctl update all --exclude codex,gemini
```

Never run destructive commands without explicit user approval:

```bash
agentctl uninstall all
agentctl remove all
```

For a single requested uninstall:

```bash
agentctl uninstall codex
```

## Verification Pattern

After installing or updating, verify with `agentctl` and the native agent command when available:

```bash
agentctl status
agentctl doctor <agent>
```

OpenClaw:

```bash
openclaw --version
openclaw gateway status --json
agentctl fix openclaw
agentctl backup openclaw
agentctl rollback openclaw
```

On native Windows, `agentctl logs openclaw` is not wired up yet because it relies on `journalctl`; prefer `agentctl doctor openclaw` there, or run the logs command from WSL/Linux.

Hermes:

```bash
hermes --version
hermes doctor
hermes gateway status
```

Multica:

```bash
multica --version
multica daemon status
```

## Platform Notes

### Windows

`install.ps1` installs `agentctl.exe` under the user profile and updates user/current-session PATH where possible.

On a clean Windows install, `agentctl.exe` itself does not require Node.js or npm. Agents installed through npm packages, such as Codex and Gemini CLI, need npm; `agentctl install codex`, `agentctl update codex`, `agentctl install gemini`, and `agentctl update gemini` try to install Node.js LTS with `winget` first when npm is missing.

Multica may install to:

```text
%LOCALAPPDATA%\multica\multica.exe
```

Do not assume PATH alone proves whether Multica is installed. `agentctl status` has richer known-path detection.

Native Windows Hermes may be unsupported or intentionally omitted. WSL is usually the expected Hermes path.

### Linux

`agentctl install multica` installs the CLI binary, not a desktop app. After install, run `multica setup` when authentication or daemon setup is needed.

### macOS

Some agents may be detect-only until unattended installer behavior is verified. Run `agentctl status` before assuming install/update support exists.

## Safety Rules

- Detect first, mutate second.
- Prefer `agentctl status` and `agentctl doctor` before install/update/uninstall.
- Do not run `uninstall all`, `remove all`, or broad update commands without explicit user approval.
- Never print API keys, auth files, `.env` files, OAuth tokens, or credential stores.
- Do not scrape one agent's private auth store for another agent.
- Keep agent-specific OAuth logins separate unless the upstream tool explicitly documents shared auth.

## Common Mistakes

- Checking only `command -v` when `agentctl status` has richer detection.
- Updating or uninstalling all agents without explicit approval.
- Printing credentials while debugging auth.
- Assuming one agent's OAuth login can safely be reused by another agent.
