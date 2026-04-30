# agentctl MVP Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a small cross-platform CLI that detects local AI agent tools and provides family-friendly `list`, `status`, and future `update` commands.

**Architecture:** Go CLI with one adapter per supported agent. The MVP keeps adapters simple: detect binary on PATH, get version, and return a compact status. Updates delegate to each agent's official updater later; no custom installer logic inside the CLI.

**Tech Stack:** Go 1.22, standard library only for v0.1, GitHub Releases later, `install.sh` and `install.ps1` for simple family installs.

---

### Task 1: Repository Skeleton

**Files:**
- Create: `go.mod`
- Create: `README.md`
- Create: `install.sh`
- Create: `install.ps1`
- Create: `.gitignore`

**Steps:**
1. Create the Go module and docs/install placeholders.
2. Keep install scripts safe placeholders until release assets exist.
3. Verify `go test ./...` runs.

### Task 2: Agent Adapter Detection

**Files:**
- Test: `internal/agents/agents_test.go`
- Create: `internal/agents/agents.go`

**Behavior:**
- Given a fake executable lookup function, detect whether `hermes`, `openclaw`, `claude`, and `codex` exist.
- Return status `installed` or `missing`.

**TDD:**
1. Write failing tests for installed/missing detection.
2. Run `go test ./internal/agents` and confirm failure.
3. Implement minimal adapter registry and detection.
4. Re-run tests.

### Task 3: CLI Commands

**Files:**
- Test: `cmd/agentctl/main_test.go`
- Create: `cmd/agentctl/main.go`

**Behavior:**
- `agentctl list` prints supported agents.
- `agentctl status` prints compact status for all supported agents.
- Unknown command exits nonzero with help text.

**TDD:**
1. Write failing tests around a `Run(args, stdout, stderr)` function.
2. Run tests and confirm failure.
3. Implement minimal command dispatcher.
4. Re-run tests.

### Task 4: Verification and Packaging Notes

**Files:**
- Modify: `README.md`

**Behavior:**
- README includes Linux/macOS and Windows install commands.
- README clearly states this is local-only and delegates updates to official CLIs.
- Verify `go test ./...` passes.

---

## v0.1 Commands

```bash
agentctl list
agentctl status
agentctl update <agent>   # planned next, can initially say not implemented
agentctl doctor           # planned next
```

## Safety Defaults

- Read-only first.
- No secrets printed.
- No update implementation until status/list are solid.
- No deleting/changing existing agent configs.
