package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/setuplinux/agentctl/internal/agents"
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

func TestRunBackupRejectsNonOpenClawAgents(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"backup", "codex"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), "usage: agentctl backup openclaw") {
		t.Fatalf("stderr missing backup usage: %s", stderr.String())
	}
}

func TestOpenClawActionableLogFiltersDoNotMatchRawTelegramMessages(t *testing.T) {
	for _, haystack := range []string{windowsOpenClawRecentLogsScript, linuxOpenClawActionableLogsScript} {
		if strings.Contains(haystack, "|telegram|") || strings.Contains(haystack, "|telegram'") || strings.Contains(haystack, "|telegram|") {
			t.Fatalf("actionable log filter still matches raw telegram log lines: %s", haystack)
		}
	}
}

func TestPatchOpenClawFrontmatterImportsSkipsMissingBundle(t *testing.T) {
	if err := patchOpenClawFrontmatterImports(t.TempDir()); err != nil {
		t.Fatalf("missing frontmatter bundle should be skipped, got error: %v", err)
	}
}

func TestRunCommandSpecForAgentUsesDetectedPathForAgentExecutable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	spec := &agents.CommandSpec{Program: "echo", Args: []string{"detected-path-used"}}
	agent := agents.Agent{Executable: "echo"}
	status := agents.Status{Path: "/bin/printf"}

	exitCode := runCommandSpecForAgent(&stdout, &stderr, time.Second, spec, agent, status)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "$ /bin/printf detected-path-used") {
		t.Fatalf("stdout did not show detected path command: %s", out)
	}
	if !strings.Contains(out, "detected-path-used") {
		t.Fatalf("stdout missing command output: %s", out)
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
	for _, want := range []string{"agentctl tui", "agentctl bundle <agent|all>", "agentctl backup openclaw", "agentctl uninstall <agent|all>", "agentctl version", "Examples:", "agentctl tui --dry-run", "agentctl install aionui", "agentctl update all", "agentctl uninstall codex"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q: %s", want, out)
		}
	}
}

func TestRunTUIDryRunShowsAgentAndActionMenus(t *testing.T) {
	var stdout, stderr bytes.Buffer
	input := strings.NewReader("q")
	exitCode := RunWithIO([]string{"tui", "--dry-run"}, input, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"agentctl operations console", "Use ↑/↓", "Hermes", "OpenClaw", "Codex", "dry-run"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tui output missing %q: %s", want, out)
		}
	}
}

func TestRunTUIDryRunArrowSelectsOpenClawUpdateWithoutExecuting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	input := strings.NewReader("\x1b[B\r\x1b[B\r")
	exitCode := RunWithIO([]string{"tui", "--dry-run"}, input, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"selected agent: openclaw", "selected action: update", "dry-run: agentctl update openclaw"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tui dry-run output missing %q: %s", want, out)
		}
	}
}

func TestRunTUIDryRunViKeysSelectOpenClawUpdateWithoutExecuting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	input := strings.NewReader("j\rj\r")
	exitCode := RunWithIO([]string{"tui", "--dry-run"}, input, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"selected agent: openclaw", "selected action: update", "dry-run: agentctl update openclaw"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tui dry-run vi-key output missing %q: %s", want, out)
		}
	}
}

func TestSelectTUIChoiceTerminalControlUsesCRLFAndRedraw(t *testing.T) {
	var stdout bytes.Buffer
	reader := bufio.NewReader(strings.NewReader("j\r"))
	choices := []tuiChoice{{Label: "Hermes", Value: "hermes"}, {Label: "OpenClaw", Value: "openclaw"}}

	selected, ok := selectTUIChoice(reader, &stdout, "Select agent", choices, true)
	if !ok {
		t.Fatalf("selectTUIChoice() ok = false, want true")
	}
	if selected.Value != "openclaw" {
		t.Fatalf("selected = %q, want openclaw", selected.Value)
	}
	out := stdout.String()
	if !strings.Contains(out, "\r\n") {
		t.Fatalf("terminal output missing CRLF line endings: %q", out)
	}
	if !strings.Contains(out, "\x1b[3F\x1b[J") {
		t.Fatalf("terminal output missing menu redraw clear sequence: %q", out)
	}
}

func TestReadTUIConfirmationAcceptsYWithoutNewlineAndCancelsInterrupts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "y without newline", input: "y", want: true},
		{name: "uppercase y", input: "Y", want: true},
		{name: "ctrl c", input: string([]byte{0x03}), want: false},
		{name: "ctrl d", input: string([]byte{0x04}), want: false},
		{name: "q", input: "q", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			got := readTUIConfirmation(bufio.NewReader(strings.NewReader(tt.input)), &stdout, true)
			if got != tt.want {
				t.Fatalf("readTUIConfirmation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadTUIKeyCancelsOnControlCAndD(t *testing.T) {
	for _, input := range []byte{0x03, 0x04} {
		key, ok := readTUIKey(bufio.NewReader(strings.NewReader(string([]byte{input}))))
		if !ok || key != "quit" {
			t.Fatalf("readTUIKey(%#x) = %q, %v; want quit, true", input, key, ok)
		}
	}
}

func TestRunTUIEnablesRawModeForFileInput(t *testing.T) {
	oldRaw := makeTerminalRaw
	defer func() { makeTerminalRaw = oldRaw }()

	called := false
	restored := false
	makeTerminalRaw = func(file *os.File) (func(), error) {
		called = true
		return func() { restored = true }, nil
	}

	var stdout, stderr bytes.Buffer
	exitCode := RunWithIO([]string{"tui", "--dry-run"}, strings.NewReader("q"), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("string reader exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	if called || restored {
		t.Fatalf("raw mode should not be enabled for non-file input; called=%v restored=%v", called, restored)
	}

	called = false
	restored = false
	file, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	defer file.Close()
	if _, err := file.WriteString("q"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = RunWithIO([]string{"tui", "--dry-run"}, file, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("file reader exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	if !called {
		t.Fatalf("raw mode was not enabled for file input")
	}
	if !restored {
		t.Fatalf("raw mode restore function was not called")
	}
}

func TestRunBundleCreatesRedactedSupportBundle(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"bundle", "codex"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "support bundle:") {
		t.Fatalf("bundle output missing path: %s", out)
	}
	bundlePath := strings.TrimSpace(strings.TrimPrefix(out[strings.LastIndex(out, "support bundle:"):], "support bundle:"))
	if _, err := os.Stat(bundlePath); err != nil {
		t.Fatalf("bundle path %q not created: %v", bundlePath, err)
	}
}

func TestRedactSensitiveTextMasksTokensAndCallbackURLs(t *testing.T) {
	input := "OPENAI_API_KEY=" + "sk-" + "test callback=http://localhost:1455/auth/callback?code=abc refresh_token=secret bot123456:ABCDEF"
	got := redactSensitiveText(input)
	for _, forbidden := range []string{"sk-test", "code=abc", "secret", "bot123456:ABCDEF"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redacted text still contains %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redacted text missing marker: %s", got)
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

func TestParseOpenClawUpdateAvailability(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantAvail   bool
		wantChecked bool
		wantErr     bool
	}{
		{
			name:        "available false",
			json:        `{"availability":{"available":false}}`,
			wantAvail:   false,
			wantChecked: true,
		},
		{
			name:        "available true",
			json:        `{"availability":{"available":true}}`,
			wantAvail:   true,
			wantChecked: true,
		},
		{
			name:        "missing availability",
			json:        `{"update":{"root":"/tmp/openclaw"}}`,
			wantAvail:   false,
			wantChecked: false,
		},
		{
			name:    "invalid json",
			json:    `{`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAvail, gotChecked, err := parseOpenClawUpdateAvailability([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseOpenClawUpdateAvailability() err = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOpenClawUpdateAvailability() err = %v", err)
			}
			if gotAvail != tt.wantAvail || gotChecked != tt.wantChecked {
				t.Fatalf("parseOpenClawUpdateAvailability() = (%v, %v), want (%v, %v)", gotAvail, gotChecked, tt.wantAvail, tt.wantChecked)
			}
		})
	}
}

func TestWindowsOpenClawRepairScriptQuarantinesBrokenInstallAndReinstalls(t *testing.T) {
	for _, want := range []string{
		"Move-Item -Path $openclawDir",
		"npm",
		"install -g openclaw@latest",
		"gateway install",
		"gateway start",
	} {
		if !strings.Contains(windowsOpenClawRepairInstallScript, want) {
			t.Fatalf("windows repair script missing %q:\n%s", want, windowsOpenClawRepairInstallScript)
		}
	}
}

func TestWindowsOpenClawScriptsAvoidLinuxOnlySystemctlJournalctlBash(t *testing.T) {
	windowsScripts := map[string]string{
		"service status": windowsOpenClawServiceStatusScript,
		"recent logs":    windowsOpenClawRecentLogsScript,
		"stop gateway":   windowsOpenClawStopGatewayScript,
		"repair install": windowsOpenClawRepairInstallScript,
	}
	for name, script := range windowsScripts {
		lower := strings.ToLower(script)
		for _, forbidden := range []string{"systemctl", "journalctl", "bash -lc"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s script contains Linux-only command %q:\n%s", name, forbidden, script)
			}
		}
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
