# agentctl

One small command to detect, install, update, uninstall, and manage local AI agent tools like Hermes, OpenClaw, Claude Code, Codex, Gemini CLI, and AionUi.

## Status

Early but usable. The current focus is a simple bootstrap flow:

- detect agents already installed on the machine
- install missing supported agents for the current OS
- update installed agents through their official CLI paths
- uninstall installed agents through their official CLI/package paths where available
- keep OpenClaw-specific repair and rollback logic for the gateway-heavy case

## Install agentctl

Linux/macOS, once release assets are published:

```bash
curl -fsSL https://raw.githubusercontent.com/setuplinux/agentctl/main/install.sh | bash
```

Windows PowerShell, once release assets are published:

```powershell
irm https://raw.githubusercontent.com/setuplinux/agentctl/main/install.ps1 | iex
```

## Build from source

```bash
go build -o bin/agentctl ./cmd/agentctl
./bin/agentctl status
./bin/agentctl setup
```

## Agent skill

This repo includes a reusable skill for AI agents that need to install or use `agentctl`:

```text
skills/agentctl/SKILL.md
```

If you want an agent to install or manage `agentctl`, just point it at this URL or give it the skill file:

```text
https://raw.githubusercontent.com/setuplinux/agentctl/main/skills/agentctl/SKILL.md
```

The skill tells the agent how to install `agentctl` on Linux, macOS, or Windows, then how to run safe checks, installs, updates, uninstalls, and verification.

## Commands

```bash
agentctl list
agentctl status
agentctl install <agent|all>
agentctl setup
agentctl doctor <agent|all>
agentctl update <agent|all> [--exclude agent1,agent2]
agentctl uninstall <agent|all>
agentctl version
agentctl fix openclaw
agentctl logs openclaw
agentctl rollback openclaw
```

## Current behavior

- `status` checks whether supported agents are already on `PATH` and shows version output when available.
- `install` runs the official installer path for the target agent on the current platform.
- `setup` installs only missing agents and skips anything already installed.
- `update` prefers each agent's own update command instead of replacing upstream lifecycle logic.
- `update all --exclude codex` skips selected installed agents during a broad update pass.
- `uninstall` delegates to each agent's official uninstall command or package-manager removal path where available.
- `version` prints the `agentctl` binary version. Release builds embed the git tag.
- `rollback` currently restores OpenClaw config and patched bundle files, not a full prior package version.

## Platform notes

- OpenClaw: Linux, macOS, and Windows install/update paths are cataloged; WSL2 is still the preferred Windows path.
- Claude Code: Linux, macOS, and Windows install/update paths are cataloged.
- Codex: installs through `npm install -g @openai/codex`; on Windows, if npm is missing, `agentctl install codex` first tries to install Node.js LTS with `winget`.
- Gemini CLI: installs through the official `npm install -g @google/gemini-cli` package; on Windows, if npm is missing, `agentctl install gemini` first tries to install Node.js LTS with `winget`; run `gemini` after install to authenticate.
- Hermes: Linux and macOS install/update paths are cataloged; native Windows is intentionally not auto-installed.
- AionUi: Linux install/update downloads the latest `.deb` from `iOfficeAI/AionUi` GitHub releases and installs it with `apt-get`; Linux uninstall uses `apt-get remove aionui`; launch Linux AionUi as a normal desktop user, not root. Windows install/update tries `winget install --id iOfficeAI.AionUi` first, then falls back to the latest GitHub `win-x64.exe`/`win-arm64.exe` installer with `/S` and adds `%LOCALAPPDATA%\Programs\AionUi` to the user PATH; Windows uninstall tries winget first, then the local `Uninstall AionUi.exe /S`; macOS is detect-only until app bundle install/update behavior is verified.

## Examples

```bash
agentctl version
agentctl status
agentctl install aionui
agentctl install gemini
agentctl update all
agentctl update all --exclude codex
agentctl uninstall codex
agentctl doctor openclaw
```

## Design

- Cross-platform Go binary.
- Detect first, mutate second.
- Delegate installs and updates to official agent installers or CLIs whenever possible.
- Keep family-friendly output by default.
- Avoid printing secrets.
