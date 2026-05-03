package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunListPrintsSupportedAgents(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"list"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"hermes", "openclaw", "claude", "codex"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list output missing %q: %s", want, out)
		}
	}
}

func TestRunStatusPrintsFamilyFriendlyStatuses(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"status"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Agent status") {
		t.Fatalf("status output missing heading: %s", out)
	}
	if !strings.Contains(out, "hermes") {
		t.Fatalf("status output missing hermes: %s", out)
	}
}

func TestRunUnknownCommandReturnsHelpfulError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"bogus"}, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("exitCode = 0, want nonzero")
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr missing unknown command message: %s", stderr.String())
	}
}

func TestRunVersionPrintsAgentctlVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"version"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "agentctl") {
		t.Fatalf("version output missing agentctl name: %s", out)
	}
	if !strings.Contains(out, version) {
		t.Fatalf("version output missing version %q: %s", version, out)
	}
}

func TestRunHelpShowsUsageExamplesAndUninstall(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"help"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"agentctl uninstall <agent|all>", "agentctl version", "Examples:", "agentctl install aionui", "agentctl update all", "agentctl uninstall codex"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q: %s", want, out)
		}
	}
}

func TestRunUninstallRequiresTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"uninstall"}, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("exitCode = 0, want nonzero")
	}
	if !strings.Contains(stderr.String(), "usage: agentctl uninstall <agent|all>") {
		t.Fatalf("stderr missing uninstall usage: %s", stderr.String())
	}
}

func TestRunRollbackRequiresOpenClawTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"rollback"}, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("exitCode = 0, want nonzero")
	}
	if !strings.Contains(stderr.String(), "usage: agentctl rollback openclaw") {
		t.Fatalf("stderr missing rollback usage: %s", stderr.String())
	}
}

func TestWriteAndLoadLatestOpenClawRollbackSnapshot(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	snapshotDir := filepath.Join(tempHome, ".openclaw", "agentctl", "rollback", "openclaw-20260503-120000")
	if err := os.MkdirAll(snapshotDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	want := &openClawRollbackSnapshot{
		CreatedAt:    "2026-05-03T12:00:00Z",
		Version:      "openclaw 1.2.3",
		ConfigBackup: filepath.Join(tempHome, ".openclaw", "config-backups", "openclaw.json.bak.agentctl-20260503-120000"),
		SnapshotDir:  snapshotDir,
		PatchedFiles: []openClawRollbackFile{
			{
				TargetPath: "/usr/lib/node_modules/openclaw/dist/frontmatter-Cc-V8aI2.js",
				BackupPath: filepath.Join(snapshotDir, "usr__lib__node_modules__openclaw__dist__frontmatter-Cc-V8aI2.js"),
			},
		},
	}

	if err := writeOpenClawRollbackSnapshot(want); err != nil {
		t.Fatalf("writeOpenClawRollbackSnapshot() error = %v", err)
	}

	got, err := loadLatestOpenClawRollbackSnapshot()
	if err != nil {
		t.Fatalf("loadLatestOpenClawRollbackSnapshot() error = %v", err)
	}

	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal(got) error = %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(want) error = %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("snapshot mismatch:\n got=%s\nwant=%s", gotJSON, wantJSON)
	}
}
