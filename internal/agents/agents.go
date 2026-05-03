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
	Name            string
	State           string
	Path            string
	Version         string
	SupportsInstall bool
	SupportsUpdate  bool
	SupportsDoctor  bool
	FirstRunHint    string
	Notes           []string
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
					FirstRunHint: "Run `codex --login` after install.",
					Notes: []string{
						"Official Windows support is still experimental; WSL is often the safer route for coding workflows.",
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
	userProfile := strings.TrimSpace(os.Getenv("USERPROFILE"))
	candidates := make([]string, 0, 8)

	if appData != "" {
		npmDir := filepath.Join(appData, "npm")
		candidates = append(candidates,
			filepath.Join(npmDir, agent.Executable),
			filepath.Join(npmDir, agent.Executable+".cmd"),
			filepath.Join(npmDir, agent.Executable+".ps1"),
			filepath.Join(npmDir, agent.Executable+".exe"),
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
