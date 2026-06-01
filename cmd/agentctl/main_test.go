package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	status := agents.Status{Path: "/bin/echo"}
	expectedCommand := "$ /bin/echo detected-path-used"
	if runtime.GOOS == "windows" {
		spec = &agents.CommandSpec{Program: "cmd", Args: []string{"/c", "echo", "detected-path-used"}}
		agent = agents.Agent{Executable: "cmd"}
		status = agents.Status{Path: filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")}
		expectedCommand = "$ " + status.Path + " /c echo detected-path-used"
	}

	exitCode := runCommandSpecForAgent(&stdout, &stderr, time.Second, spec, agent, status)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, expectedCommand) {
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
	for _, want := range []string{"agentctl tui", "agentctl bundle <agent|all>", "agentctl backup openclaw", "agentctl uninstall <agent|all>", "agentctl version", "Examples:", "agentctl tui --dry-run", "agentctl install aionui", "agentctl update all", "agentctl update all --exclude codex", "agentctl uninstall codex"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q: %s", want, out)
		}
	}
}

func TestParseUpdateArgsSupportsExcludeVariants(t *testing.T) {
	name, exclude, err := parseUpdateArgs([]string{"all", "--exclude", "codex, gemini", "--exclude=openclaw"})
	if err != nil {
		t.Fatalf("parseUpdateArgs returned error: %v", err)
	}
	if name != "all" {
		t.Fatalf("name = %q, want all", name)
	}
	for _, want := range []string{"codex", "gemini", "openclaw"} {
		if _, ok := exclude[want]; !ok {
			t.Fatalf("exclude missing %q: %#v", want, exclude)
		}
	}
}

func TestParseUpdateArgsRejectsExcludeWithoutValue(t *testing.T) {
	_, _, err := parseUpdateArgs([]string{"all", "--exclude"})
	if err == nil {
		t.Fatal("expected missing-value error")
	}
	if !strings.Contains(err.Error(), "missing value for --exclude") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunUpdateRejectsExcludeForSingleAgent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"update", "codex", "--exclude", "gemini"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), "--exclude can only be used with `agentctl update all`") {
		t.Fatalf("stderr missing exclude guidance: %s", stderr.String())
	}
}

func TestRunOpenClawLogsRejectsNativeWindowsGracefully(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runOpenClawLogs("windows", &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), "not wired up on native Windows yet") {
		t.Fatalf("stderr missing Windows guidance: %s", stderr.String())
	}
}

func TestLinuxUserServiceEnvFillsRootUserBusWhenUnset(t *testing.T) {
	got := linuxUserServiceEnvForUID("0", []string{"HOME=/root", "PATH=/usr/bin"})
	for _, want := range []string{"XDG_RUNTIME_DIR=/run/user/0", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/0/bus"} {
		if !containsString(got, want) {
			t.Fatalf("linuxUserServiceEnvForUID() missing %q from %#v", want, got)
		}
	}
}

func TestLinuxUserServiceEnvPreservesExistingBusEnv(t *testing.T) {
	got := linuxUserServiceEnvForUID("0", []string{"XDG_RUNTIME_DIR=/custom/run", "DBUS_SESSION_BUS_ADDRESS=unix:path=/custom/bus"})
	for _, want := range []string{"XDG_RUNTIME_DIR=/custom/run", "DBUS_SESSION_BUS_ADDRESS=unix:path=/custom/bus"} {
		if !containsString(got, want) {
			t.Fatalf("linuxUserServiceEnvForUID() did not preserve %q in %#v", want, got)
		}
	}
	for _, forbidden := range []string{"XDG_RUNTIME_DIR=/run/user/0", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/0/bus"} {
		if containsString(got, forbidden) {
			t.Fatalf("linuxUserServiceEnvForUID() should not add fallback %q when env already set: %#v", forbidden, got)
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

func TestBubbleTUISelectModelHandlesArrowKeysAndSelection(t *testing.T) {
	model := newBubbleTUISelectModel("Select agent", []tuiChoice{
		{Label: "Hermes", Value: "hermes"},
		{Label: "OpenClaw", Value: "openclaw"},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(bubbleTUISelectModel)
	if model.cursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", model.cursor)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(bubbleTUISelectModel)
	if !model.done || model.cancelled || model.selected.Value != "openclaw" {
		t.Fatalf("selection = %+v; done=%v cancelled=%v, want openclaw done", model.selected, model.done, model.cancelled)
	}
}

func TestBubbleTUISelectModelCancelsOnControlC(t *testing.T) {
	model := newBubbleTUISelectModel("Select agent", []tuiChoice{{Label: "Hermes", Value: "hermes"}})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(bubbleTUISelectModel)
	if !model.cancelled || model.done {
		t.Fatalf("cancelled=%v done=%v, want cancelled only", model.cancelled, model.done)
	}
}

func TestBubbleTUIAgentActionModelSelectsBothStepsInOneProgram(t *testing.T) {
	model := newBubbleTUIAgentActionModel([]tuiChoice{
		{Label: "Hermes", Value: "hermes"},
		{Label: "OpenClaw", Value: "openclaw"},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(bubbleTUIAgentActionModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(bubbleTUIAgentActionModel)
	if model.step != "action" || model.agent.Value != "openclaw" || model.cursor != 0 || model.done {
		t.Fatalf("after agent select = %+v; want action step with openclaw and cursor reset", model)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(bubbleTUIAgentActionModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(bubbleTUIAgentActionModel)
	if !model.done || model.cancelled || model.action.Value != "update" {
		t.Fatalf("final selection = %+v; want update action done", model)
	}
}

func TestBubbleTUIAgentActionModelCancelsOnControlC(t *testing.T) {
	model := newBubbleTUIAgentActionModel([]tuiChoice{{Label: "Hermes", Value: "hermes"}})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(bubbleTUIAgentActionModel)
	if !model.cancelled || model.done {
		t.Fatalf("cancelled=%v done=%v, want cancelled only", model.cancelled, model.done)
	}
}

func TestBubbleTUIConfirmModelAcceptsEnterAndY(t *testing.T) {
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyRunes, Runes: []rune{'y'}}} {
		model := newBubbleTUIConfirmModel("claude", "update")
		updated, _ := model.Update(msg)
		model = updated.(bubbleTUIConfirmModel)
		if !model.confirmed || model.cancelled {
			t.Fatalf("confirmation after %v = confirmed %v cancelled %v; want confirmed only", msg, model.confirmed, model.cancelled)
		}
	}
}

func TestBubbleTUIConfirmModelCancelsOnNQAndControlC(t *testing.T) {
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'n'}}, {Type: tea.KeyRunes, Runes: []rune{'q'}}, {Type: tea.KeyCtrlC}} {
		model := newBubbleTUIConfirmModel("claude", "update")
		updated, _ := model.Update(msg)
		model = updated.(bubbleTUIConfirmModel)
		if model.confirmed || !model.cancelled {
			t.Fatalf("confirmation after %v = confirmed %v cancelled %v; want cancelled only", msg, model.confirmed, model.cancelled)
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

func TestReadTUIKeySupportsWindowsConsoleArrowSequences(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{name: "extended up", input: []byte{0xe0, 0x48}, want: "up"},
		{name: "extended down", input: []byte{0xe0, 0x50}, want: "down"},
		{name: "null-prefixed up", input: []byte{0x00, 0x48}, want: "up"},
		{name: "null-prefixed down", input: []byte{0x00, 0x50}, want: "down"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, ok := readTUIKey(bufio.NewReader(bytes.NewReader(tt.input)))
			if !ok || key != tt.want {
				t.Fatalf("readTUIKey(%#v) = %q, %v; want %q, true", tt.input, key, ok, tt.want)
			}
		})
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
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

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
