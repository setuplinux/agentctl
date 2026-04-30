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

func TestCheckAllMarksInstalledAndMissingAgents(t *testing.T) {
	lookup := func(name string) (string, error) {
		switch name {
		case "hermes", "codex":
			return "/fake/bin/" + name, nil
		default:
			return "", ErrNotFound
		}
	}

	statuses := CheckAll(lookup)

	assertStatus(t, statuses, "hermes", "installed")
	assertStatus(t, statuses, "codex", "installed")
	assertStatus(t, statuses, "openclaw", "missing")
	assertStatus(t, statuses, "claude", "missing")
}

func assertStatus(t *testing.T, statuses []Status, name string, want string) {
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
