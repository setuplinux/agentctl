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

const aionUiLinuxUninstallScript = `
set -euo pipefail
if [ "$(id -u)" -eq 0 ]; then
  apt-get remove -y aionui
elif command -v sudo >/dev/null 2>&1; then
  sudo apt-get remove -y aionui
else
  echo "AionUi uninstall requires root or sudo" >&2
  exit 1
fi
`

const aionUiWindowsUninstallScript = `
$ErrorActionPreference = "Stop"
$winget = Get-Command winget -ErrorAction SilentlyContinue
if ($winget) {
  winget uninstall --id iOfficeAI.AionUi --exact --accept-source-agreements
  if ($LASTEXITCODE -eq 0) { exit 0 }
  Write-Host "winget uninstall iOfficeAI.AionUi failed; trying local uninstaller..."
}
$candidates = @()
foreach ($root in @($env:LOCALAPPDATA, $env:ProgramFiles, ${env:ProgramFiles(x86)})) {
  if ($root) { $candidates += Join-Path $root "Programs\AionUi\Uninstall AionUi.exe" }
  if ($root) { $candidates += Join-Path $root "AionUi\Uninstall AionUi.exe" }
}
$candidates = $candidates | Where-Object { $_ -and (Test-Path $_) }
if (-not $candidates) { throw "AionUi uninstaller not found" }
$uninstaller = $candidates | Select-Object -First 1
Write-Host "uninstall: $uninstaller /S"
$process = Start-Process -FilePath $uninstaller -ArgumentList "/S" -Wait -PassThru
exit $process.ExitCode
`

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

const aionUiLinuxDebInstallScript = `
set -euo pipefail
need() { command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }; }
need curl
need python3
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) asset_arch="amd64" ;;
  aarch64|arm64) asset_arch="arm64" ;;
  *) echo "unsupported AionUi Linux architecture: $arch" >&2; exit 2 ;;
esac
asset_url="$({ python3 - "$asset_arch" <<'PY'
import json, sys, urllib.request
asset_arch = sys.argv[1]
req = urllib.request.Request(
    'https://api.github.com/repos/iOfficeAI/AionUi/releases/latest',
    headers={'Accept':'application/vnd.github+json','User-Agent':'agentctl'}
)
data = json.load(urllib.request.urlopen(req, timeout=30))
suffix = f'linux-{asset_arch}.deb'
for asset in data.get('assets', []):
    if asset.get('name', '').endswith(suffix):
        print(asset['browser_download_url'])
        break
else:
    raise SystemExit(f'no AionUi release asset ending with {suffix}')
PY
} )"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
deb="$tmpdir/aionui.deb"
echo "download: $asset_url"
curl -fL "$asset_url" -o "$deb"
if [ "$(id -u)" -eq 0 ]; then
  apt-get install -y "$deb"
elif command -v sudo >/dev/null 2>&1; then
  sudo apt-get install -y "$deb"
else
  echo "AionUi .deb downloaded to $deb but installing requires root or sudo" >&2
  exit 1
fi
`

const aionUiWindowsInstallScript = `
$ErrorActionPreference = "Stop"
$winget = Get-Command winget -ErrorAction SilentlyContinue
if ($winget) {
  winget install --id iOfficeAI.AionUi --exact --accept-package-agreements --accept-source-agreements
  if ($LASTEXITCODE -eq 0) { exit 0 }
  Write-Host "winget package iOfficeAI.AionUi was not installable; falling back to GitHub release installer..."
}
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "x64" }
$release = Invoke-RestMethod -Headers @{"User-Agent"="agentctl"} -Uri "https://api.github.com/repos/iOfficeAI/AionUi/releases/latest"
$suffix = "win-$arch.exe"
$asset = $release.assets | Where-Object { $_.name.EndsWith($suffix) } | Select-Object -First 1
if (-not $asset) { throw "No AionUi release asset ending with $suffix" }
$installer = Join-Path $env:TEMP $asset.name
Write-Host "download: $($asset.browser_download_url)"
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $installer
Write-Host "install: $installer /S"
$process = Start-Process -FilePath $installer -ArgumentList "/S" -Wait -PassThru
if ($process.ExitCode -ne 0) { exit $process.ExitCode }
$installDir = Join-Path $env:LOCALAPPDATA "Programs\AionUi"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$pathParts = @()
if ($userPath) { $pathParts = $userPath -split ';' | Where-Object { $_ } }
$alreadyInPath = $false
foreach ($part in $pathParts) {
  if ($part.TrimEnd('\') -ieq $installDir.TrimEnd('\')) { $alreadyInPath = $true; break }
}
if (-not $alreadyInPath) {
  $newUserPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
  [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
  Write-Host "path: added $installDir to user PATH. Open a new terminal to use AionUi.exe directly."
}
if (($env:PATH -split ';') -notcontains $installDir) {
  $env:PATH = "$installDir;$env:PATH"
}
exit 0
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
					Uninstall: &CommandSpec{
						Program: "hermes",
						Args:    []string{"uninstall", "--yes"},
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
					Uninstall: &CommandSpec{
						Program: "hermes",
						Args:    []string{"uninstall", "--yes"},
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
					Install: &CommandSpec{
						Program: "npm",
						Args:    []string{"install", "-g", "@openai/codex"},
					},
					Update: &CommandSpec{
						Program: "codex",
						Args:    []string{"--upgrade"},
					},
					Uninstall: &CommandSpec{
						Program: "npm",
						Args:    []string{"uninstall", "-g", "@openai/codex"},
					},
					FirstRunHint: "Run `codex --login` after install.",
					Notes: []string{
						"Official Codex CLI supports Linux and macOS. Windows support is still experimental and often works best in WSL.",
					},
				},
				PlatformDarwin: {
					Install: &CommandSpec{
						Program: "npm",
						Args:    []string{"install", "-g", "@openai/codex"},
					},
					Update: &CommandSpec{
						Program: "codex",
						Args:    []string{"--upgrade"},
					},
					Uninstall: &CommandSpec{
						Program: "npm",
						Args:    []string{"uninstall", "-g", "@openai/codex"},
					},
					FirstRunHint: "Run `codex --login` after install.",
				},
				PlatformWindows: {
					Install: &CommandSpec{
						Program: "npm",
						Args:    []string{"install", "-g", "@openai/codex"},
					},
					Update: &CommandSpec{
						Program: "codex",
						Args:    []string{"--upgrade"},
					},
					Uninstall: &CommandSpec{
						Program: "npm",
						Args:    []string{"uninstall", "-g", "@openai/codex"},
					},
					FirstRunHint: "Run `codex --login` after install.",
					Notes: []string{
						"Official Windows support is still experimental; WSL is often the safer route for coding workflows.",
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
					Install: &CommandSpec{
						Program: "npm",
						Args:    []string{"install", "-g", "@google/gemini-cli"},
					},
					Update: &CommandSpec{
						Program: "npm",
						Args:    []string{"install", "-g", "@google/gemini-cli"},
					},
					Uninstall: &CommandSpec{
						Program: "npm",
						Args:    []string{"uninstall", "-g", "@google/gemini-cli"},
					},
					FirstRunHint: "Run `gemini`, then choose a Google authentication method.",
					Notes: []string{
						"Official Gemini CLI standard installation uses npm package @google/gemini-cli.",
					},
				},
				PlatformDarwin: {
					Install: &CommandSpec{
						Program: "npm",
						Args:    []string{"install", "-g", "@google/gemini-cli"},
					},
					Update: &CommandSpec{
						Program: "npm",
						Args:    []string{"install", "-g", "@google/gemini-cli"},
					},
					Uninstall: &CommandSpec{
						Program: "npm",
						Args:    []string{"uninstall", "-g", "@google/gemini-cli"},
					},
					FirstRunHint: "Run `gemini`, then choose a Google authentication method.",
				},
				PlatformWindows: {
					Install: &CommandSpec{
						Program: "npm",
						Args:    []string{"install", "-g", "@google/gemini-cli"},
					},
					Update: &CommandSpec{
						Program: "npm",
						Args:    []string{"install", "-g", "@google/gemini-cli"},
					},
					Uninstall: &CommandSpec{
						Program: "npm",
						Args:    []string{"uninstall", "-g", "@google/gemini-cli"},
					},
					FirstRunHint: "Run `gemini`, then choose a Google authentication method.",
					Notes: []string{
						"On Windows, npm usually places the gemini shim under %APPDATA%\\npm.",
					},
				},
			},
		},
		{
			Name:        "aionui",
			Executable:  "AionUi",
			Description: "AionUi desktop cowork app for local AI agents",
			Platforms: map[Platform]PlatformSupport{
				PlatformLinux: {
					Install: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", aionUiLinuxDebInstallScript},
					},
					Update: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", aionUiLinuxDebInstallScript},
					},
					Uninstall: &CommandSpec{
						Program: "bash",
						Args:    []string{"-lc", aionUiLinuxUninstallScript},
					},
					FirstRunHint: "Launch `AionUi` as a normal desktop user; Electron will not run as root without --no-sandbox, and root GUI app state is not recommended.",
					Notes: []string{
						"Linux install/update downloads the latest AionUi .deb from iOfficeAI/AionUi GitHub releases and installs it with apt-get.",
					},
				},
				PlatformDarwin: {
					FirstRunHint: "Install the latest AionUi .dmg/.zip from https://github.com/iOfficeAI/AionUi/releases, then launch AionUi from /Applications.",
					Notes: []string{
						"macOS AionUi is currently detect-only in agentctl; app bundle install/update is left to AionUi's Electron updater or manual GitHub release install.",
					},
				},
				PlatformWindows: {
					Install: &CommandSpec{
						Program: "powershell",
						Args:    []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", aionUiWindowsInstallScript},
					},
					Update: &CommandSpec{
						Program: "powershell",
						Args:    []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", aionUiWindowsInstallScript},
					},
					Uninstall: &CommandSpec{
						Program: "powershell",
						Args:    []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", aionUiWindowsUninstallScript},
					},
					FirstRunHint: "Launch AionUi after install; it auto-detects installed ACP/CLI agents such as Hermes, OpenClaw, Claude Code, Codex, Qwen, and OpenCode.",
					Notes: []string{
						"Windows install/update uses winget package iOfficeAI.AionUi from https://www.aionui.com/download/.",
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
		if version, err := runner(agent.Executable, agent.VersionArgs...); err == nil {
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
