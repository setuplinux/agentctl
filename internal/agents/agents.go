package agents

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrNotFound = errors.New("agent executable not found")

const npmUninstallCodexScript = `npm uninstall -g @openai/codex`

const claudeUnixUninstallScript = `
set -euo pipefail
if command -v claude >/dev/null 2>&1; then
  claude uninstall || true
fi
if command -v npm >/dev/null 2>&1; then
  npm uninstall -g @anthropic-ai/claude-code || true
fi
rm -f "$HOME/.local/bin/claude" "$HOME/.local/bin/claude.exe" "$HOME/.local/bin/claude.cmd" "$HOME/.local/bin/claude.ps1"
`

const claudeWindowsUninstallScript = `
$ErrorActionPreference = "Continue"
$claude = Get-Command claude -ErrorAction SilentlyContinue
if ($claude) { claude uninstall }
$npm = Get-Command npm -ErrorAction SilentlyContinue
if ($npm) { npm uninstall -g @anthropic-ai/claude-code }
$localBin = Join-Path $env:USERPROFILE ".local\bin"
@("claude.exe", "claude.cmd", "claude.ps1", "claude") | ForEach-Object {
  $path = Join-Path $localBin $_
  if (Test-Path $path) { Remove-Item -Force $path }
}
exit 0
`

const openClawUninstallScript = `
set -euo pipefail
if command -v openclaw >/dev/null 2>&1; then
  openclaw uninstall --service --yes --non-interactive || true
fi
if command -v npm >/dev/null 2>&1; then
  npm uninstall -g openclaw || true
fi
`

const openClawWindowsUninstallScript = `
$ErrorActionPreference = "Continue"
$openclaw = Get-Command openclaw -ErrorAction SilentlyContinue
if ($openclaw) { openclaw uninstall --service --yes --non-interactive }
$npm = Get-Command npm -ErrorAction SilentlyContinue
if ($npm) { npm uninstall -g openclaw }
exit 0
`

const multicaUnixInstallScript = `
set -euo pipefail
need() { command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }; }
need curl
need python3
need tar
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux|darwin) ;;
  *) echo "unsupported multica OS: $os" >&2; exit 2 ;;
esac
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) asset_arch="amd64" ;;
  aarch64|arm64) asset_arch="arm64" ;;
  *) echo "unsupported multica architecture: $arch" >&2; exit 2 ;;
esac
asset_url="$({ python3 - "$os" "$asset_arch" <<'PY'
import json, sys, urllib.request
os_name, asset_arch = sys.argv[1], sys.argv[2]
req = urllib.request.Request(
    'https://api.github.com/repos/multica-ai/multica/releases/latest',
    headers={'Accept':'application/vnd.github+json','User-Agent':'agentctl'}
)
data = json.load(urllib.request.urlopen(req, timeout=30))
suffix = f'multica_{os_name}_{asset_arch}.tar.gz'
for asset in data.get('assets', []):
    if asset.get('name', '') == suffix:
        print(asset['browser_download_url'])
        break
else:
    raise SystemExit(f'no multica release asset named {suffix}')
PY
} )"
tmp_parent="${TMPDIR:-/var/tmp}"
if [ ! -d "$tmp_parent" ] || [ ! -w "$tmp_parent" ]; then
  tmp_parent="/tmp"
fi
tmpdir="$(mktemp -d -p "$tmp_parent" agentctl-multica.XXXXXX)"
trap 'rm -rf "$tmpdir"' EXIT
archive="$tmpdir/multica.tar.gz"
echo "download: $asset_url"
curl -fL "$asset_url" -o "$archive"
tar -xzf "$archive" -C "$tmpdir"
binary="$(find "$tmpdir" -type f -name multica -perm /111 | head -n 1)"
if [ -z "$binary" ]; then
  echo "multica binary not found in release archive" >&2
  exit 1
fi
install_dir="${HOME:-}/.local/bin"
if [ "$(id -u)" -eq 0 ] && [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
  install_dir="/usr/local/bin"
fi
mkdir -p "$install_dir"
install -m 0755 "$binary" "$install_dir/multica"
echo "installed: $install_dir/multica"
`

const multicaWindowsInstallScript = `
$ErrorActionPreference = "Stop"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$release = Invoke-RestMethod -Headers @{"User-Agent"="agentctl"} -Uri "https://api.github.com/repos/multica-ai/multica/releases/latest"
$name = "multica_windows_$arch.zip"
$asset = $release.assets | Where-Object { $_.name -eq $name } | Select-Object -First 1
if (-not $asset) { throw "No multica release asset named $name" }
$installDir = Join-Path $env:LOCALAPPDATA "multica"
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$archive = Join-Path $env:TEMP $asset.name
Write-Host "download: $($asset.browser_download_url)"
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $archive
Expand-Archive -Path $archive -DestinationPath $installDir -Force
$candidates = Get-ChildItem -Path $installDir -Recurse -Filter "multica.exe" | Select-Object -First 1
if (-not $candidates) { throw "multica.exe not found in release archive" }
Copy-Item -Force $candidates.FullName (Join-Path $installDir "multica.exe")
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$parts = @()
if ($userPath) { $parts = $userPath -split ';' | Where-Object { $_ } }
$alreadyInPath = $false
foreach ($part in $parts) {
  if ($part.TrimEnd('\\') -ieq $installDir.TrimEnd('\\')) { $alreadyInPath = $true; break }
}
if (-not $alreadyInPath) {
  $newUserPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
  [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
  Write-Host "path: added $installDir to user PATH. Open a new terminal to use multica.exe directly."
}
if (($env:PATH -split ';') -notcontains $installDir) {
  $env:PATH = "$installDir;$env:PATH"
}
Write-Host "installed: $(Join-Path $installDir 'multica.exe')"
exit 0
`

const windowsNpmInstallScriptTemplate = `
$ErrorActionPreference = "Stop"
$package = "__PACKAGE__"

function Add-ProcessPath {
  param([string]$Dir)
  if (-not $Dir) { return }
  $expanded = [Environment]::ExpandEnvironmentVariables($Dir)
  if (-not (Test-Path $expanded)) { return }
  $parts = @()
  if ($env:PATH) { $parts = $env:PATH -split ';' | Where-Object { $_ } }
  foreach ($part in $parts) {
    if ($part.TrimEnd('\') -ieq $expanded.TrimEnd('\')) { return }
  }
  $env:PATH = "$expanded;$env:PATH"
}

Add-ProcessPath (Join-Path $env:ProgramFiles "nodejs")
Add-ProcessPath (Join-Path $env:APPDATA "npm")
$npm = Get-Command npm.cmd -ErrorAction SilentlyContinue
if (-not $npm) { $npm = Get-Command npm -ErrorAction SilentlyContinue }
if (-not $npm) {
  $winget = Get-Command winget -ErrorAction SilentlyContinue
  if (-not $winget) {
    throw "npm is required to install $package. Install Node.js LTS from https://nodejs.org/ or install winget, then rerun agentctl."
  }
  Write-Host "npm was not found; installing Node.js LTS with winget..."
  winget install --id OpenJS.NodeJS.LTS --exact --source winget --accept-package-agreements --accept-source-agreements
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  Add-ProcessPath (Join-Path $env:ProgramFiles "nodejs")
  Add-ProcessPath (Join-Path $env:APPDATA "npm")
  $npm = Get-Command npm.cmd -ErrorAction SilentlyContinue
  if (-not $npm) { $npm = Get-Command npm -ErrorAction SilentlyContinue }
}
if (-not $npm) {
  throw "Node.js was installed, but npm is not visible in this PowerShell session. Open a new terminal and rerun agentctl."
}
& $npm.Path install -g $package
exit $LASTEXITCODE
`

type Platform string

const (
	PlatformLinux   Platform = "linux"
	PlatformWindows Platform = "windows"
	PlatformDarwin  Platform = "darwin"
	PlatformUnknown Platform = "unknown"
)

type CommandSpec struct {
	Program string
	Args    []string
}

type PlatformSupport struct {
	Install      *CommandSpec
	Update       *CommandSpec
	Uninstall    *CommandSpec
	Doctor       *CommandSpec
	FirstRunHint string
	Notes        []string
}

type Agent struct {
	Name        string
	Executable  string
	Description string
	VersionArgs []string
	Platforms   map[Platform]PlatformSupport
}

type Status struct {
	Name              string
	State             string
	Path              string
	Version           string
	SupportsInstall   bool
	SupportsUpdate    bool
	SupportsUninstall bool
	SupportsDoctor    bool
	FirstRunHint      string
	Notes             []string
}

type LookupFunc func(name string) (string, error)
type RunnerFunc func(name string, args ...string) (string, error)

func PlatformFromGOOS(goos string) Platform {
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case string(PlatformLinux):
		return PlatformLinux
	case string(PlatformWindows):
		return PlatformWindows
	case string(PlatformDarwin):
		return PlatformDarwin
	default:
		return PlatformUnknown
	}
}

func npmInstallSpec(packageName string) *CommandSpec {
	return &CommandSpec{
		Program: "npm",
		Args:    []string{"install", "-g", packageName},
	}
}

func npmUninstallSpec(packageName string) *CommandSpec {
	return &CommandSpec{
		Program: "npm",
		Args:    []string{"uninstall", "-g", packageName},
	}
}

func windowsNpmInstallSpec(packageName string) *CommandSpec {
	return &CommandSpec{
		Program: "powershell",
		Args: []string{
			"-NoProfile",
			"-ExecutionPolicy",
			"Bypass",
			"-Command",
			strings.ReplaceAll(windowsNpmInstallScriptTemplate, "__PACKAGE__", packageName),
		},
	}
}

func Supported() []Agent {
	return []Agent{
		{
			Name:        "hermes",
			Executable:  "hermes",
			Description: "Hermes Agent",
			VersionArgs: []string{"--version"},
			Platforms: map[Platform]PlatformSupport{
				PlatformLinux: {
					Install: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", "curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash"},
					},
					Update: &CommandSpec{
						Program: "hermes",
						Args:    []string{"update"},
					},
					Doctor: &CommandSpec{
						Program: "hermes",
						Args:    []string{"doctor"},
					},
					FirstRunHint: "Run `hermes setup`, then `hermes doctor`.",
					Notes: []string{
						"Native Windows is not supported by the Hermes installer; use WSL2 there.",
					},
				},
				PlatformDarwin: {
					Install: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", "curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash"},
					},
					Update: &CommandSpec{
						Program: "hermes",
						Args:    []string{"update"},
					},
					Doctor: &CommandSpec{
						Program: "hermes",
						Args:    []string{"doctor"},
					},
					FirstRunHint: "Run `hermes setup`, then `hermes doctor`.",
				},
			},
		},
		{
			Name:        "openclaw",
			Executable:  "openclaw",
			Description: "OpenClaw",
			VersionArgs: []string{"--version"},
			Platforms: map[Platform]PlatformSupport{
				PlatformLinux: {
					Install: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", "curl -fsSL https://openclaw.ai/install.sh | bash -s -- --no-onboard"},
					},
					Update: &CommandSpec{
						Program: "openclaw",
						Args:    []string{"update", "--yes", "--json", "--timeout", "1200"},
					},
					Uninstall: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", openClawUninstallScript},
					},
					Doctor: &CommandSpec{
						Program: "openclaw",
						Args:    []string{"doctor"},
					},
					FirstRunHint: "Run `openclaw onboard --install-daemon`, then `openclaw gateway status`.",
				},
				PlatformDarwin: {
					Install: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", "curl -fsSL https://openclaw.ai/install.sh | bash -s -- --no-onboard"},
					},
					Update: &CommandSpec{
						Program: "openclaw",
						Args:    []string{"update", "--yes", "--json", "--timeout", "1200"},
					},
					Uninstall: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", openClawUninstallScript},
					},
					Doctor: &CommandSpec{
						Program: "openclaw",
						Args:    []string{"doctor"},
					},
					FirstRunHint: "Run `openclaw onboard --install-daemon`, then `openclaw gateway status`.",
				},
				PlatformWindows: {
					Install: &CommandSpec{
						Program: "powershell",
						Args:    []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "& ([scriptblock]::Create((iwr -useb https://openclaw.ai/install.ps1))) -NoOnboard"},
					},
					Update: &CommandSpec{
						Program: "openclaw",
						Args:    []string{"update", "--yes", "--json", "--timeout", "1200"},
					},
					Uninstall: &CommandSpec{
						Program: "powershell",
						Args:    []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", openClawWindowsUninstallScript},
					},
					Doctor: &CommandSpec{
						Program: "openclaw",
						Args:    []string{"doctor"},
					},
					FirstRunHint: "Run `openclaw onboard --install-daemon` or `openclaw gateway install` after install.",
					Notes: []string{
						"WSL2 is the more stable OpenClaw path on Windows.",
					},
				},
			},
		},
		{
			Name:        "claude",
			Executable:  "claude",
			Description: "Claude Code",
			VersionArgs: []string{"--version"},
			Platforms: map[Platform]PlatformSupport{
				PlatformLinux: {
					Install: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", "curl -fsSL https://claude.ai/install.sh | bash"},
					},
					Update: &CommandSpec{
						Program: "claude",
						Args:    []string{"update"},
					},
					Uninstall: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", claudeUnixUninstallScript},
					},
					Doctor: &CommandSpec{
						Program: "claude",
						Args:    []string{"doctor"},
					},
					FirstRunHint: "Run `claude`, sign in, then `claude doctor`.",
				},
				PlatformDarwin: {
					Install: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", "curl -fsSL https://claude.ai/install.sh | bash"},
					},
					Update: &CommandSpec{
						Program: "claude",
						Args:    []string{"update"},
					},
					Uninstall: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", claudeUnixUninstallScript},
					},
					Doctor: &CommandSpec{
						Program: "claude",
						Args:    []string{"doctor"},
					},
					FirstRunHint: "Run `claude`, sign in, then `claude doctor`.",
				},
				PlatformWindows: {
					Install: &CommandSpec{
						Program: "powershell",
						Args:    []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "irm https://claude.ai/install.ps1 | iex"},
					},
					Update: &CommandSpec{
						Program: "claude",
						Args:    []string{"update"},
					},
					Uninstall: &CommandSpec{
						Program: "powershell",
						Args:    []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", claudeWindowsUninstallScript},
					},
					Doctor: &CommandSpec{
						Program: "claude",
						Args:    []string{"doctor"},
					},
					FirstRunHint: "Run `claude`, sign in, then `claude doctor`.",
					Notes: []string{
						"Git for Windows is recommended on native Windows; WSL2 is preferred when you need sandboxed Linux toolchains.",
					},
				},
			},
		},
		{
			Name:        "codex",
			Executable:  "codex",
			Description: "OpenAI Codex",
			VersionArgs: []string{"--version"},
			Platforms: map[Platform]PlatformSupport{
				PlatformLinux: {
					Install:      npmInstallSpec("@openai/codex"),
					Update:       npmInstallSpec("@openai/codex"),
					Uninstall:    npmUninstallSpec("@openai/codex"),
					FirstRunHint: "Run `codex --login` after install.",
					Notes: []string{
						"Official Codex CLI supports Linux and macOS. Windows support is still experimental and often works best in WSL.",
					},
				},
				PlatformDarwin: {
					Install:      npmInstallSpec("@openai/codex"),
					Update:       npmInstallSpec("@openai/codex"),
					Uninstall:    npmUninstallSpec("@openai/codex"),
					FirstRunHint: "Run `codex --login` after install.",
				},
				PlatformWindows: {
					Install:      windowsNpmInstallSpec("@openai/codex"),
					Update:       windowsNpmInstallSpec("@openai/codex"),
					Uninstall:    npmUninstallSpec("@openai/codex"),
					FirstRunHint: "Run `codex --login` after install.",
					Notes: []string{
						"Official Windows support is still experimental; WSL is often the safer route for coding workflows.",
						"If npm is missing, agentctl install codex tries to install Node.js LTS with winget first.",
					},
				},
			},
		},
		{
			Name:        "gemini",
			Executable:  "gemini",
			Description: "Google Gemini CLI",
			VersionArgs: []string{"--version"},
			Platforms: map[Platform]PlatformSupport{
				PlatformLinux: {
					Install:      npmInstallSpec("@google/gemini-cli"),
					Update:       npmInstallSpec("@google/gemini-cli"),
					Uninstall:    npmUninstallSpec("@google/gemini-cli"),
					FirstRunHint: "Run `gemini`, then choose a Google authentication method.",
					Notes: []string{
						"Official Gemini CLI standard installation uses npm package @google/gemini-cli.",
					},
				},
				PlatformDarwin: {
					Install:      npmInstallSpec("@google/gemini-cli"),
					Update:       npmInstallSpec("@google/gemini-cli"),
					Uninstall:    npmUninstallSpec("@google/gemini-cli"),
					FirstRunHint: "Run `gemini`, then choose a Google authentication method.",
				},
				PlatformWindows: {
					Install:      windowsNpmInstallSpec("@google/gemini-cli"),
					Update:       windowsNpmInstallSpec("@google/gemini-cli"),
					Uninstall:    npmUninstallSpec("@google/gemini-cli"),
					FirstRunHint: "Run `gemini`, then choose a Google authentication method.",
					Notes: []string{
						"On Windows, npm usually places the gemini shim under %APPDATA%\\npm.",
						"If npm is missing, agentctl install gemini tries to install Node.js LTS with winget first.",
					},
				},
			},
		},
		{
			Name:        "multica",
			Executable:  "multica",
			Description: "Multica CLI and local agent runtime",
			VersionArgs: []string{"--version"},
			Platforms: map[Platform]PlatformSupport{
				PlatformLinux: {
					Install: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", multicaUnixInstallScript},
					},
					Update: &CommandSpec{
						Program: "multica",
						Args:    []string{"update"},
					},
					FirstRunHint: "Run `multica setup` to authenticate and start the local daemon.",
					Notes: []string{
						"Linux install downloads the latest multica CLI archive from multica-ai/multica GitHub releases.",
					},
				},
				PlatformDarwin: {
					Install: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", multicaUnixInstallScript},
					},
					Update: &CommandSpec{
						Program: "multica",
						Args:    []string{"update"},
					},
					FirstRunHint: "Run `multica setup` to authenticate and start the local daemon.",
				},
				PlatformWindows: {
					Install: &CommandSpec{
						Program: "powershell",
						Args:    []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", multicaWindowsInstallScript},
					},
					Update: &CommandSpec{
						Program: "multica",
						Args:    []string{"update"},
					},
					FirstRunHint: "Run `multica setup` to authenticate and start the local daemon.",
					Notes: []string{
						"Windows install downloads the latest multica CLI zip from multica-ai/multica GitHub releases and adds %LOCALAPPDATA%\\multica to the user PATH.",
					},
				},
			},
		},
	}
}

func Find(name string) (Agent, bool) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	for _, agent := range Supported() {
		if agent.Name == normalized {
			return agent, true
		}
	}
	return Agent{}, false
}

func CheckAll(lookup LookupFunc) []Status {
	return CheckAllForPlatform(PlatformUnknown, lookup, defaultRunner)
}

func CheckAllForPlatform(platform Platform, lookup LookupFunc, runner RunnerFunc) []Status {
	agentList := Supported()
	statuses := make([]Status, 0, len(agentList))
	for _, agent := range agentList {
		statuses = append(statuses, CheckAgent(platform, agent, lookup, runner))
	}
	return statuses
}

func CheckAgent(platform Platform, agent Agent, lookup LookupFunc, runner RunnerFunc) Status {
	path, err := resolveExecutablePath(platform, agent, lookup)
	status := Status{Name: agent.Name, State: "installed", Path: path}
	if err != nil || path == "" {
		status.State = "missing"
		status.Path = ""
	}
	if runner == nil {
		runner = defaultRunner
	}
	if status.State == "installed" && len(agent.VersionArgs) > 0 {
		version, err := runner(agent.Executable, agent.VersionArgs...)
		if err != nil && status.Path != "" && status.Path != agent.Executable {
			version, err = runner(status.Path, agent.VersionArgs...)
		}
		if err == nil {
			status.Version = firstMeaningfulLine(version)
		}
	}
	if support, ok := agent.Platforms[platform]; ok {
		status.SupportsInstall = support.Install != nil
		status.SupportsUpdate = support.Update != nil
		status.SupportsUninstall = support.Uninstall != nil
		status.SupportsDoctor = support.Doctor != nil
		status.FirstRunHint = support.FirstRunHint
		status.Notes = append(status.Notes, support.Notes...)
	}
	return status
}

func resolveExecutablePath(platform Platform, agent Agent, lookup LookupFunc) (string, error) {
	if lookup == nil {
		return "", ErrNotFound
	}
	path, err := lookup(agent.Executable)
	if err == nil && path != "" {
		return path, nil
	}
	if platform != PlatformWindows {
		for _, candidate := range unixExecutableCandidates(agent) {
			if fileExists(candidate) {
				return candidate, nil
			}
		}
		return path, err
	}
	for _, candidate := range windowsExecutableCandidates(agent) {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return path, err
}

func defaultRunner(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stdout.Len() > 0 {
			return stdout.String(), err
		}
		if stderr.Len() > 0 {
			return stderr.String(), err
		}
		return "", err
	}
	return stdout.String(), nil
}

func unixExecutableCandidates(agent Agent) []string {
	candidates := make([]string, 0, 4)
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", agent.Executable))
	}
	return candidates
}

func windowsExecutableCandidates(agent Agent) []string {
	appData := strings.TrimSpace(os.Getenv("APPDATA"))
	localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	programFiles := strings.TrimSpace(os.Getenv("ProgramFiles"))
	userProfile := strings.TrimSpace(os.Getenv("USERPROFILE"))
	candidates := make([]string, 0, 16)

	if appData != "" {
		npmDir := filepath.Join(appData, "npm")
		candidates = append(candidates,
			filepath.Join(npmDir, agent.Executable),
			filepath.Join(npmDir, agent.Executable+".cmd"),
			filepath.Join(npmDir, agent.Executable+".ps1"),
			filepath.Join(npmDir, agent.Executable+".exe"),
		)
	}
	if localAppData != "" {
		candidates = append(candidates,
			filepath.Join(localAppData, "Programs", agent.Executable, agent.Executable+".exe"),
		)
		if agent.Name == "multica" {
			candidates = append(candidates, filepath.Join(localAppData, "multica", "multica.exe"))
		}
	}
	if programFiles != "" {
		candidates = append(candidates,
			filepath.Join(programFiles, agent.Executable, agent.Executable+".exe"),
		)
	}
	if userProfile != "" {
		localBin := filepath.Join(userProfile, ".local", "bin")
		candidates = append(candidates,
			filepath.Join(localBin, agent.Executable),
			filepath.Join(localBin, agent.Executable+".cmd"),
			filepath.Join(localBin, agent.Executable+".ps1"),
			filepath.Join(localBin, agent.Executable+".exe"),
		)
	}
	return candidates
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func firstMeaningfulLine(raw string) string {
	fallback := ""
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if fallback == "" {
				fallback = trimmed
			}
			if strings.HasPrefix(trimmed, "WARNING:") || strings.HasPrefix(trimmed, "warning:") {
				continue
			}
			return trimmed
		}
	}
	return fallback
}
