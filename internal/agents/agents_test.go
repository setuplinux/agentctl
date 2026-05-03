package agents

import "testing"

func TestSupportedAgentsIncludesInitialFour(t *testing.T) {
	got := Supported()
	want := []string{"hermes", "openclaw", "claude", "codex"}

	if len(got) != len(want) {
		t.Fatalf("Supported() length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("Supported()[%d].Name = %q, want %q", i, got[i].Name, name)
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
