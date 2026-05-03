# agentctl

One small command to detect, install, update, and manage local AI agent tools like Hermes, OpenClaw, Claude Code, Codex, and AionUi.

## Status

Early but usable. The current focus is a simple bootstrap flow:

- detect agents already installed on the machine
- install missing supported agents for the current OS
- update installed agents through their official CLI paths
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

## Commands

```bash
agentctl list
agentctl status
agentctl install <agent|all>
agentctl setup
agentctl doctor <agent|all>
agentctl update <agent|all>
agentctl fix openclaw
agentctl logs openclaw
agentctl rollback openclaw
```

## Current behavior

- `status` checks whether supported agents are already on `PATH` and shows version output when available.
- `install` runs the official installer path for the target agent on the current platform.
- `setup` installs only missing agents and skips anything already installed.
- `update` prefers each agent's own update command instead of replacing upstream lifecycle logic.
- `rollback` currently restores OpenClaw config and patched bundle files, not a full prior package version.

## Platform notes

- OpenClaw: Linux, macOS, and Windows install/update paths are cataloged; WSL2 is still the preferred Windows path.
- Claude Code: Linux, macOS, and Windows install/update paths are cataloged.
- Codex: installs through `npm install -g @openai/codex`; Windows support is still best-effort and often smoother in WSL.
- Hermes: Linux and macOS install/update paths are cataloged; native Windows is intentionally not auto-installed.
- AionUi: Linux install/update downloads the latest `.deb` from `iOfficeAI/AionUi` GitHub releases and installs it with `apt-get`; Windows install/update uses `winget install/upgrade --id iOfficeAI.AionUi`; macOS is detect-only until app bundle install/update behavior is verified.

## Design

- Cross-platform Go binary.
- Detect first, mutate second.
- Delegate installs and updates to official agent installers or CLIs whenever possible.
- Keep family-friendly output by default.
- Avoid printing secrets.
