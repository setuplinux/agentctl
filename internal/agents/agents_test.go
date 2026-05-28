package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupportedAgentsIncludesCatalogOrder(t *testing.T) {
	got := Supported()
	want := []string{"hermes", "openclaw", "claude", "codex", "gemini", "multica"}

	if len(got) != len(want) {
		t.Fatalf("Supported() length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("Supported()[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestAionUiIsRemovedFromCatalog(t *testing.T) {
	if _, ok := Find("aionui"); ok {
		t.Fatal("Find(aionui) = true, want removed from catalog")
	}
}

func TestMulticaLinuxSupportInstallsWithOfficialScriptAndUpdatesWithCLI(t *testing.T) {
	agent, ok := Find("multica")
	if !ok {
		t.Fatal("Find(multica) = false")
	}
	if agent.Executable != "multica" {
		t.Fatalf("Executable = %q, want multica", agent.Executable)
	}
	if got := strings.Join(agent.VersionArgs, " "); got != "--version" {
		t.Fatalf("Multica VersionArgs = %q, want --version", got)
	}
	support, ok := agent.Platforms[PlatformLinux]
	if !ok {
		t.Fatal("Multica missing Linux support")
	}
	if support.Install == nil {
		t.Fatal("Multica Linux install command is nil")
	}
	if support.Update == nil {
		t.Fatal("Multica Linux update command is nil")
	}
	install := support.Install.Program + " " + strings.Join(support.Install.Args, " ")
	for _, want := range []string{"bash", "raw.githubusercontent.com/multica-ai/multica/main/scripts/install.sh"} {
		if !strings.Contains(install, want) {
			t.Fatalf("Multica Linux install command missing %q: %s", want, install)
		}
	}
	if support.Update.Program != "multica" || strings.Join(support.Update.Args, " ") != "update" {
		t.Fatalf("Multica Linux update = %s %s, want multica update", support.Update.Program, strings.Join(support.Update.Args, " "))
	}
}

func TestMulticaDarwinSupportInstallsWithOfficialScriptAndUpdatesWithCLI(t *testing.T) {
	agent, ok := Find("multica")
	if !ok {
		t.Fatal("Find(multica) = false")
	}
	support, ok := agent.Platforms[PlatformDarwin]
	if !ok {
		t.Fatal("Multica missing Darwin support")
	}
	if support.Install == nil || support.Update == nil {
		t.Fatalf("Multica Darwin install/update commands must both be configured: %#v", support)
	}
	install := support.Install.Program + " " + strings.Join(support.Install.Args, " ")
	if !strings.Contains(install, "raw.githubusercontent.com/multica-ai/multica/main/scripts/install.sh") {
		t.Fatalf("Multica Darwin install command should use official script: %s", install)
	}
	if support.Update.Program != "multica" || strings.Join(support.Update.Args, " ") != "update" {
		t.Fatalf("Multica Darwin update = %s %s, want multica update", support.Update.Program, strings.Join(support.Update.Args, " "))
	}
}

func TestEveryAgentInstallSupportHasUninstallExceptInteractiveHermes(t *testing.T) {
	for _, agent := range Supported() {
		for platform, support := range agent.Platforms {
			if agent.Name == "hermes" {
				continue
			}
			if support.Install != nil && support.Uninstall == nil {
				t.Fatalf("%s has install support on %s but no uninstall command", agent.Name, platform)
			}
		}
	}
}

func TestHermesDoesNotAdvertiseNonInteractiveUninstall(t *testing.T) {
	agent, ok := Find("hermes")
	if !ok {
		t.Fatal("Find(hermes) = false")
	}
	for _, platform := range []Platform{PlatformLinux, PlatformDarwin} {
		support := agent.Platforms[platform]
		if support.Uninstall != nil {
			t.Fatalf("Hermes %s uninstall should be nil until native hermes uninstall supports non-interactive subprocesses", platform)
		}
	}
}

func TestCheckAllForPlatformMarksInstalledAndMissingAgents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lookup := func(name string) (string, error) {
		switch name {
		case "hermes", "codex":
			return "/fake/bin/" + name, nil
		default:
			return "", ErrNotFound
		}
	}
	runner := func(name string, args ...string) (string, error) {
		return name + " 1.2.3\n", nil
	}

	statuses := CheckAllForPlatform(PlatformLinux, lookup, runner)

	assertStatusState(t, statuses, "hermes", "installed")
	assertStatusState(t, statuses, "codex", "installed")
	assertStatusState(t, statuses, "openclaw", "missing")
	assertStatusState(t, statuses, "claude", "missing")
	assertStatusState(t, statuses, "gemini", "missing")
	assertStatusVersion(t, statuses, "hermes", "hermes 1.2.3")
	assertStatusVersion(t, statuses, "codex", "codex 1.2.3")
}

func TestWindowsSupportFlagsReflectCatalog(t *testing.T) {
	lookup := func(name string) (string, error) {
		return "", ErrNotFound
	}

	statuses := CheckAllForPlatform(PlatformWindows, lookup, nil)

	assertSupport(t, statuses, "hermes", false, false)
	assertSupport(t, statuses, "claude", true, true)
	assertSupport(t, statuses, "openclaw", true, true)
	assertSupport(t, statuses, "codex", true, true)
	assertSupport(t, statuses, "gemini", true, true)
	assertSupport(t, statuses, "multica", true, true)
}

func TestWindowsInstallsDoNotAssumeNpmIsAlreadyInstalled(t *testing.T) {
	for _, agent := range Supported() {
		support, ok := agent.Platforms[PlatformWindows]
		if !ok || support.Install == nil {
			continue
		}
		if support.Install.Program == "npm" {
			t.Fatalf("%s Windows install calls npm directly; use windowsNpmInstallSpec so clean Windows can bootstrap Node.js first", agent.Name)
		}
	}
}

func TestGeminiSupportUsesOfficialNpmPackage(t *testing.T) {
	agent, ok := Find("gemini")
	if !ok {
		t.Fatal("Find(gemini) = false")
	}
	if agent.Executable != "gemini" {
		t.Fatalf("Executable = %q, want gemini", agent.Executable)
	}
	for _, platform := range []Platform{PlatformLinux, PlatformDarwin, PlatformWindows} {
		support, ok := agent.Platforms[platform]
		if !ok {
			t.Fatalf("Gemini missing %s support", platform)
		}
		for label, spec := range map[string]*CommandSpec{
			"install":   support.Install,
			"update":    support.Update,
			"uninstall": support.Uninstall,
		} {
			if spec == nil {
				t.Fatalf("Gemini %s %s command is nil", platform, label)
			}
			command := spec.Program + " " + strings.Join(spec.Args, " ")
			for _, want := range []string{"npm", "-g", "@google/gemini-cli"} {
				if !strings.Contains(command, want) {
					t.Fatalf("Gemini %s %s command missing %q: %s", platform, label, want, command)
				}
			}
		}
	}
}

func TestWindowsNpmBackedInstallsBootstrapNodeWithWinget(t *testing.T) {
	for name, packageName := range map[string]string{
		"codex":  "@openai/codex",
		"gemini": "@google/gemini-cli",
	} {
		agent, ok := Find(name)
		if !ok {
			t.Fatalf("Find(%s) = false", name)
		}
		support, ok := agent.Platforms[PlatformWindows]
		if !ok {
			t.Fatalf("%s missing Windows support", name)
		}
		if support.Install == nil {
			t.Fatalf("%s Windows install command is nil", name)
		}
		command := support.Install.Program + " " + strings.Join(support.Install.Args, " ")
		for _, want := range []string{"powershell", "winget install", "OpenJS.NodeJS.LTS", "npm", "install", "-g", packageName} {
			if !strings.Contains(command, want) {
				t.Fatalf("%s Windows install command missing %q: %s", name, want, command)
			}
		}
	}
}

func TestCodexSupportUpdatesViaNpmPackageNotRemovedUpgradeFlag(t *testing.T) {
	agent, ok := Find("codex")
	if !ok {
		t.Fatal("Find(codex) = false")
	}
	for _, platform := range []Platform{PlatformLinux, PlatformDarwin, PlatformWindows} {
		support, ok := agent.Platforms[platform]
		if !ok {
			t.Fatalf("Codex missing %s support", platform)
		}
		if support.Update == nil {
			t.Fatalf("Codex %s update command is nil", platform)
		}
		command := support.Update.Program + " " + strings.Join(support.Update.Args, " ")
		if strings.Contains(command, "--upgrade") {
			t.Fatalf("Codex %s update still uses removed --upgrade flag: %s", platform, command)
		}
		for _, want := range []string{"npm", "install", "-g", "@openai/codex"} {
			if !strings.Contains(command, want) {
				t.Fatalf("Codex %s update command missing %q: %s", platform, want, command)
			}
		}
	}
}

func TestGeminiWindowsUpdateAlsoBootstrapsNodeWithWinget(t *testing.T) {
	agent, ok := Find("gemini")
	if !ok {
		t.Fatal("Find(gemini) = false")
	}
	support, ok := agent.Platforms[PlatformWindows]
	if !ok {
		t.Fatal("Gemini missing Windows support")
	}
	if support.Update == nil {
		t.Fatal("Gemini Windows update command is nil")
	}
	command := support.Update.Program + " " + strings.Join(support.Update.Args, " ")
	for _, want := range []string{"powershell", "winget install", "OpenJS.NodeJS.LTS", "npm", "install", "-g", "@google/gemini-cli"} {
		if !strings.Contains(command, want) {
			t.Fatalf("Gemini Windows update command missing %q: %s", want, command)
		}
	}
}

func TestLinuxDetectionFindsClaudeInLocalBinWhenPathIsStale(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	claudePath := filepath.Join(tempHome, ".local", "bin", "claude")
	if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(claudePath, []byte("stub"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	lookup := func(name string) (string, error) {
		return "", ErrNotFound
	}
	runner := func(name string, args ...string) (string, error) {
		if name != claudePath {
			return "", ErrNotFound
		}
		return "2.1.126 (Claude Code)\n", nil
	}

	agent, ok := Find("claude")
	if !ok {
		t.Fatal("Find(claude) = false")
	}
	status := CheckAgent(PlatformLinux, agent, lookup, runner)

	if status.State != "installed" {
		t.Fatalf("status.State = %q, want installed", status.State)
	}
	if status.Path != claudePath {
		t.Fatalf("status.Path = %q, want %q", status.Path, claudePath)
	}
	if status.Version != "2.1.126 (Claude Code)" {
		t.Fatalf("status.Version = %q", status.Version)
	}
}

func TestWindowsDetectionFindsClaudeInLocalBinWhenPathIsStale(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("APPDATA", filepath.Join(tempHome, "AppData", "Roaming"))

	claudePath := filepath.Join(tempHome, ".local", "bin", "claude.exe")
	if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(claudePath, []byte("stub"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	lookup := func(name string) (string, error) {
		return "", ErrNotFound
	}
	runner := func(name string, args ...string) (string, error) {
		return "2.1.126 (Claude Code)\n", nil
	}

	agent, ok := Find("claude")
	if !ok {
		t.Fatal("Find(claude) = false")
	}
	status := CheckAgent(PlatformWindows, agent, lookup, runner)

	if status.State != "installed" {
		t.Fatalf("status.State = %q, want installed", status.State)
	}
	if status.Path != claudePath {
		t.Fatalf("status.Path = %q, want %q", status.Path, claudePath)
	}
	if status.Version != "2.1.126 (Claude Code)" {
		t.Fatalf("status.Version = %q", status.Version)
	}
}

func assertStatusState(t *testing.T, statuses []Status, name string, want string) {
	t.Helper()
	for _, status := range statuses {
		if status.Name == name {
			if status.State != want {
				t.Fatalf("%s status = %q, want %q", name, status.State, want)
			}
			return
		}
	}
	t.Fatalf("missing status for %s in %#v", name, statuses)
}

func assertStatusVersion(t *testing.T, statuses []Status, name string, want string) {
	t.Helper()
	for _, status := range statuses {
		if status.Name == name {
			if status.Version != want {
				t.Fatalf("%s version = %q, want %q", name, status.Version, want)
			}
			return
		}
	}
	t.Fatalf("missing status for %s in %#v", name, statuses)
}

func assertSupport(t *testing.T, statuses []Status, name string, wantInstall bool, wantUpdate bool) {
	t.Helper()
	for _, status := range statuses {
		if status.Name == name {
			if status.SupportsInstall != wantInstall {
				t.Fatalf("%s SupportsInstall = %v, want %v", name, status.SupportsInstall, wantInstall)
			}
			if status.SupportsUpdate != wantUpdate {
				t.Fatalf("%s SupportsUpdate = %v, want %v", name, status.SupportsUpdate, wantUpdate)
			}
			return
		}
	}
	t.Fatalf("missing status for %s in %#v", name, statuses)
}
