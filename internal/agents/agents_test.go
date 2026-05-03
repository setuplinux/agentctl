package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupportedAgentsIncludesCatalogOrder(t *testing.T) {
	got := Supported()
	want := []string{"hermes", "openclaw", "claude", "codex", "aionui"}

	if len(got) != len(want) {
		t.Fatalf("Supported() length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("Supported()[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestAionUiLinuxSupportInstallsAndUpdatesFromLatestDeb(t *testing.T) {
	agent, ok := Find("aionui")
	if !ok {
		t.Fatal("Find(aionui) = false")
	}
	if agent.Executable != "AionUi" {
		t.Fatalf("Executable = %q, want AionUi", agent.Executable)
	}
	if len(agent.VersionArgs) != 0 {
		t.Fatalf("AionUi VersionArgs = %v, want none because Electron --version launches app state", agent.VersionArgs)
	}
	support, ok := agent.Platforms[PlatformLinux]
	if !ok {
		t.Fatal("AionUi missing Linux support")
	}
	if support.Install == nil {
		t.Fatal("AionUi Linux install command is nil")
	}
	if support.Update == nil {
		t.Fatal("AionUi Linux update command is nil")
	}
	install := support.Install.Program + " " + strings.Join(support.Install.Args, " ")
	update := support.Update.Program + " " + strings.Join(support.Update.Args, " ")
	for _, command := range []string{install, update} {
		for _, want := range []string{"api.github.com/repos/iOfficeAI/AionUi/releases/latest", ".deb", "apt-get"} {
			if !strings.Contains(command, want) {
				t.Fatalf("AionUi command missing %q: %s", want, command)
			}
		}
	}
}

func TestAionUiWindowsSupportInstallsAndUpdatesWithWinget(t *testing.T) {
	agent, ok := Find("aionui")
	if !ok {
		t.Fatal("Find(aionui) = false")
	}
	support, ok := agent.Platforms[PlatformWindows]
	if !ok {
		t.Fatal("AionUi missing Windows support")
	}
	if support.Install == nil {
		t.Fatal("AionUi Windows install command is nil")
	}
	if support.Update == nil {
		t.Fatal("AionUi Windows update command is nil")
	}
	for label, spec := range map[string]*CommandSpec{"install": support.Install, "update": support.Update} {
		command := spec.Program + " " + strings.Join(spec.Args, " ")
		for _, want := range []string{"winget", "iOfficeAI.AionUi", "--accept-package-agreements", "--accept-source-agreements"} {
			if !strings.Contains(command, want) {
				t.Fatalf("AionUi Windows %s command missing %q: %s", label, want, command)
			}
		}
	}
}

func TestCheckAllForPlatformMarksInstalledAndMissingAgents(t *testing.T) {
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
	assertSupport(t, statuses, "aionui", true, true)
}

func TestWindowsDetectionFindsAionUiInLocalAppProgramsWhenPathIsStale(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(tempHome, "AppData", "Local"))
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("APPDATA", filepath.Join(tempHome, "AppData", "Roaming"))

	aionPath := filepath.Join(tempHome, "AppData", "Local", "Programs", "AionUi", "AionUi.exe")
	if err := os.MkdirAll(filepath.Dir(aionPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(aionPath, []byte("stub"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	lookup := func(name string) (string, error) {
		return "", ErrNotFound
	}

	agent, ok := Find("aionui")
	if !ok {
		t.Fatal("Find(aionui) = false")
	}
	status := CheckAgent(PlatformWindows, agent, lookup, nil)

	if status.State != "installed" {
		t.Fatalf("status.State = %q, want installed", status.State)
	}
	if status.Path != aionPath {
		t.Fatalf("status.Path = %q, want %q", status.Path, aionPath)
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
