package main

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/setuplinux/agentctl/internal/agents"
	"golang.org/x/term"
)

var version = "0.2.0"

var makeTerminalRaw = func(file *os.File) (func(), error) {
	fd := int(file.Fd())
	if !term.IsTerminal(fd) {
		return func() {}, nil
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() { _ = term.Restore(fd, oldState) }, nil
}

func main() {
	os.Exit(RunWithIO(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	return RunWithIO(args, os.Stdin, stdout, stderr)
}

func RunWithIO(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "tui", "console":
		return runTUI(args[1:], stdin, stdout, stderr)
	case "bundle":
		return runBundle(args[1:], stdout, stderr)
	case "backup":
		return runBackup(args[1:], stdout, stderr)
	case "list":
		return runList(stdout)
	case "status":
		return runStatus(stdout)
	case "install":
		return runInstall(args[1:], stdout, stderr)
	case "setup":
		return runSetup(stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "update":
		return runUpdate(args[1:], stdout, stderr)
	case "uninstall", "remove":
		return runUninstall(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		return runVersion(stdout)
	case "fix":
		return runFix(args[1:], stdout, stderr)
	case "logs":
		return runLogs(args[1:], stdout, stderr)
	case "rollback":
		return runRollback(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printHelp(stderr)
		return 2
	}
}

func runList(stdout io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	fmt.Fprintf(stdout, "Supported agents (%s):\n", platform)
	for _, agent := range agents.Supported() {
		status := agents.CheckAgent(platform, agent, func(name string) (string, error) { return "", agents.ErrNotFound }, nil)
		installable := "detect-only"
		if status.SupportsInstall {
			installable = "installable"
		}
		fmt.Fprintf(stdout, "  %-8s %-12s %s\n", agent.Name, installable, agent.Description)
	}
	return 0
}

func runStatus(stdout io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	fmt.Fprintf(stdout, "Agent status (%s):\n", platform)
	for _, status := range agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput) {
		flags := make([]string, 0, 3)
		if status.SupportsInstall {
			flags = append(flags, "install")
		}
		if status.SupportsUpdate {
			flags = append(flags, "update")
		}
		if status.SupportsUninstall {
			flags = append(flags, "uninstall")
		}
		capabilityLabel := ""
		if len(flags) > 0 {
			capabilityLabel = " [" + strings.Join(flags, "/") + "]"
		}
		if status.State == "installed" {
			version := ""
			if status.Version != "" {
				version = "  " + status.Version
			}
			fmt.Fprintf(stdout, "  %-8s installed%s  %s%s\n", status.Name, capabilityLabel, status.Path, version)
			continue
		}
		fmt.Fprintf(stdout, "  %-8s missing%s\n", status.Name, capabilityLabel)
	}
	return 0
}

func runDoctor(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name == "" {
		return runStatus(stdout)
	}
	if name == "all" {
		return runDoctorAll(stdout, stderr)
	}
	if name == "openclaw" {
		return openClawDoctor(stdout, stderr)
	}
	agent, ok := agents.Find(name)
	if !ok {
		fmt.Fprintf(stderr, "unknown agent: %s\n", name)
		return 2
	}
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	support, ok := agent.Platforms[platform]
	if !ok || support.Doctor == nil {
		fmt.Fprintf(stderr, "doctor is not implemented for %q on %s\n", name, platform)
		return 2
	}
	fmt.Fprintf(stdout, "== %s doctor ==\n", titleCase(agent.Name))
	return runCommandSpec(stdout, stderr, 10*time.Minute, support.Doctor)
}

func runUpdate(args []string, stdout io.Writer, stderr io.Writer) int {
	name, exclude, err := parseUpdateArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		fmt.Fprintf(stderr, "usage: agentctl update <agent|all> [--exclude agent1,agent2]\n")
		return 2
	}
	if name == "" {
		fmt.Fprintf(stderr, "usage: agentctl update <agent|all> [--exclude agent1,agent2]\n")
		return 2
	}
	if name == "all" {
		return runUpdateAll(exclude, stdout, stderr)
	}
	if len(exclude) > 0 {
		fmt.Fprintln(stderr, "--exclude can only be used with `agentctl update all`")
		return 2
	}
	if name == "openclaw" {
		return openClawUpdate(stdout, stderr)
	}
	return runGenericAgentUpdate(name, stdout, stderr)
}

func runUninstall(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name == "" {
		fmt.Fprintf(stderr, "usage: agentctl uninstall <agent|all>\n")
		return 2
	}
	if name == "all" {
		return runUninstallAll(stdout, stderr)
	}
	return runGenericAgentUninstall(name, stdout, stderr)
}

func runVersion(stdout io.Writer) int {
	fmt.Fprintf(stdout, "agentctl %s\n", version)
	return 0
}

func runFix(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name != "openclaw" {
		fmt.Fprintf(stderr, "usage: agentctl fix openclaw\n")
		return 2
	}
	return openClawFix(stdout, stderr)
}

func runLogs(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name != "openclaw" {
		fmt.Fprintf(stderr, "usage: agentctl logs openclaw\n")
		return 2
	}
	return runOpenClawLogs(runtime.GOOS, stdout, stderr)
}

func runOpenClawLogs(goos string, stdout io.Writer, stderr io.Writer) int {
	if goos == "windows" {
		fmt.Fprintln(stderr, "openclaw logs are not wired up on native Windows yet; use `agentctl doctor openclaw` or run `agentctl logs openclaw` from WSL/Linux for journalctl-based gateway logs")
		return 2
	}
	return runLogged(stdout, stderr, 30*time.Second, "journalctl", "--user", "-u", "openclaw-gateway", "--since", "30 minutes ago", "--no-pager")
}

func runRollback(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name != "openclaw" {
		fmt.Fprintf(stderr, "usage: agentctl rollback openclaw\n")
		return 2
	}
	return openClawRollback(stdout, stderr)
}

func runBackup(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name != "openclaw" {
		fmt.Fprintf(stderr, "usage: agentctl backup openclaw\n")
		return 2
	}
	_, code := createOpenClawRollbackSnapshot(stdout, stderr)
	return code
}

func runInstall(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name == "" {
		fmt.Fprintf(stderr, "usage: agentctl install <agent|all>\n")
		return 2
	}
	if name == "all" {
		return installMissingAgents(stdout, stderr)
	}
	return installAgentByName(name, stdout, stderr)
}

func runSetup(stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stdout, "== Agent setup ==")
	return installMissingAgents(stdout, stderr)
}

func agentName(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(args[0]))
}

func parseUpdateArgs(args []string) (string, map[string]struct{}, error) {
	name := ""
	exclude := make(map[string]struct{})
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		switch {
		case strings.HasPrefix(arg, "--exclude="):
			addExcludedAgents(exclude, strings.TrimPrefix(arg, "--exclude="))
		case arg == "--exclude":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("missing value for --exclude")
			}
			i++
			addExcludedAgents(exclude, args[i])
		case strings.HasPrefix(arg, "-"):
			return "", nil, fmt.Errorf("unknown update flag: %s", arg)
		case name == "":
			name = strings.ToLower(arg)
		default:
			return "", nil, fmt.Errorf("unexpected update argument: %s", arg)
		}
	}
	return name, exclude, nil
}

func addExcludedAgents(exclude map[string]struct{}, value string) {
	for _, raw := range strings.Split(value, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		exclude[name] = struct{}{}
	}
}

type openClawRollbackSnapshot struct {
	CreatedAt    string                 `json:"createdAt"`
	Version      string                 `json:"version,omitempty"`
	ConfigBackup string                 `json:"configBackup,omitempty"`
	PatchedFiles []openClawRollbackFile `json:"patchedFiles,omitempty"`
	SnapshotDir  string                 `json:"snapshotDir,omitempty"`
}

type openClawRollbackFile struct {
	TargetPath string `json:"targetPath"`
	BackupPath string `json:"backupPath"`
}

func installMissingAgents(stdout io.Writer, stderr io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	statuses := agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput)
	code := 0
	for _, status := range statuses {
		if status.State == "installed" {
			fmt.Fprintf(stdout, "skip: %s already installed", status.Name)
			if status.Version != "" {
				fmt.Fprintf(stdout, " (%s)", status.Version)
			}
			fmt.Fprintln(stdout)
			continue
		}
		if !status.SupportsInstall {
			fmt.Fprintf(stderr, "skip: %s is not auto-installable on %s\n", status.Name, platform)
			code = 1
			continue
		}
		if installAgentByName(status.Name, stdout, stderr) != 0 {
			code = 1
		}
	}
	return code
}

func installAgentByName(name string, stdout io.Writer, stderr io.Writer) int {
	agent, ok := agents.Find(name)
	if !ok {
		fmt.Fprintf(stderr, "unknown agent: %s\n", name)
		return 2
	}
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	support, ok := agent.Platforms[platform]
	if !ok || support.Install == nil {
		fmt.Fprintf(stderr, "install is not supported for %s on %s\n", agent.Name, platform)
		return 2
	}
	status := agents.CheckAgent(platform, agent, exec.LookPath, captureCommandOutput)
	if status.State == "installed" && status.Path != "" {
		fmt.Fprintf(stdout, "skip: %s already installed at %s\n", agent.Name, status.Path)
		return 0
	}

	fmt.Fprintf(stdout, "== Install %s ==\n", titleCase(agent.Name))
	code := runCommandSpec(stdout, stderr, 30*time.Minute, support.Install)
	if code != 0 {
		return code
	}
	status = agents.CheckAgent(platform, agent, exec.LookPath, captureCommandOutput)
	if status.State == "installed" && status.Path != "" {
		fmt.Fprintf(stdout, "installed: %s -> %s\n", agent.Name, status.Path)
	}
	if support.FirstRunHint != "" {
		fmt.Fprintf(stdout, "next: %s\n", support.FirstRunHint)
	}
	return 0
}

func runUpdateAll(exclude map[string]struct{}, stdout io.Writer, stderr io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	statuses := agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput)
	code := 0
	for _, status := range statuses {
		if _, skipped := exclude[status.Name]; skipped {
			fmt.Fprintf(stdout, "skip: %s excluded\n", status.Name)
			continue
		}
		if status.State != "installed" {
			fmt.Fprintf(stdout, "skip: %s missing\n", status.Name)
			continue
		}
		if !status.SupportsUpdate {
			fmt.Fprintf(stdout, "skip: %s has no managed update path on %s\n", status.Name, platform)
			continue
		}
		if status.Name == "openclaw" {
			if openClawUpdate(stdout, stderr) != 0 {
				code = 1
			}
			continue
		}
		if runGenericAgentUpdate(status.Name, stdout, stderr) != 0 {
			code = 1
		}
	}
	return code
}

func runUninstallAll(stdout io.Writer, stderr io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	statuses := agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput)
	code := 0
	for _, status := range statuses {
		if status.State != "installed" {
			fmt.Fprintf(stdout, "skip: %s missing\n", status.Name)
			continue
		}
		if !status.SupportsUninstall {
			fmt.Fprintf(stdout, "skip: %s has no managed uninstall path on %s\n", status.Name, platform)
			continue
		}
		if runGenericAgentUninstall(status.Name, stdout, stderr) != 0 {
			code = 1
		}
	}
	return code
}

func runGenericAgentUninstall(name string, stdout io.Writer, stderr io.Writer) int {
	agent, ok := agents.Find(name)
	if !ok {
		fmt.Fprintf(stderr, "unknown agent: %s\n", name)
		return 2
	}
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	support, ok := agent.Platforms[platform]
	if !ok || support.Uninstall == nil {
		fmt.Fprintf(stderr, "uninstall is not supported for %s on %s\n", agent.Name, platform)
		return 2
	}
	status := agents.CheckAgent(platform, agent, exec.LookPath, captureCommandOutput)
	if status.State != "installed" || status.Path == "" {
		fmt.Fprintf(stdout, "skip: %s is not installed\n", agent.Name)
		return 0
	}
	fmt.Fprintf(stdout, "== Uninstall %s ==\n", titleCase(agent.Name))
	return runCommandSpec(stdout, stderr, 20*time.Minute, support.Uninstall)
}

func runGenericAgentUpdate(name string, stdout io.Writer, stderr io.Writer) int {
	agent, ok := agents.Find(name)
	if !ok {
		fmt.Fprintf(stderr, "unknown agent: %s\n", name)
		return 2
	}
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	support, ok := agent.Platforms[platform]
	if !ok || support.Update == nil {
		fmt.Fprintf(stderr, "update is not supported for %s on %s\n", agent.Name, platform)
		return 2
	}
	status := agents.CheckAgent(platform, agent, exec.LookPath, captureCommandOutput)
	if status.State != "installed" || status.Path == "" {
		fmt.Fprintf(stderr, "%s is not installed\n", agent.Name)
		return 1
	}
	fmt.Fprintf(stdout, "== Update %s ==\n", titleCase(agent.Name))
	return runCommandSpecForAgent(stdout, stderr, 20*time.Minute, support.Update, agent, status)
}

func runDoctorAll(stdout io.Writer, stderr io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	statuses := agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput)
	code := 0
	for _, status := range statuses {
		if status.State != "installed" {
			fmt.Fprintf(stdout, "skip: %s missing\n", status.Name)
			continue
		}
		if status.Name == "openclaw" {
			if openClawDoctor(stdout, stderr) != 0 {
				code = 1
			}
			continue
		}
		agent, ok := agents.Find(status.Name)
		if !ok {
			code = 1
			continue
		}
		support, ok := agent.Platforms[platform]
		if !ok || support.Doctor == nil {
			fmt.Fprintf(stdout, "skip: %s has no managed doctor path on %s\n", agent.Name, platform)
			continue
		}
		fmt.Fprintf(stdout, "== %s doctor ==\n", titleCase(agent.Name))
		if runCommandSpec(stdout, stderr, 10*time.Minute, support.Doctor) != 0 {
			code = 1
		}
	}
	return code
}

func runCommandSpec(stdout io.Writer, stderr io.Writer, timeout time.Duration, spec *agents.CommandSpec) int {
	if spec == nil {
		fmt.Fprintln(stderr, "command is not configured")
		return 2
	}
	return runLogged(stdout, stderr, timeout, spec.Program, spec.Args...)
}

func runCommandSpecForAgent(stdout io.Writer, stderr io.Writer, timeout time.Duration, spec *agents.CommandSpec, agent agents.Agent, status agents.Status) int {
	if spec == nil {
		fmt.Fprintln(stderr, "command is not configured")
		return 2
	}
	program := spec.Program
	if program == agent.Executable && status.Path != "" {
		program = status.Path
	}
	return runLogged(stdout, stderr, timeout, program, spec.Args...)
}

func displayAgentName(value string) string {
	switch strings.ToLower(value) {
	case "openclaw":
		return "OpenClaw"
	case "aionui":
		return "AionUi"
	case "codex":
		return "Codex"
	case "claude":
		return "Claude"
	case "gemini":
		return "Gemini"
	case "hermes":
		return "Hermes"
	default:
		return titleCase(value)
	}
}

func titleCase(value string) string {
	if value == "" {
		return value
	}
	if len(value) == 1 {
		return strings.ToUpper(value)
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

const windowsOpenClawServiceStatusScript = `
$ErrorActionPreference = "Continue"
Write-Host "scheduled tasks:"
schtasks /Query /FO LIST /V | Select-String -Pattern "OpenClaw|TaskName|Status|Last Run Time|Last Result" | ForEach-Object { $_.Line }
Write-Host "port 18789 listeners:"
Get-NetTCPConnection -LocalPort 18789 -ErrorAction SilentlyContinue |
  Select-Object -Property LocalAddress,LocalPort,State,OwningProcess |
  Format-Table -AutoSize | Out-String | Write-Host
Get-NetTCPConnection -LocalPort 18789 -ErrorAction SilentlyContinue |
  Select-Object -ExpandProperty OwningProcess -Unique |
  ForEach-Object { Get-CimInstance Win32_Process -Filter "ProcessId=$_" | Select-Object ProcessId,Name,CommandLine } |
  Format-List | Out-String | Write-Host
`

const windowsOpenClawRecentLogsScript = `
$ErrorActionPreference = "Continue"
$roots = @(
  (Join-Path $env:TEMP "openclaw"),
  (Join-Path $env:LOCALAPPDATA "Temp\openclaw"),
  "\tmp\openclaw"
) | Where-Object { $_ -and (Test-Path $_) } | Select-Object -Unique
foreach ($root in $roots) {
  Get-ChildItem -Path $root -Filter "*.log" -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending |
    Select-Object -First 3 |
    ForEach-Object {
      Write-Host "== $($_.FullName) =="
      Get-Content -Path $_.FullName -Tail 200 -ErrorAction SilentlyContinue |
        Select-String -Pattern "error|fail|timeout|reject|crash|stability|ciao|bonjour|probing|json5|Cannot find package|active=|queued=" |
        Select-Object -Last 120 |
        ForEach-Object { $_.Line -replace 'bot[0-9]+:[^/ ]+', 'bot[REDACTED]' }
    }
}
`

const linuxOpenClawActionableLogsScript = "journalctl --user -u openclaw-gateway --since '30 minutes ago' --no-pager | grep -Ei 'error|fail|timeout|reject|crash|stability|ciao|bonjour|probing|json5|Cannot find package|active=|queued=' | sed -E 's#bot[0-9]+:[^/ ]+#bot[REDACTED]#g' | tail -120 || true"

const windowsOpenClawStopGatewayScript = `
$ErrorActionPreference = "Continue"
schtasks /End /TN "OpenClaw Gateway" 2>$null | Out-Null
schtasks /End /TN "openclaw-gateway" 2>$null | Out-Null
Get-CimInstance Win32_Process -Filter "name = 'node.exe'" -ErrorAction SilentlyContinue |
  Where-Object { $_.CommandLine -match 'openclaw' -and $_.CommandLine -match 'gateway' } |
  ForEach-Object { Write-Host "stop openclaw gateway node pid=$($_.ProcessId)"; Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Get-NetTCPConnection -LocalPort 18789 -ErrorAction SilentlyContinue |
  Select-Object -ExpandProperty OwningProcess -Unique |
  ForEach-Object { Write-Host "stop port 18789 owner pid=$_"; Stop-Process -Id $_ -Force -ErrorAction SilentlyContinue }
Start-Sleep -Seconds 2
`

const windowsOpenClawRepairInstallScript = `
$ErrorActionPreference = "Stop"
function Add-ProcessPath([string]$Dir) {
  if (-not $Dir -or -not (Test-Path $Dir)) { return }
  $parts = @()
  if ($env:PATH) { $parts = $env:PATH -split ';' | Where-Object { $_ } }
  foreach ($part in $parts) {
    if ($part.TrimEnd('\') -ieq $Dir.TrimEnd('\')) { return }
  }
  $env:PATH = "$Dir;$env:PATH"
}
Add-ProcessPath (Join-Path $env:ProgramFiles "nodejs")
Add-ProcessPath (Join-Path $env:APPDATA "npm")
$npm = Get-Command npm.cmd -ErrorAction SilentlyContinue
if (-not $npm) { $npm = Get-Command npm -ErrorAction SilentlyContinue }
if (-not $npm) { throw "npm is required to repair OpenClaw. Install Node.js LTS, then rerun agentctl fix openclaw." }
$prefix = (& $npm.Path config get prefix).Trim()
if (-not $prefix) { $prefix = Join-Path $env:APPDATA "npm" }
$nodeModules = Join-Path $prefix "node_modules"
$openclawDir = Join-Path $nodeModules "openclaw"
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
Write-Host "npm prefix: $prefix"
if (Test-Path $openclawDir) {
  $backup = "$openclawDir.agentctl-broken-$stamp"
  Write-Host "quarantine: $openclawDir -> $backup"
  Move-Item -Path $openclawDir -Destination $backup -Force
}
Get-ChildItem -Path $nodeModules -Filter ".openclaw-*" -ErrorAction SilentlyContinue |
  ForEach-Object { Write-Host "cleanup: $($_.FullName)"; Remove-Item -LiteralPath $_.FullName -Recurse -Force -ErrorAction SilentlyContinue }
& $npm.Path install -g openclaw@latest --no-fund --no-audit --loglevel=error
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$openclaw = Get-Command openclaw.cmd -ErrorAction SilentlyContinue
if (-not $openclaw) { $openclaw = Get-Command openclaw -ErrorAction SilentlyContinue }
if (-not $openclaw) { throw "OpenClaw installed but openclaw.cmd is not visible on PATH" }
& $openclaw.Path --version
& $openclaw.Path gateway install
& $openclaw.Path gateway start
exit 0
`

func openClawDoctor(stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stdout, "== OpenClaw version ==")
	code := runLogged(stdout, stderr, 30*time.Second, "openclaw", "--version")

	fmt.Fprintln(stdout, "\n== OpenClaw update status ==")
	if c := runLogged(stdout, stderr, 60*time.Second, "openclaw", "update", "status", "--json"); c != 0 {
		code = c
	}

	fmt.Fprintln(stdout, "\n== OpenClaw gateway service ==")
	if runtime.GOOS == "windows" {
		_ = runLogged(stdout, stderr, 30*time.Second, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", windowsOpenClawServiceStatusScript)
	} else {
		_ = runLogged(stdout, stderr, 30*time.Second, "systemctl", "--user", "show", "openclaw-gateway", "-p", "MainPID", "-p", "NRestarts", "-p", "ActiveState", "-p", "SubState", "-p", "ExecMainStatus", "-p", "MemoryCurrent")
	}

	fmt.Fprintln(stdout, "\n== OpenClaw gateway RPC ==")
	if c := runLoggedEnv(stdout, stderr, 75*time.Second, []string{"OPENCLAW_RPC_TIMEOUT=30000"}, "openclaw", "gateway", "status", "--json"); c != 0 {
		code = c
	} else if !openClawGatewayRpcOk() {
		fmt.Fprintln(stderr, "gateway RPC reported ok=false")
		code = 1
	}

	fmt.Fprintln(stdout, "\n== Recent actionable gateway log lines ==")
	if runtime.GOOS == "windows" {
		_ = runLogged(stdout, stderr, 45*time.Second, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", windowsOpenClawRecentLogsScript)
	} else {
		_ = runShell(stdout, stderr, 45*time.Second, linuxOpenClawActionableLogsScript)
	}
	return code
}

func openClawUpdate(stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stdout, "== Preflight ==")
	if c := openClawDoctor(stdout, stderr); c != 0 {
		fmt.Fprintln(stderr, "preflight reported issues; continuing with official updater, then fix/verify")
	}

	available, checked, err := openClawUpdateAvailable()
	if err != nil {
		fmt.Fprintf(stderr, "update availability check failed; continuing with official updater: %v\n", err)
	} else if checked && !available {
		fmt.Fprintln(stdout, "\n== Official OpenClaw update ==")
		fmt.Fprintln(stdout, "skip: OpenClaw registry reports no update available")
		return openClawDoctor(stdout, stderr)
	}

	if runtime.GOOS == "windows" {
		fmt.Fprintln(stdout, "\n== Windows OpenClaw gateway stop ==")
		_ = stopOpenClawWindowsGateway(stdout, stderr)
	}

	snapshot, c := createOpenClawRollbackSnapshot(stdout, stderr)
	if c != 0 {
		return 1
	}

	fmt.Fprintln(stdout, "\n== Official OpenClaw update ==")
	updateCode := runLogged(stdout, stderr, 25*time.Minute, "openclaw", "update", "--yes", "--json", "--timeout", "1200")
	if updateCode != 0 {
		fmt.Fprintln(stderr, "openclaw update exited non-zero; running repair/verification path before final judgment")
	}

	fixCode := openClawFix(stdout, stderr)
	if fixCode == 0 {
		if updateCode != 0 {
			fmt.Fprintln(stdout, "official updater exited non-zero, but repair and verification succeeded")
		}
		return 0
	}
	fmt.Fprintln(stderr, "\nupdate verification failed; attempting rollback to last known pre-update state")
	rollbackCode := restoreOpenClawRollback(snapshot, stdout, stderr)
	if rollbackCode != 0 {
		return rollbackCode
	}
	return fixCode
}

func openClawUpdateAvailable() (available bool, checked bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "openclaw", "update", "status", "--json")
	out, err := cmd.Output()
	if err != nil {
		return false, false, err
	}
	return parseOpenClawUpdateAvailability(out)
}

func parseOpenClawUpdateAvailability(data []byte) (available bool, checked bool, err error) {
	var status struct {
		Availability struct {
			Available *bool `json:"available"`
		} `json:"availability"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return false, false, err
	}
	if status.Availability.Available == nil {
		return false, false, nil
	}
	return *status.Availability.Available, true, nil
}

func stopOpenClawWindowsGateway(stdout io.Writer, stderr io.Writer) int {
	return runLogged(stdout, stderr, 90*time.Second, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", windowsOpenClawStopGatewayScript)
}

func restartOpenClawGateway(stdout io.Writer, stderr io.Writer) int {
	if runtime.GOOS == "windows" {
		if c := stopOpenClawWindowsGateway(stdout, stderr); c != 0 {
			return c
		}
		return runLogged(stdout, stderr, 2*time.Minute, "openclaw", "gateway", "start")
	}
	return runLogged(stdout, stderr, 2*time.Minute, "systemctl", "--user", "restart", "openclaw-gateway")
}

func openClawFixWindows(stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stdout, "== OpenClaw Windows targeted repair ==")
	if c := stopOpenClawWindowsGateway(stdout, stderr); c != 0 {
		return c
	}
	if c := runLogged(stdout, stderr, 15*time.Minute, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", windowsOpenClawRepairInstallScript); c != 0 {
		return c
	}
	fmt.Fprintln(stdout, "repair: waiting for Windows gateway startup")
	time.Sleep(20 * time.Second)
	return openClawDoctor(stdout, stderr)
}

func openClawFix(stdout io.Writer, stderr io.Writer) int {
	if runtime.GOOS == "windows" {
		return openClawFixWindows(stdout, stderr)
	}

	fmt.Fprintln(stdout, "== OpenClaw targeted repair ==")

	// Frequent post-update failure: Telegram providers crash because the plugin
	// runtime cannot resolve package-level deps (json5/yaml) from OpenClaw's
	// generated runtime-deps tree. Patch both the installed bundle and staged
	// runtime bundle so the fix is present before Telegram providers load.
	if runtime, ok := latestOpenClawRuntimeDeps(); ok {
		fmt.Fprintf(stdout, "runtime deps: %s\n", runtime)
		if err := patchOpenClawFrontmatterImports(runtime); err != nil {
			fmt.Fprintf(stderr, "frontmatter runtime patch failed: %v\n", err)
			return 1
		}
		if !nodeCanImportFrontmatter(runtime) {
			fmt.Fprintln(stdout, "repair: frontmatter import still fails; ensuring json5 exists in runtime deps")
			if c := ensureOpenClawRuntimeJson5(runtime, stdout, stderr); c != 0 {
				return c
			}
			if err := patchOpenClawFrontmatterImports(runtime); err != nil {
				fmt.Fprintf(stderr, "frontmatter runtime patch failed after deps repair: %v\n", err)
				return 1
			}
		}
		fmt.Fprintln(stdout, "repair: staged frontmatter imports verified")
	}
	if err := patchOpenClawFrontmatterImports("/usr/lib/node_modules/openclaw"); err != nil {
		fmt.Fprintf(stderr, "frontmatter installed-bundle patch failed: %v\n", err)
		return 1
	}

	restartSince := time.Now().Add(-2 * time.Second).Format("2006-01-02 15:04:05")
	fmt.Fprintln(stdout, "repair: restarting openclaw-gateway")
	if c := runLogged(stdout, stderr, 2*time.Minute, "systemctl", "--user", "restart", "openclaw-gateway"); c != 0 {
		return c
	}
	fmt.Fprintln(stdout, "repair: waiting for startup")
	time.Sleep(75 * time.Second)

	if runtime, ok := latestOpenClawRuntimeDeps(); ok {
		if err := patchOpenClawFrontmatterImports(runtime); err != nil {
			fmt.Fprintf(stderr, "frontmatter post-start patch failed: %v\n", err)
			return 1
		}
		if !nodeCanImportFrontmatter(runtime) {
			fmt.Fprintln(stderr, "frontmatter bundle still cannot import after repair")
			return 1
		}
	}

	fmt.Fprintln(stdout, "\n== Post-repair verification ==")
	code := openClawDoctor(stdout, stderr)
	if recentGatewayLogMentions(restartSince, "Cannot find package 'json5'") || recentGatewayLogMentions(restartSince, "channel exited") {
		fmt.Fprintln(stderr, "post-restart gateway logs still show channel/import failures")
		return 1
	}
	return code
}

func backupOpenClawConfig(stdout io.Writer, stderr io.Writer) (string, int) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "cannot locate home directory: %v\n", err)
		return "", 1
	}
	src := filepath.Join(home, ".openclaw", "openclaw.json")
	if _, err := os.Stat(src); err != nil {
		fmt.Fprintf(stdout, "config backup skipped: %v\n", err)
		return "", 0
	}
	dir := filepath.Join(home, ".openclaw", "config-backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintf(stderr, "backup dir failed: %v\n", err)
		return "", 1
	}
	dst := filepath.Join(dir, "openclaw.json.bak.agentctl-"+time.Now().Format("20060102-150405"))
	data, err := os.ReadFile(src)
	if err != nil {
		fmt.Fprintf(stderr, "config backup read failed: %v\n", err)
		return "", 1
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		fmt.Fprintf(stderr, "config backup write failed: %v\n", err)
		return "", 1
	}
	fmt.Fprintf(stdout, "config backup: %s\n", dst)
	return dst, 0
}

func latestOpenClawRuntimeDeps() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	matches, err := filepath.Glob(filepath.Join(home, ".openclaw", "plugin-runtime-deps", "openclaw-*"))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	latest := matches[0]
	var latestTime time.Time
	for _, match := range matches {
		info, err := os.Stat(match)
		if err == nil && info.IsDir() && info.ModTime().After(latestTime) {
			latest = match
			latestTime = info.ModTime()
		}
	}
	return latest, true
}

func nodeCanResolve(path string, module string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "-e", "require.resolve(process.argv[1], {paths:[process.argv[2]]})", module, path)
	return cmd.Run() == nil
}

func nodeCanImportFrontmatter(runtime string) bool {
	frontmatter := filepath.Join(runtime, "dist", "frontmatter-Cc-V8aI2.js")
	if !fileExists(frontmatter) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "--input-type=module", "-e", "import(process.argv[1])", frontmatter)
	return cmd.Run() == nil
}

func openClawGatewayRpcOk() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "openclaw", "gateway", "status", "--json")
	cmd.Env = append(os.Environ(), "OPENCLAW_RPC_TIMEOUT=30000")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), `"rpc"`) && strings.Contains(string(out), `"ok": true`)
}

func patchOpenClawFrontmatterImports(root string) error {
	frontmatter := filepath.Join(root, "dist", "frontmatter-Cc-V8aI2.js")
	if !fileExists(frontmatter) {
		return nil
	}
	json5Path := "/usr/lib/node_modules/openclaw/node_modules/json5/lib/index.js"
	yamlPath := "/usr/lib/node_modules/openclaw/node_modules/yaml/dist/index.js"
	if !fileExists(json5Path) {
		return fmt.Errorf("json5 package not found at %s", json5Path)
	}
	if !fileExists(yamlPath) {
		return fmt.Errorf("yaml package not found at %s", yamlPath)
	}
	data, err := os.ReadFile(frontmatter)
	if err != nil {
		return err
	}
	contents := string(data)
	next := strings.ReplaceAll(contents, "import JSON5 from \"json5\";", "import JSON5 from \"/usr/lib/node_modules/openclaw/node_modules/json5/lib/index.js\";")
	next = strings.ReplaceAll(next, "import YAML from \"yaml\";", "import YAML from \"/usr/lib/node_modules/openclaw/node_modules/yaml/dist/index.js\";")
	if next == contents {
		return nil
	}
	backup := frontmatter + ".agentctl-bak"
	if !fileExists(backup) {
		if err := os.WriteFile(backup, data, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(frontmatter, []byte(next), 0o644)
}

func ensureOpenClawRuntimeJson5(runtime string, stdout io.Writer, stderr io.Writer) int {
	if nodeCanResolve(runtime, "json5") {
		fmt.Fprintln(stdout, "repair: json5 already resolves")
		return 0
	}

	pnpmManaged := fileExists(filepath.Join(runtime, "pnpm-lock.yaml")) || fileExists(filepath.Join(runtime, "node_modules", ".modules.yaml"))
	if pnpmManaged {
		fmt.Fprintln(stdout, "repair: runtime deps are pnpm-managed; using local pnpm store/virtual-store")
		if c := runLogged(stdout, stderr, 10*time.Minute,
			"pnpm",
			"--dir", runtime,
			"--store-dir", filepath.Join(runtime, ".openclaw-pnpm-store"),
			"--virtual-store-dir", filepath.Join(runtime, ".pnpm"),
			"add", "json5@^2.2.3"); c != 0 {
			fmt.Fprintln(stderr, "pnpm add failed; trying direct symlink fallback if package exists in pnpm store")
		}
	} else {
		if c := runLogged(stdout, stderr, 5*time.Minute, "npm", "install", "--prefix", runtime, "json5@^2.2.3", "--omit=dev"); c != 0 {
			fmt.Fprintln(stderr, "npm install failed; trying direct symlink fallback if package exists")
		}
	}

	if nodeCanResolve(runtime, "json5") {
		fmt.Fprintln(stdout, "repair: json5 resolves after package-manager install")
		return 0
	}

	if err := symlinkJson5FromPnpmStore(runtime); err != nil {
		fmt.Fprintf(stderr, "json5 fallback symlink failed: %v\n", err)
		return 1
	}
	if !nodeCanResolve(runtime, "json5") {
		fmt.Fprintln(stderr, "json5 still does not resolve after repair")
		return 1
	}
	fmt.Fprintln(stdout, "repair: json5 resolves after symlink fallback")
	return 0
}

func symlinkJson5FromPnpmStore(runtime string) error {
	matches, err := filepath.Glob(filepath.Join(runtime, ".pnpm", "json5@*", "node_modules", "json5"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no json5 package found under %s", filepath.Join(runtime, ".pnpm"))
	}
	target := matches[len(matches)-1]
	nodeModules := filepath.Join(runtime, "node_modules")
	if err := os.MkdirAll(nodeModules, 0o755); err != nil {
		return err
	}
	link := filepath.Join(nodeModules, "json5")
	if err := os.RemoveAll(link); err != nil {
		return err
	}
	rel, err := filepath.Rel(nodeModules, target)
	if err != nil {
		rel = target
	}
	return os.Symlink(rel, link)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func logMentions(stdout io.Writer, stderr io.Writer, needle string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", "journalctl --user -u openclaw-gateway --since '2 hours ago' --no-pager | grep -F -- \"$0\" >/dev/null", needle)
	return cmd.Run() == nil
}

func recentGatewayLogMentions(since string, needle string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", "journalctl --user -u openclaw-gateway --since \"$0\" --no-pager | grep -F -- \"$1\" >/dev/null", since, needle)
	return cmd.Run() == nil
}

func runLogged(stdout io.Writer, stderr io.Writer, timeout time.Duration, name string, args ...string) int {
	return runLoggedEnv(stdout, stderr, timeout, nil, name, args...)
}

func captureCommandOutput(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runLoggedEnv(stdout io.Writer, stderr io.Writer, timeout time.Duration, env []string, name string, args ...string) int {
	fmt.Fprintf(stdout, "$ %s %s\n", name, strings.Join(args, " "))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Fprintf(stderr, "command timed out after %s: %s\n", timeout, name)
			return 124
		}
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintf(stderr, "command failed: %v\n", err)
		return 1
	}
	return 0
}

func runShell(stdout io.Writer, stderr io.Writer, timeout time.Duration, script string) int {
	return runLogged(stdout, stderr, timeout, "bash", "-lc", script)
}

func createOpenClawRollbackSnapshot(stdout io.Writer, stderr io.Writer) (*openClawRollbackSnapshot, int) {
	fmt.Fprintln(stdout, "\n== Rollback snapshot ==")
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "cannot locate home directory: %v\n", err)
		return nil, 1
	}
	root := filepath.Join(home, ".openclaw", "agentctl", "rollback")
	if err := os.MkdirAll(root, 0o700); err != nil {
		fmt.Fprintf(stderr, "rollback dir failed: %v\n", err)
		return nil, 1
	}
	snapshotDir := filepath.Join(root, "openclaw-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(snapshotDir, 0o700); err != nil {
		fmt.Fprintf(stderr, "snapshot dir failed: %v\n", err)
		return nil, 1
	}

	version, _ := captureOpenClawVersion()
	if version != "" {
		fmt.Fprintf(stdout, "version snapshot: %s\n", version)
	}
	configBackup, code := backupOpenClawConfig(stdout, stderr)
	if code != 0 {
		return nil, code
	}

	snapshot := &openClawRollbackSnapshot{
		CreatedAt:    time.Now().Format(time.RFC3339),
		Version:      version,
		ConfigBackup: configBackup,
		SnapshotDir:  snapshotDir,
	}

	targets := []string{
		filepath.Join("/usr/lib/node_modules/openclaw", "dist", "frontmatter-Cc-V8aI2.js"),
	}
	if runtime, ok := latestOpenClawRuntimeDeps(); ok {
		targets = append(targets, filepath.Join(runtime, "dist", "frontmatter-Cc-V8aI2.js"))
	}
	for _, target := range targets {
		entry, err := snapshotRollbackFile(snapshotDir, target)
		if err != nil {
			fmt.Fprintf(stderr, "snapshot file failed for %s: %v\n", target, err)
			return nil, 1
		}
		if entry.TargetPath != "" {
			snapshot.PatchedFiles = append(snapshot.PatchedFiles, entry)
			fmt.Fprintf(stdout, "file snapshot: %s\n", target)
		}
	}

	if err := writeOpenClawRollbackSnapshot(snapshot); err != nil {
		fmt.Fprintf(stderr, "snapshot metadata write failed: %v\n", err)
		return nil, 1
	}
	fmt.Fprintf(stdout, "rollback snapshot: %s\n", filepath.Join(snapshotDir, "metadata.json"))
	return snapshot, 0
}

func snapshotRollbackFile(snapshotDir string, target string) (openClawRollbackFile, error) {
	if !fileExists(target) {
		return openClawRollbackFile{}, nil
	}
	name := strings.ReplaceAll(strings.TrimPrefix(target, "/"), "/", "__")
	backupPath := filepath.Join(snapshotDir, name)
	data, err := os.ReadFile(target)
	if err != nil {
		return openClawRollbackFile{}, err
	}
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return openClawRollbackFile{}, err
	}
	return openClawRollbackFile{TargetPath: target, BackupPath: backupPath}, nil
}

func writeOpenClawRollbackSnapshot(snapshot *openClawRollbackSnapshot) error {
	if snapshot == nil || snapshot.SnapshotDir == "" {
		return fmt.Errorf("rollback snapshot is incomplete")
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	metadataPath := filepath.Join(snapshot.SnapshotDir, "metadata.json")
	if err := os.WriteFile(metadataPath, data, 0o600); err != nil {
		return err
	}
	latestPath := filepath.Join(filepath.Dir(snapshot.SnapshotDir), "latest-openclaw.json")
	return os.WriteFile(latestPath, data, 0o600)
}

func loadLatestOpenClawRollbackSnapshot() (*openClawRollbackSnapshot, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	latestPath := filepath.Join(home, ".openclaw", "agentctl", "rollback", "latest-openclaw.json")
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return nil, err
	}
	var snapshot openClawRollbackSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func captureOpenClawVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "openclaw", "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func restoreOpenClawRollback(snapshot *openClawRollbackSnapshot, stdout io.Writer, stderr io.Writer) int {
	if snapshot == nil {
		fmt.Fprintln(stderr, "rollback snapshot is missing")
		return 1
	}
	fmt.Fprintln(stdout, "\n== OpenClaw rollback ==")
	if snapshot.Version != "" {
		fmt.Fprintf(stdout, "snapshot version: %s\n", snapshot.Version)
	}
	if snapshot.ConfigBackup != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(stderr, "cannot locate home directory: %v\n", err)
			return 1
		}
		target := filepath.Join(home, ".openclaw", "openclaw.json")
		if err := restoreFileFromBackup(snapshot.ConfigBackup, target, 0o600); err != nil {
			fmt.Fprintf(stderr, "config restore failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "config restored: %s\n", target)
	}
	for _, patched := range snapshot.PatchedFiles {
		if err := restoreFileFromBackup(patched.BackupPath, patched.TargetPath, 0o644); err != nil {
			fmt.Fprintf(stderr, "file restore failed for %s: %v\n", patched.TargetPath, err)
			return 1
		}
		fmt.Fprintf(stdout, "file restored: %s\n", patched.TargetPath)
	}
	if c := restartOpenClawGateway(stdout, stderr); c != 0 {
		return c
	}
	fmt.Fprintln(stdout, "rollback: waiting for startup")
	time.Sleep(20 * time.Second)
	return openClawDoctor(stdout, stderr)
}

func restoreFileFromBackup(src string, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func openClawRollback(stdout io.Writer, stderr io.Writer) int {
	snapshot, err := loadLatestOpenClawRollbackSnapshot()
	if err != nil {
		fmt.Fprintf(stderr, "could not load latest rollback snapshot: %v\n", err)
		return 1
	}
	return restoreOpenClawRollback(snapshot, stdout, stderr)
}

type tuiChoice struct {
	Label string
	Value string
}

func runTUI(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	dryRun := false
	for _, arg := range args {
		if arg == "--dry-run" || arg == "-n" {
			dryRun = true
		}
	}
	terminalControl := false
	interactiveTUI := false
	restoreTerminal := func() {}
	if file, ok := stdin.(*os.File); ok {
		inputIsTerminal := term.IsTerminal(int(file.Fd()))
		interactiveTUI = inputIsTerminal && outputIsTerminal(stdout)
		if !interactiveTUI {
			restore, err := makeTerminalRaw(file)
			if err != nil {
				fmt.Fprintf(stderr, "warning: could not enable raw terminal mode: %v\n", err)
			} else {
				restored := false
				restoreTerminal = func() {
					if !restored {
						restore()
						restored = true
					}
				}
				defer restoreTerminal()
			}
		}
		terminalControl = !interactiveTUI && inputIsTerminal && outputIsTerminal(stdout)
	}
	writeTUILine(stdout, terminalControl, "agentctl operations console")
	writeTUILine(stdout, terminalControl, "Use ↑/↓ arrows, j/k, or n/p to move; Enter to select; q to quit.")
	if dryRun {
		writeTUILine(stdout, terminalControl, "mode: dry-run (no installer, updater, fix, or rollback command will execute)")
	}
	writeTUILine(stdout, terminalControl, "")

	platform := agents.PlatformFromGOOS(runtime.GOOS)
	statuses := agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput)
	agentChoices := make([]tuiChoice, 0, len(statuses))
	for _, status := range statuses {
		label := fmt.Sprintf("%-8s %s", displayAgentName(status.Name), status.State)
		if status.Version != "" {
			label += "  " + status.Version
		}
		agentChoices = append(agentChoices, tuiChoice{Label: label, Value: status.Name})
	}

	reader := bufio.NewReader(stdin)
	agent, action, ok := selectTUIAgentActionForRuntime(reader, stdin, stdout, agentChoices, terminalControl, interactiveTUI)
	if !ok {
		writeTUILine(stdout, terminalControl, "cancelled")
		return 0
	}
	writeTUILine(stdout, terminalControl, "selected agent: %s", agent.Value)
	writeTUILine(stdout, terminalControl, "selected action: %s", action.Value)

	cmdArgs := commandArgsForTUIAction(agent.Value, action.Value)
	if len(cmdArgs) == 0 {
		fmt.Fprintf(stderr, "action %q is not available for %s\n", action.Value, agent.Value)
		return 2
	}
	writeTUILine(stdout, terminalControl, "dry-run: agentctl %s", strings.Join(cmdArgs, " "))
	if dryRun {
		return 0
	}
	if isMutationAction(action.Value) {
		if interactiveTUI {
			ok, err := confirmTUIActionBubbleTea(stdin, stdout, agent.Value, action.Value)
			if err != nil {
				fmt.Fprintf(stdout, "Bubble Tea confirmation failed (%v); falling back to basic confirmation.\n", err)
				fmt.Fprint(stdout, "Type y to continue: ")
				ok = readTUIConfirmation(reader, stdout, terminalControl)
			}
			if !ok {
				writeTUILine(stdout, terminalControl, "cancelled")
				return 0
			}
		} else {
			fmt.Fprint(stdout, "Type y to continue: ")
			if !readTUIConfirmation(reader, stdout, terminalControl) {
				writeTUILine(stdout, terminalControl, "cancelled")
				return 0
			}
		}
	}
	restoreTerminal()
	return RunWithIO(cmdArgs, stdin, stdout, stderr)
}

func outputIsTerminal(stdout io.Writer) bool {
	file, ok := stdout.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func writeTUILine(stdout io.Writer, terminalControl bool, format string, args ...any) {
	fmt.Fprintf(stdout, format, args...)
	if terminalControl {
		fmt.Fprint(stdout, "\r\n")
		return
	}
	fmt.Fprint(stdout, "\n")
}

func readTUIConfirmation(reader *bufio.Reader, stdout io.Writer, terminalControl bool) bool {
	for {
		b, err := reader.ReadByte()
		if err != nil {
			writeTUILine(stdout, terminalControl, "")
			return false
		}
		switch b {
		case 'y', 'Y':
			writeTUILine(stdout, terminalControl, "y")
			return true
		case 'n', 'N', 'q', 'Q', 0x03, 0x04, 0x1b:
			writeTUILine(stdout, terminalControl, "")
			return false
		case '\r', '\n', ' ', '\t':
			continue
		default:
			writeTUILine(stdout, terminalControl, "")
			return false
		}
	}
}

func actionsForAgent(agent string) []tuiChoice {
	actions := []tuiChoice{
		{Label: "Doctor / troubleshoot", Value: "doctor"},
		{Label: "Update", Value: "update"},
		{Label: "Install", Value: "install"},
		{Label: "Support bundle", Value: "bundle"},
	}
	if agent == "openclaw" {
		actions = append(actions,
			tuiChoice{Label: "Backup rollback snapshot", Value: "backup"},
			tuiChoice{Label: "Fix common gateway/update breakage", Value: "fix"},
			tuiChoice{Label: "Logs", Value: "logs"},
			tuiChoice{Label: "Rollback", Value: "rollback"},
		)
	}
	return actions
}

func commandArgsForTUIAction(agent string, action string) []string {
	switch action {
	case "doctor":
		return []string{"doctor", agent}
	case "update":
		return []string{"update", agent}
	case "install":
		return []string{"install", agent}
	case "bundle":
		return []string{"bundle", agent}
	case "backup", "fix", "logs", "rollback":
		if agent == "openclaw" {
			return []string{action, agent}
		}
	}
	return nil
}

func isMutationAction(action string) bool {
	switch action {
	case "install", "update", "backup", "fix", "rollback":
		return true
	default:
		return false
	}
}

func selectTUIAgentActionForRuntime(reader *bufio.Reader, stdin io.Reader, stdout io.Writer, agentChoices []tuiChoice, terminalControl bool, interactiveTUI bool) (tuiChoice, tuiChoice, bool) {
	if interactiveTUI {
		agent, action, ok, err := selectTUIAgentActionBubbleTea(stdin, stdout, agentChoices)
		if err != nil {
			fmt.Fprintf(stdout, "Bubble Tea TUI failed (%v); falling back to basic selector.\n", err)
		} else {
			return agent, action, ok
		}
	}

	agent, ok := selectTUIChoice(reader, stdout, "Select agent", agentChoices, terminalControl)
	if !ok {
		return tuiChoice{}, tuiChoice{}, false
	}
	action, ok := selectTUIChoice(reader, stdout, "Select action", actionsForAgent(agent.Value), terminalControl)
	if !ok {
		return tuiChoice{}, tuiChoice{}, false
	}
	return agent, action, true
}

func selectTUIChoiceForRuntime(reader *bufio.Reader, stdin io.Reader, stdout io.Writer, title string, choices []tuiChoice, terminalControl bool, interactiveTUI bool) (tuiChoice, bool) {
	if interactiveTUI {
		choice, ok, err := selectTUIChoiceBubbleTea(stdin, stdout, title, choices)
		if err != nil {
			fmt.Fprintf(stdout, "Bubble Tea TUI failed (%v); falling back to basic selector.\n", err)
			return selectTUIChoice(reader, stdout, title, choices, terminalControl)
		}
		return choice, ok
	}
	return selectTUIChoice(reader, stdout, title, choices, terminalControl)
}

func selectTUIChoiceBubbleTea(stdin io.Reader, stdout io.Writer, title string, choices []tuiChoice) (tuiChoice, bool, error) {
	model := newBubbleTUISelectModel(title, choices)
	options := bubbleTeaProgramOptions(stdin, stdout)
	program := tea.NewProgram(model, options...)
	finalModel, err := program.Run()
	if err != nil {
		return tuiChoice{}, false, err
	}
	final, ok := finalModel.(bubbleTUISelectModel)
	if !ok || final.cancelled || !final.done {
		return tuiChoice{}, false, nil
	}
	return final.selected, true, nil
}

func selectTUIAgentActionBubbleTea(stdin io.Reader, stdout io.Writer, agentChoices []tuiChoice) (tuiChoice, tuiChoice, bool, error) {
	model := newBubbleTUIAgentActionModel(agentChoices)
	program := tea.NewProgram(model, bubbleTeaProgramOptions(stdin, stdout)...)
	finalModel, err := program.Run()
	if err != nil {
		return tuiChoice{}, tuiChoice{}, false, err
	}
	final, ok := finalModel.(bubbleTUIAgentActionModel)
	if !ok || final.cancelled || !final.done {
		return tuiChoice{}, tuiChoice{}, false, nil
	}
	return final.agent, final.action, true, nil
}

func confirmTUIActionBubbleTea(stdin io.Reader, stdout io.Writer, agent string, action string) (bool, error) {
	model := newBubbleTUIConfirmModel(agent, action)
	program := tea.NewProgram(model, bubbleTeaProgramOptions(stdin, stdout)...)
	finalModel, err := program.Run()
	if err != nil {
		return false, err
	}
	final, ok := finalModel.(bubbleTUIConfirmModel)
	return ok && final.confirmed, nil
}

func bubbleTeaProgramOptions(stdin io.Reader, stdout io.Writer) []tea.ProgramOption {
	options := []tea.ProgramOption{tea.WithOutput(stdout)}
	if file, ok := stdin.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		// On Windows this makes Bubble Tea read from CONIN$, which avoids the
		// brittle raw-byte handling that caused CMD arrow keys to be echoed
		// instead of delivered as navigation events.
		options = append(options, tea.WithInputTTY())
	} else {
		options = append(options, tea.WithInput(stdin))
	}
	return options
}

type bubbleTUIConfirmModel struct {
	agent     string
	action    string
	confirmed bool
	cancelled bool
}

func newBubbleTUIConfirmModel(agent string, action string) bubbleTUIConfirmModel {
	return bubbleTUIConfirmModel{agent: agent, action: action}
}

func (m bubbleTUIConfirmModel) Init() tea.Cmd {
	return nil
}

func (m bubbleTUIConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEnter:
			m.confirmed = true
			return m, tea.Quit
		}
		switch msg.String() {
		case "y", "Y":
			m.confirmed = true
			return m, tea.Quit
		case "n", "N", "q", "Q":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m bubbleTUIConfirmModel) View() string {
	return fmt.Sprintf("Run agentctl %s %s?\n\nEnter/y continue • n/q cancel\n", m.action, m.agent)
}

type bubbleTUIAgentActionModel struct {
	agentChoices  []tuiChoice
	actionChoices []tuiChoice
	step          string
	cursor        int
	agent         tuiChoice
	action        tuiChoice
	done          bool
	cancelled     bool
}

func newBubbleTUIAgentActionModel(agentChoices []tuiChoice) bubbleTUIAgentActionModel {
	return bubbleTUIAgentActionModel{agentChoices: agentChoices, step: "agent"}
}

func (m bubbleTUIAgentActionModel) Init() tea.Cmd {
	return nil
}

func (m bubbleTUIAgentActionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyUp:
			m.moveUp()
			return m, nil
		case tea.KeyDown:
			m.moveDown()
			return m, nil
		case tea.KeyEnter:
			return m.selectCurrent()
		}
		switch msg.String() {
		case "q", "Q":
			m.cancelled = true
			return m, tea.Quit
		case "j", "J", "s", "S", "n", "N":
			m.moveDown()
		case "k", "K", "w", "W", "p", "P":
			m.moveUp()
		}
	}
	return m, nil
}

func (m *bubbleTUIAgentActionModel) activeChoices() []tuiChoice {
	if m.step == "action" {
		return m.actionChoices
	}
	return m.agentChoices
}

func (m *bubbleTUIAgentActionModel) moveDown() {
	choices := m.activeChoices()
	if len(choices) == 0 {
		return
	}
	m.cursor = (m.cursor + 1) % len(choices)
}

func (m *bubbleTUIAgentActionModel) moveUp() {
	choices := m.activeChoices()
	if len(choices) == 0 {
		return
	}
	if m.cursor == 0 {
		m.cursor = len(choices) - 1
		return
	}
	m.cursor--
}

func (m bubbleTUIAgentActionModel) selectCurrent() (tea.Model, tea.Cmd) {
	choices := m.activeChoices()
	if len(choices) == 0 {
		m.cancelled = true
		return m, tea.Quit
	}
	if m.step == "agent" {
		m.agent = choices[m.cursor]
		m.actionChoices = actionsForAgent(m.agent.Value)
		m.cursor = 0
		m.step = "action"
		return m, nil
	}
	m.action = choices[m.cursor]
	m.done = true
	return m, tea.Quit
}

func (m bubbleTUIAgentActionModel) View() string {
	var b strings.Builder
	if m.step == "action" {
		fmt.Fprintf(&b, "== Select action for %s ==\n", displayAgentName(m.agent.Value))
	} else {
		b.WriteString("== Select agent ==\n")
	}
	for i, choice := range m.activeChoices() {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		fmt.Fprintf(&b, "%s%s\n", prefix, choice.Label)
	}
	b.WriteString("\n↑/↓ move • Enter select • q quit\n")
	return b.String()
}

type bubbleTUISelectModel struct {
	title     string
	choices   []tuiChoice
	cursor    int
	selected  tuiChoice
	done      bool
	cancelled bool
}

func newBubbleTUISelectModel(title string, choices []tuiChoice) bubbleTUISelectModel {
	return bubbleTUISelectModel{title: title, choices: choices}
}

func (m bubbleTUISelectModel) Init() tea.Cmd {
	return nil
}

func (m bubbleTUISelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyUp:
			m.moveUp()
			return m, nil
		case tea.KeyDown:
			m.moveDown()
			return m, nil
		case tea.KeyEnter:
			if len(m.choices) == 0 {
				m.cancelled = true
				return m, tea.Quit
			}
			m.selected = m.choices[m.cursor]
			m.done = true
			return m, tea.Quit
		}
		switch msg.String() {
		case "q", "Q":
			m.cancelled = true
			return m, tea.Quit
		case "j", "J", "s", "S", "n", "N":
			m.moveDown()
		case "k", "K", "w", "W", "p", "P":
			m.moveUp()
		}
	}
	return m, nil
}

func (m *bubbleTUISelectModel) moveDown() {
	if len(m.choices) == 0 {
		return
	}
	m.cursor = (m.cursor + 1) % len(m.choices)
}

func (m *bubbleTUISelectModel) moveUp() {
	if len(m.choices) == 0 {
		return
	}
	if m.cursor == 0 {
		m.cursor = len(m.choices) - 1
		return
	}
	m.cursor--
}

func (m bubbleTUISelectModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "== %s ==\n", m.title)
	for i, choice := range m.choices {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		fmt.Fprintf(&b, "%s%s\n", prefix, choice.Label)
	}
	b.WriteString("\n↑/↓ move • Enter select • q quit\n")
	return b.String()
}

func selectTUIChoice(reader *bufio.Reader, stdout io.Writer, title string, choices []tuiChoice, terminalControl bool) (tuiChoice, bool) {
	if len(choices) == 0 {
		return tuiChoice{}, false
	}
	selected := 0
	previousMenuLines := 0
	for {
		if terminalControl && previousMenuLines > 0 {
			fmt.Fprintf(stdout, "\x1b[%dF\x1b[J", previousMenuLines)
		}
		writeTUILine(stdout, terminalControl, "== %s ==", title)
		for i, choice := range choices {
			prefix := "  "
			if i == selected {
				prefix = "> "
			}
			writeTUILine(stdout, terminalControl, "%s%s", prefix, choice.Label)
		}
		previousMenuLines = len(choices) + 1
		key, ok := readTUIKey(reader)
		if !ok {
			return choices[selected], true
		}
		switch key {
		case "up":
			if selected == 0 {
				selected = len(choices) - 1
			} else {
				selected--
			}
		case "down":
			selected = (selected + 1) % len(choices)
		case "enter":
			return choices[selected], true
		case "quit":
			return tuiChoice{}, false
		}
	}
}

func readTUIKey(reader *bufio.Reader) (string, bool) {
	b, err := reader.ReadByte()
	if err != nil {
		return "enter", false
	}
	switch b {
	case 0x03, 0x04:
		return "quit", true
	case 'q', 'Q':
		return "quit", true
	case 'j', 'J', 's', 'S', 'n', 'N':
		return "down", true
	case 'k', 'K', 'w', 'W', 'p', 'P':
		return "up", true
	case '\r', '\n':
		return "enter", true
	case 0x00, 0xe0:
		second, err := reader.ReadByte()
		if err != nil {
			return "", true
		}
		switch second {
		case 0x48:
			return "up", true
		case 0x50:
			return "down", true
		}
	case 0x1b:
		second, err := reader.ReadByte()
		if err != nil || second != '[' {
			return "", true
		}
		third, err := reader.ReadByte()
		if err != nil {
			return "", true
		}
		if third == 'A' {
			return "up", true
		}
		if third == 'B' {
			return "down", true
		}
	}
	return "", true
}

func runBundle(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name == "" {
		fmt.Fprintf(stderr, "usage: agentctl bundle <agent|all>\n")
		return 2
	}
	agentsForBundle := []string{name}
	if name == "all" {
		agentsForBundle = nil
		for _, agent := range agents.Supported() {
			agentsForBundle = append(agentsForBundle, agent.Name)
		}
	} else if _, ok := agents.Find(name); !ok {
		fmt.Fprintf(stderr, "unknown agent: %s\n", name)
		return 2
	}
	path, err := createSupportBundle(agentsForBundle)
	if err != nil {
		fmt.Fprintf(stderr, "support bundle failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "support bundle: %s\n", path)
	return 0
}

func createSupportBundle(agentNames []string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, ".agentctl", "bundles")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	bundlePath := filepath.Join(root, "agentctl-bundle-"+time.Now().Format("20060102-150405")+".zip")
	file, err := os.Create(bundlePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	zw := zip.NewWriter(file)
	defer zw.Close()

	manifest := map[string]any{
		"created_at": time.Now().Format(time.RFC3339),
		"agentctl":   version,
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"agents":     agentNames,
	}
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
	if err := addZipFile(zw, "manifest.json", string(manifestJSON)+"\n"); err != nil {
		return "", err
	}
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	var status strings.Builder
	status.WriteString("Agent status:\n")
	for _, s := range agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput) {
		if !containsString(agentNames, s.Name) {
			continue
		}
		status.WriteString(fmt.Sprintf("- %s: %s %s %s\n", s.Name, s.State, s.Path, s.Version))
	}
	if err := addZipFile(zw, "status.txt", redactSensitiveText(status.String())); err != nil {
		return "", err
	}
	if err := addZipFile(zw, "README.txt", "This agentctl support bundle is redacted. It excludes raw auth files, .env files, browser sessions, and callback URLs.\n"); err != nil {
		return "", err
	}
	return bundlePath, nil
}

func addZipFile(zw *zip.Writer, name string, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(content))
	return err
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

var redactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|refresh[_-]?token|access[_-]?token|id[_-]?token|authorization)(\s*[=:]\s*)[^\s]+`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]+`),
	regexp.MustCompile(`bot[0-9]+:[^\s/]+`),
	regexp.MustCompile(`https?://localhost:1455/[^\s]+`),
	regexp.MustCompile(`https?://127\.0\.0\.1:1455/[^\s]+`),
}

func redactSensitiveText(input string) string {
	out := input
	for _, pattern := range redactionPatterns {
		out = pattern.ReplaceAllString(out, "[REDACTED]")
	}
	return out
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "agentctl - manage local AI agent tools")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  agentctl tui")
	fmt.Fprintln(w, "  agentctl list")
	fmt.Fprintln(w, "  agentctl status")
	fmt.Fprintln(w, "  agentctl install <agent|all>")
	fmt.Fprintln(w, "  agentctl setup")
	fmt.Fprintln(w, "  agentctl doctor <agent|all>")
	fmt.Fprintln(w, "  agentctl bundle <agent|all>")
	fmt.Fprintln(w, "  agentctl backup openclaw")
	fmt.Fprintln(w, "  agentctl update <agent|all> [--exclude agent1,agent2]")
	fmt.Fprintln(w, "  agentctl uninstall <agent|all>")
	fmt.Fprintln(w, "  agentctl version")
	fmt.Fprintln(w, "  agentctl fix openclaw")
	fmt.Fprintln(w, "  agentctl logs openclaw")
	fmt.Fprintln(w, "  agentctl rollback openclaw")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  agentctl tui --dry-run")
	fmt.Fprintln(w, "  agentctl status")
	fmt.Fprintln(w, "  agentctl install aionui")
	fmt.Fprintln(w, "  agentctl update all")
	fmt.Fprintln(w, "  agentctl update all --exclude codex")
	fmt.Fprintln(w, "  agentctl backup openclaw")
	fmt.Fprintln(w, "  agentctl uninstall codex")
	fmt.Fprintln(w, "  agentctl doctor openclaw")
}
