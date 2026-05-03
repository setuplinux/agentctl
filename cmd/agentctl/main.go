package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/setuplinux/agentctl/internal/agents"
)

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "list":
		return runList(stdout)
	case "status":
		return runStatus(stdout)
	case "install":
		return runInstall(args[1:], stdout, stderr)
	case "setup":
		return runSetup(stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "update":
		return runUpdate(args[1:], stdout, stderr)
	case "fix":
		return runFix(args[1:], stdout, stderr)
	case "logs":
		return runLogs(args[1:], stdout, stderr)
	case "rollback":
		return runRollback(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printHelp(stderr)
		return 2
	}
}

func runList(stdout io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	fmt.Fprintf(stdout, "Supported agents (%s):\n", platform)
	for _, agent := range agents.Supported() {
		status := agents.CheckAgent(platform, agent, func(name string) (string, error) { return "", agents.ErrNotFound }, nil)
		installable := "detect-only"
		if status.SupportsInstall {
			installable = "installable"
		}
		fmt.Fprintf(stdout, "  %-8s %-12s %s\n", agent.Name, installable, agent.Description)
	}
	return 0
}

func runStatus(stdout io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	fmt.Fprintf(stdout, "Agent status (%s):\n", platform)
	for _, status := range agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput) {
		flags := make([]string, 0, 2)
		if status.SupportsInstall {
			flags = append(flags, "install")
		}
		if status.SupportsUpdate {
			flags = append(flags, "update")
		}
		capabilityLabel := ""
		if len(flags) > 0 {
			capabilityLabel = " [" + strings.Join(flags, "/") + "]"
		}
		if status.State == "installed" {
			version := ""
			if status.Version != "" {
				version = "  " + status.Version
			}
			fmt.Fprintf(stdout, "  %-8s installed%s  %s%s\n", status.Name, capabilityLabel, status.Path, version)
			continue
		}
		fmt.Fprintf(stdout, "  %-8s missing%s\n", status.Name, capabilityLabel)
	}
	return 0
}

func runDoctor(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name == "" {
		return runStatus(stdout)
	}
	if name == "all" {
		return runDoctorAll(stdout, stderr)
	}
	if name == "openclaw" {
		return openClawDoctor(stdout, stderr)
	}
	agent, ok := agents.Find(name)
	if !ok {
		fmt.Fprintf(stderr, "unknown agent: %s\n", name)
		return 2
	}
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	support, ok := agent.Platforms[platform]
	if !ok || support.Doctor == nil {
		fmt.Fprintf(stderr, "doctor is not implemented for %q on %s\n", name, platform)
		return 2
	}
	fmt.Fprintf(stdout, "== %s doctor ==\n", titleCase(agent.Name))
	return runCommandSpec(stdout, stderr, 10*time.Minute, support.Doctor)
}

func runUpdate(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name == "" {
		fmt.Fprintf(stderr, "usage: agentctl update <agent|all>\n")
		return 2
	}
	if name == "all" {
		return runUpdateAll(stdout, stderr)
	}
	if name == "openclaw" {
		return openClawUpdate(stdout, stderr)
	}
	return runGenericAgentUpdate(name, stdout, stderr)
}

func runFix(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name != "openclaw" {
		fmt.Fprintf(stderr, "usage: agentctl fix openclaw\n")
		return 2
	}
	return openClawFix(stdout, stderr)
}

func runLogs(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name != "openclaw" {
		fmt.Fprintf(stderr, "usage: agentctl logs openclaw\n")
		return 2
	}
	return runLogged(stdout, stderr, 30*time.Second, "journalctl", "--user", "-u", "openclaw-gateway", "--since", "30 minutes ago", "--no-pager")
}

func runRollback(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name != "openclaw" {
		fmt.Fprintf(stderr, "usage: agentctl rollback openclaw\n")
		return 2
	}
	return openClawRollback(stdout, stderr)
}

func runInstall(args []string, stdout io.Writer, stderr io.Writer) int {
	name := agentName(args)
	if name == "" {
		fmt.Fprintf(stderr, "usage: agentctl install <agent|all>\n")
		return 2
	}
	if name == "all" {
		return installMissingAgents(stdout, stderr)
	}
	return installAgentByName(name, stdout, stderr)
}

func runSetup(stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stdout, "== Agent setup ==")
	return installMissingAgents(stdout, stderr)
}

func agentName(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(args[0]))
}

type openClawRollbackSnapshot struct {
	CreatedAt    string                 `json:"createdAt"`
	Version      string                 `json:"version,omitempty"`
	ConfigBackup string                 `json:"configBackup,omitempty"`
	PatchedFiles []openClawRollbackFile `json:"patchedFiles,omitempty"`
	SnapshotDir  string                 `json:"snapshotDir,omitempty"`
}

type openClawRollbackFile struct {
	TargetPath string `json:"targetPath"`
	BackupPath string `json:"backupPath"`
}

func installMissingAgents(stdout io.Writer, stderr io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	statuses := agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput)
	code := 0
	for _, status := range statuses {
		if status.State == "installed" {
			fmt.Fprintf(stdout, "skip: %s already installed", status.Name)
			if status.Version != "" {
				fmt.Fprintf(stdout, " (%s)", status.Version)
			}
			fmt.Fprintln(stdout)
			continue
		}
		if !status.SupportsInstall {
			fmt.Fprintf(stderr, "skip: %s is not auto-installable on %s\n", status.Name, platform)
			code = 1
			continue
		}
		if installAgentByName(status.Name, stdout, stderr) != 0 {
			code = 1
		}
	}
	return code
}

func installAgentByName(name string, stdout io.Writer, stderr io.Writer) int {
	agent, ok := agents.Find(name)
	if !ok {
		fmt.Fprintf(stderr, "unknown agent: %s\n", name)
		return 2
	}
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	support, ok := agent.Platforms[platform]
	if !ok || support.Install == nil {
		fmt.Fprintf(stderr, "install is not supported for %s on %s\n", agent.Name, platform)
		return 2
	}
	status := agents.CheckAgent(platform, agent, exec.LookPath, captureCommandOutput)
	if status.State == "installed" && status.Path != "" {
		fmt.Fprintf(stdout, "skip: %s already installed at %s\n", agent.Name, status.Path)
		return 0
	}

	fmt.Fprintf(stdout, "== Install %s ==\n", titleCase(agent.Name))
	code := runCommandSpec(stdout, stderr, 30*time.Minute, support.Install)
	if code != 0 {
		return code
	}
	status = agents.CheckAgent(platform, agent, exec.LookPath, captureCommandOutput)
	if status.State == "installed" && status.Path != "" {
		fmt.Fprintf(stdout, "installed: %s -> %s\n", agent.Name, status.Path)
	}
	if support.FirstRunHint != "" {
		fmt.Fprintf(stdout, "next: %s\n", support.FirstRunHint)
	}
	return 0
}

func runUpdateAll(stdout io.Writer, stderr io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	statuses := agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput)
	code := 0
	for _, status := range statuses {
		if status.State != "installed" {
			fmt.Fprintf(stdout, "skip: %s missing\n", status.Name)
			continue
		}
		if !status.SupportsUpdate {
			fmt.Fprintf(stdout, "skip: %s has no managed update path on %s\n", status.Name, platform)
			continue
		}
		if status.Name == "openclaw" {
			if openClawUpdate(stdout, stderr) != 0 {
				code = 1
			}
			continue
		}
		if runGenericAgentUpdate(status.Name, stdout, stderr) != 0 {
			code = 1
		}
	}
	return code
}

func runGenericAgentUpdate(name string, stdout io.Writer, stderr io.Writer) int {
	agent, ok := agents.Find(name)
	if !ok {
		fmt.Fprintf(stderr, "unknown agent: %s\n", name)
		return 2
	}
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	support, ok := agent.Platforms[platform]
	if !ok || support.Update == nil {
		fmt.Fprintf(stderr, "update is not supported for %s on %s\n", agent.Name, platform)
		return 2
	}
	status := agents.CheckAgent(platform, agent, exec.LookPath, captureCommandOutput)
	if status.State != "installed" || status.Path == "" {
		fmt.Fprintf(stderr, "%s is not installed\n", agent.Name)
		return 1
	}
	fmt.Fprintf(stdout, "== Update %s ==\n", titleCase(agent.Name))
	return runCommandSpec(stdout, stderr, 20*time.Minute, support.Update)
}

func runDoctorAll(stdout io.Writer, stderr io.Writer) int {
	platform := agents.PlatformFromGOOS(runtime.GOOS)
	statuses := agents.CheckAllForPlatform(platform, exec.LookPath, captureCommandOutput)
	code := 0
	for _, status := range statuses {
		if status.State != "installed" {
			fmt.Fprintf(stdout, "skip: %s missing\n", status.Name)
			continue
		}
		if status.Name == "openclaw" {
			if openClawDoctor(stdout, stderr) != 0 {
				code = 1
			}
			continue
		}
		agent, ok := agents.Find(status.Name)
		if !ok {
			code = 1
			continue
		}
		support, ok := agent.Platforms[platform]
		if !ok || support.Doctor == nil {
			fmt.Fprintf(stdout, "skip: %s has no managed doctor path on %s\n", agent.Name, platform)
			continue
		}
		fmt.Fprintf(stdout, "== %s doctor ==\n", titleCase(agent.Name))
		if runCommandSpec(stdout, stderr, 10*time.Minute, support.Doctor) != 0 {
			code = 1
		}
	}
	return code
}

func runCommandSpec(stdout io.Writer, stderr io.Writer, timeout time.Duration, spec *agents.CommandSpec) int {
	if spec == nil {
		fmt.Fprintln(stderr, "command is not configured")
		return 2
	}
	return runLogged(stdout, stderr, timeout, spec.Program, spec.Args...)
}

func titleCase(value string) string {
	if value == "" {
		return value
	}
	if len(value) == 1 {
		return strings.ToUpper(value)
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func openClawDoctor(stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stdout, "== OpenClaw version ==")
	code := runLogged(stdout, stderr, 30*time.Second, "openclaw", "--version")

	fmt.Fprintln(stdout, "\n== OpenClaw update status ==")
	if c := runLogged(stdout, stderr, 60*time.Second, "openclaw", "update", "status", "--json"); c != 0 {
		code = c
	}

	fmt.Fprintln(stdout, "\n== OpenClaw gateway systemd ==")
	_ = runLogged(stdout, stderr, 30*time.Second, "systemctl", "--user", "show", "openclaw-gateway", "-p", "MainPID", "-p", "NRestarts", "-p", "ActiveState", "-p", "SubState", "-p", "ExecMainStatus", "-p", "MemoryCurrent")

	fmt.Fprintln(stdout, "\n== OpenClaw gateway RPC ==")
	if c := runLoggedEnv(stdout, stderr, 75*time.Second, []string{"OPENCLAW_RPC_TIMEOUT=30000"}, "openclaw", "gateway", "status", "--json"); c != 0 {
		code = c
	} else if !openClawGatewayRpcOk() {
		fmt.Fprintln(stderr, "gateway RPC reported ok=false")
		code = 1
	}

	fmt.Fprintln(stdout, "\n== Recent actionable gateway log lines ==")
	_ = runShell(stdout, stderr, 45*time.Second, "journalctl --user -u openclaw-gateway --since '30 minutes ago' --no-pager | grep -Ei 'error|fail|timeout|reject|crash|stability|ciao|bonjour|probing|json5|Cannot find package|telegram|active=|queued=' | sed -E 's#bot[0-9]+:[^/ ]+#bot[REDACTED]#g' | tail -120 || true")
	return code
}

func openClawUpdate(stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stdout, "== Preflight ==")
	if c := openClawDoctor(stdout, stderr); c != 0 {
		fmt.Fprintln(stderr, "preflight reported issues; continuing with official updater, then fix/verify")
	}

	snapshot, c := createOpenClawRollbackSnapshot(stdout, stderr)
	if c != 0 {
		return 1
	}

	fmt.Fprintln(stdout, "\n== Official OpenClaw update ==")
	updateCode := runLogged(stdout, stderr, 25*time.Minute, "openclaw", "update", "--yes", "--json", "--timeout", "1200")
	if updateCode != 0 {
		fmt.Fprintln(stderr, "openclaw update exited non-zero; running repair/verification path before final judgment")
	}

	fixCode := openClawFix(stdout, stderr)
	if updateCode != 0 || fixCode != 0 {
		fmt.Fprintln(stderr, "\nupdate verification failed; attempting rollback to last known pre-update state")
		rollbackCode := restoreOpenClawRollback(snapshot, stdout, stderr)
		if rollbackCode != 0 {
			return rollbackCode
		}
		if updateCode != 0 {
			return updateCode
		}
		return fixCode
	}
	return fixCode
}

func openClawFix(stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stdout, "== OpenClaw targeted repair ==")

	// Frequent post-update failure: Telegram providers crash because the plugin
	// runtime cannot resolve package-level deps (json5/yaml) from OpenClaw's
	// generated runtime-deps tree. Patch both the installed bundle and staged
	// runtime bundle so the fix is present before Telegram providers load.
	if runtime, ok := latestOpenClawRuntimeDeps(); ok {
		fmt.Fprintf(stdout, "runtime deps: %s\n", runtime)
		if err := patchOpenClawFrontmatterImports(runtime); err != nil {
			fmt.Fprintf(stderr, "frontmatter runtime patch failed: %v\n", err)
			return 1
		}
		if !nodeCanImportFrontmatter(runtime) {
			fmt.Fprintln(stdout, "repair: frontmatter import still fails; ensuring json5 exists in runtime deps")
			if c := ensureOpenClawRuntimeJson5(runtime, stdout, stderr); c != 0 {
				return c
			}
			if err := patchOpenClawFrontmatterImports(runtime); err != nil {
				fmt.Fprintf(stderr, "frontmatter runtime patch failed after deps repair: %v\n", err)
				return 1
			}
		}
		fmt.Fprintln(stdout, "repair: staged frontmatter imports verified")
	}
	if err := patchOpenClawFrontmatterImports("/usr/lib/node_modules/openclaw"); err != nil {
		fmt.Fprintf(stderr, "frontmatter installed-bundle patch failed: %v\n", err)
		return 1
	}

	restartSince := time.Now().Add(-2 * time.Second).Format("2006-01-02 15:04:05")
	fmt.Fprintln(stdout, "repair: restarting openclaw-gateway")
	if c := runLogged(stdout, stderr, 2*time.Minute, "systemctl", "--user", "restart", "openclaw-gateway"); c != 0 {
		return c
	}
	fmt.Fprintln(stdout, "repair: waiting for startup")
	time.Sleep(75 * time.Second)

	if runtime, ok := latestOpenClawRuntimeDeps(); ok {
		if err := patchOpenClawFrontmatterImports(runtime); err != nil {
			fmt.Fprintf(stderr, "frontmatter post-start patch failed: %v\n", err)
			return 1
		}
		if !nodeCanImportFrontmatter(runtime) {
			fmt.Fprintln(stderr, "frontmatter bundle still cannot import after repair")
			return 1
		}
	}

	fmt.Fprintln(stdout, "\n== Post-repair verification ==")
	code := openClawDoctor(stdout, stderr)
	if recentGatewayLogMentions(restartSince, "Cannot find package 'json5'") || recentGatewayLogMentions(restartSince, "channel exited") {
		fmt.Fprintln(stderr, "post-restart gateway logs still show channel/import failures")
		return 1
	}
	return code
}

func backupOpenClawConfig(stdout io.Writer, stderr io.Writer) (string, int) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "cannot locate home directory: %v\n", err)
		return "", 1
	}
	src := filepath.Join(home, ".openclaw", "openclaw.json")
	if _, err := os.Stat(src); err != nil {
		fmt.Fprintf(stdout, "config backup skipped: %v\n", err)
		return "", 0
	}
	dir := filepath.Join(home, ".openclaw", "config-backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintf(stderr, "backup dir failed: %v\n", err)
		return "", 1
	}
	dst := filepath.Join(dir, "openclaw.json.bak.agentctl-"+time.Now().Format("20060102-150405"))
	data, err := os.ReadFile(src)
	if err != nil {
		fmt.Fprintf(stderr, "config backup read failed: %v\n", err)
		return "", 1
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		fmt.Fprintf(stderr, "config backup write failed: %v\n", err)
		return "", 1
	}
	fmt.Fprintf(stdout, "config backup: %s\n", dst)
	return dst, 0
}

func latestOpenClawRuntimeDeps() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	matches, err := filepath.Glob(filepath.Join(home, ".openclaw", "plugin-runtime-deps", "openclaw-*"))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	latest := matches[0]
	var latestTime time.Time
	for _, match := range matches {
		info, err := os.Stat(match)
		if err == nil && info.IsDir() && info.ModTime().After(latestTime) {
			latest = match
			latestTime = info.ModTime()
		}
	}
	return latest, true
}

func nodeCanResolve(path string, module string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "-e", "require.resolve(process.argv[1], {paths:[process.argv[2]]})", module, path)
	return cmd.Run() == nil
}

func nodeCanImportFrontmatter(runtime string) bool {
	frontmatter := filepath.Join(runtime, "dist", "frontmatter-Cc-V8aI2.js")
	if !fileExists(frontmatter) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "--input-type=module", "-e", "import(process.argv[1])", frontmatter)
	return cmd.Run() == nil
}

func openClawGatewayRpcOk() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "openclaw", "gateway", "status", "--json")
	cmd.Env = append(os.Environ(), "OPENCLAW_RPC_TIMEOUT=30000")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), `"rpc"`) && strings.Contains(string(out), `"ok": true`)
}

func patchOpenClawFrontmatterImports(root string) error {
	frontmatter := filepath.Join(root, "dist", "frontmatter-Cc-V8aI2.js")
	if !fileExists(frontmatter) {
		return fmt.Errorf("frontmatter bundle not found: %s", frontmatter)
	}
	json5Path := "/usr/lib/node_modules/openclaw/node_modules/json5/lib/index.js"
	yamlPath := "/usr/lib/node_modules/openclaw/node_modules/yaml/dist/index.js"
	if !fileExists(json5Path) {
		return fmt.Errorf("json5 package not found at %s", json5Path)
	}
	if !fileExists(yamlPath) {
		return fmt.Errorf("yaml package not found at %s", yamlPath)
	}
	data, err := os.ReadFile(frontmatter)
	if err != nil {
		return err
	}
	contents := string(data)
	next := strings.ReplaceAll(contents, "import JSON5 from \"json5\";", "import JSON5 from \"/usr/lib/node_modules/openclaw/node_modules/json5/lib/index.js\";")
	next = strings.ReplaceAll(next, "import YAML from \"yaml\";", "import YAML from \"/usr/lib/node_modules/openclaw/node_modules/yaml/dist/index.js\";")
	if next == contents {
		return nil
	}
	backup := frontmatter + ".agentctl-bak"
	if !fileExists(backup) {
		if err := os.WriteFile(backup, data, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(frontmatter, []byte(next), 0o644)
}

func ensureOpenClawRuntimeJson5(runtime string, stdout io.Writer, stderr io.Writer) int {
	if nodeCanResolve(runtime, "json5") {
		fmt.Fprintln(stdout, "repair: json5 already resolves")
		return 0
	}

	pnpmManaged := fileExists(filepath.Join(runtime, "pnpm-lock.yaml")) || fileExists(filepath.Join(runtime, "node_modules", ".modules.yaml"))
	if pnpmManaged {
		fmt.Fprintln(stdout, "repair: runtime deps are pnpm-managed; using local pnpm store/virtual-store")
		if c := runLogged(stdout, stderr, 10*time.Minute,
			"pnpm",
			"--dir", runtime,
			"--store-dir", filepath.Join(runtime, ".openclaw-pnpm-store"),
			"--virtual-store-dir", filepath.Join(runtime, ".pnpm"),
			"add", "json5@^2.2.3"); c != 0 {
			fmt.Fprintln(stderr, "pnpm add failed; trying direct symlink fallback if package exists in pnpm store")
		}
	} else {
		if c := runLogged(stdout, stderr, 5*time.Minute, "npm", "install", "--prefix", runtime, "json5@^2.2.3", "--omit=dev"); c != 0 {
			fmt.Fprintln(stderr, "npm install failed; trying direct symlink fallback if package exists")
		}
	}

	if nodeCanResolve(runtime, "json5") {
		fmt.Fprintln(stdout, "repair: json5 resolves after package-manager install")
		return 0
	}

	if err := symlinkJson5FromPnpmStore(runtime); err != nil {
		fmt.Fprintf(stderr, "json5 fallback symlink failed: %v\n", err)
		return 1
	}
	if !nodeCanResolve(runtime, "json5") {
		fmt.Fprintln(stderr, "json5 still does not resolve after repair")
		return 1
	}
	fmt.Fprintln(stdout, "repair: json5 resolves after symlink fallback")
	return 0
}

func symlinkJson5FromPnpmStore(runtime string) error {
	matches, err := filepath.Glob(filepath.Join(runtime, ".pnpm", "json5@*", "node_modules", "json5"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no json5 package found under %s", filepath.Join(runtime, ".pnpm"))
	}
	target := matches[len(matches)-1]
	nodeModules := filepath.Join(runtime, "node_modules")
	if err := os.MkdirAll(nodeModules, 0o755); err != nil {
		return err
	}
	link := filepath.Join(nodeModules, "json5")
	if err := os.RemoveAll(link); err != nil {
		return err
	}
	rel, err := filepath.Rel(nodeModules, target)
	if err != nil {
		rel = target
	}
	return os.Symlink(rel, link)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func logMentions(stdout io.Writer, stderr io.Writer, needle string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", "journalctl --user -u openclaw-gateway --since '2 hours ago' --no-pager | grep -F -- \"$0\" >/dev/null", needle)
	return cmd.Run() == nil
}

func recentGatewayLogMentions(since string, needle string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", "journalctl --user -u openclaw-gateway --since \"$0\" --no-pager | grep -F -- \"$1\" >/dev/null", since, needle)
	return cmd.Run() == nil
}

func runLogged(stdout io.Writer, stderr io.Writer, timeout time.Duration, name string, args ...string) int {
	return runLoggedEnv(stdout, stderr, timeout, nil, name, args...)
}

func captureCommandOutput(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runLoggedEnv(stdout io.Writer, stderr io.Writer, timeout time.Duration, env []string, name string, args ...string) int {
	fmt.Fprintf(stdout, "$ %s %s\n", name, strings.Join(args, " "))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Fprintf(stderr, "command timed out after %s: %s\n", timeout, name)
			return 124
		}
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintf(stderr, "command failed: %v\n", err)
		return 1
	}
	return 0
}

func runShell(stdout io.Writer, stderr io.Writer, timeout time.Duration, script string) int {
	return runLogged(stdout, stderr, timeout, "bash", "-lc", script)
}

func createOpenClawRollbackSnapshot(stdout io.Writer, stderr io.Writer) (*openClawRollbackSnapshot, int) {
	fmt.Fprintln(stdout, "\n== Rollback snapshot ==")
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "cannot locate home directory: %v\n", err)
		return nil, 1
	}
	root := filepath.Join(home, ".openclaw", "agentctl", "rollback")
	if err := os.MkdirAll(root, 0o700); err != nil {
		fmt.Fprintf(stderr, "rollback dir failed: %v\n", err)
		return nil, 1
	}
	snapshotDir := filepath.Join(root, "openclaw-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(snapshotDir, 0o700); err != nil {
		fmt.Fprintf(stderr, "snapshot dir failed: %v\n", err)
		return nil, 1
	}

	version, _ := captureOpenClawVersion()
	if version != "" {
		fmt.Fprintf(stdout, "version snapshot: %s\n", version)
	}
	configBackup, code := backupOpenClawConfig(stdout, stderr)
	if code != 0 {
		return nil, code
	}

	snapshot := &openClawRollbackSnapshot{
		CreatedAt:    time.Now().Format(time.RFC3339),
		Version:      version,
		ConfigBackup: configBackup,
		SnapshotDir:  snapshotDir,
	}

	targets := []string{
		filepath.Join("/usr/lib/node_modules/openclaw", "dist", "frontmatter-Cc-V8aI2.js"),
	}
	if runtime, ok := latestOpenClawRuntimeDeps(); ok {
		targets = append(targets, filepath.Join(runtime, "dist", "frontmatter-Cc-V8aI2.js"))
	}
	for _, target := range targets {
		entry, err := snapshotRollbackFile(snapshotDir, target)
		if err != nil {
			fmt.Fprintf(stderr, "snapshot file failed for %s: %v\n", target, err)
			return nil, 1
		}
		if entry.TargetPath != "" {
			snapshot.PatchedFiles = append(snapshot.PatchedFiles, entry)
			fmt.Fprintf(stdout, "file snapshot: %s\n", target)
		}
	}

	if err := writeOpenClawRollbackSnapshot(snapshot); err != nil {
		fmt.Fprintf(stderr, "snapshot metadata write failed: %v\n", err)
		return nil, 1
	}
	fmt.Fprintf(stdout, "rollback snapshot: %s\n", filepath.Join(snapshotDir, "metadata.json"))
	return snapshot, 0
}

func snapshotRollbackFile(snapshotDir string, target string) (openClawRollbackFile, error) {
	if !fileExists(target) {
		return openClawRollbackFile{}, nil
	}
	name := strings.ReplaceAll(strings.TrimPrefix(target, "/"), "/", "__")
	backupPath := filepath.Join(snapshotDir, name)
	data, err := os.ReadFile(target)
	if err != nil {
		return openClawRollbackFile{}, err
	}
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return openClawRollbackFile{}, err
	}
	return openClawRollbackFile{TargetPath: target, BackupPath: backupPath}, nil
}

func writeOpenClawRollbackSnapshot(snapshot *openClawRollbackSnapshot) error {
	if snapshot == nil || snapshot.SnapshotDir == "" {
		return fmt.Errorf("rollback snapshot is incomplete")
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	metadataPath := filepath.Join(snapshot.SnapshotDir, "metadata.json")
	if err := os.WriteFile(metadataPath, data, 0o600); err != nil {
		return err
	}
	latestPath := filepath.Join(filepath.Dir(snapshot.SnapshotDir), "latest-openclaw.json")
	return os.WriteFile(latestPath, data, 0o600)
}

func loadLatestOpenClawRollbackSnapshot() (*openClawRollbackSnapshot, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	latestPath := filepath.Join(home, ".openclaw", "agentctl", "rollback", "latest-openclaw.json")
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return nil, err
	}
	var snapshot openClawRollbackSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func captureOpenClawVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "openclaw", "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func restoreOpenClawRollback(snapshot *openClawRollbackSnapshot, stdout io.Writer, stderr io.Writer) int {
	if snapshot == nil {
		fmt.Fprintln(stderr, "rollback snapshot is missing")
		return 1
	}
	fmt.Fprintln(stdout, "\n== OpenClaw rollback ==")
	if snapshot.Version != "" {
		fmt.Fprintf(stdout, "snapshot version: %s\n", snapshot.Version)
	}
	if snapshot.ConfigBackup != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(stderr, "cannot locate home directory: %v\n", err)
			return 1
		}
		target := filepath.Join(home, ".openclaw", "openclaw.json")
		if err := restoreFileFromBackup(snapshot.ConfigBackup, target, 0o600); err != nil {
			fmt.Fprintf(stderr, "config restore failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "config restored: %s\n", target)
	}
	for _, patched := range snapshot.PatchedFiles {
		if err := restoreFileFromBackup(patched.BackupPath, patched.TargetPath, 0o644); err != nil {
			fmt.Fprintf(stderr, "file restore failed for %s: %v\n", patched.TargetPath, err)
			return 1
		}
		fmt.Fprintf(stdout, "file restored: %s\n", patched.TargetPath)
	}
	if c := runLogged(stdout, stderr, 2*time.Minute, "systemctl", "--user", "restart", "openclaw-gateway"); c != 0 {
		return c
	}
	fmt.Fprintln(stdout, "rollback: waiting for startup")
	time.Sleep(20 * time.Second)
	return openClawDoctor(stdout, stderr)
}

func restoreFileFromBackup(src string, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func openClawRollback(stdout io.Writer, stderr io.Writer) int {
	snapshot, err := loadLatestOpenClawRollbackSnapshot()
	if err != nil {
		fmt.Fprintf(stderr, "could not load latest rollback snapshot: %v\n", err)
		return 1
	}
	return restoreOpenClawRollback(snapshot, stdout, stderr)
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "agentctl - manage local AI agent tools")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  agentctl list")
	fmt.Fprintln(w, "  agentctl status")
	fmt.Fprintln(w, "  agentctl install <agent|all>")
	fmt.Fprintln(w, "  agentctl setup")
	fmt.Fprintln(w, "  agentctl doctor <agent|all>")
	fmt.Fprintln(w, "  agentctl update <agent|all>")
	fmt.Fprintln(w, "  agentctl fix openclaw")
	fmt.Fprintln(w, "  agentctl logs openclaw")
	fmt.Fprintln(w, "  agentctl rollback openclaw")
}
