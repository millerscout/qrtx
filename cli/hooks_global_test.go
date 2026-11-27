package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/millerscout/qrtx"
)

// chdir moves the test process to dir until the test ends, so nothing
// resolves to this repository (a qrtx project with live sync state).
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func clearRepoEnv(t *testing.T) {
	t.Helper()
	for _, name := range qrtx.RepoEnv {
		t.Setenv(name, "")
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
}

func hookJSON(session, cwd string) string {
	b, _ := json.Marshal(map[string]string{"session_id": session, "cwd": cwd, "hook_event_name": "UserPromptSubmit"})
	return string(b)
}

var hookCommands = [][]string{
	{"sync", "heartbeat", "--hook"},
	{"sync", "begin", "--hook"},
	{"sync", "inbox", "--hook", "--brief"},
	{"sync", "end", "--hook"},
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// S4: hook-mode commands outside a qrtx project print nothing and create nothing.
func TestSyncHookOutsideProject(t *testing.T) {
	clearRepoEnv(t)
	envFile := filepath.Join(t.TempDir(), "env")
	_ = os.WriteFile(envFile, nil, 0o644)
	t.Setenv("CLAUDE_ENV_FILE", envFile)
	neutral := t.TempDir()
	chdir(t, neutral)

	empty := t.TempDir()
	plainGit := t.TempDir()
	gitInit(t, plainGit)
	_ = os.WriteFile(filepath.Join(plainGit, "go.mod"), []byte("module example.com/x\n"), 0o644)
	project := t.TempDir()
	_ = os.WriteFile(filepath.Join(project, qrtx.RepoConfigFile), nil, 0o644)

	cases := []struct {
		name, cwd, repoEnv string
	}{
		{"empty dir", empty, ""},
		{"plain git repo", plainGit, ""},
		{"QRTX_REPO set, cwd unrelated", empty, project},
	}
	for _, c := range cases {
		t.Setenv(qrtx.EnvRepo, c.repoEnv)
		for _, args := range hookCommands {
			withHookStdin(t, hookJSON("abc", c.cwd))
			out, errOut, err := run(t, args...)
			if err != nil || out != "" || errOut != "" {
				t.Fatalf("%s: %v: out %q err %q %v", c.name, args, out, errOut, err)
			}
		}
		for _, dir := range []string{c.cwd, neutral, project} {
			for _, state := range []string{".sdd", filepath.Join(".git", "sdd")} {
				if exists(filepath.Join(dir, state)) {
					t.Fatalf("%s: %s created in %s", c.name, state, dir)
				}
			}
		}
	}
	if b, _ := os.ReadFile(envFile); len(b) != 0 {
		t.Fatalf("CLAUDE_ENV_FILE written: %q", b)
	}
}

// S5: in a qrtx project the hooks work as before.
func TestSyncHookInProject(t *testing.T) {
	clearRepoEnv(t)
	chdir(t, t.TempDir())
	withYAML := t.TempDir()
	gitInit(t, withYAML)
	_ = os.WriteFile(filepath.Join(withYAML, qrtx.RepoConfigFile), nil, 0o644)
	withChanges := t.TempDir()
	gitInit(t, withChanges)
	_ = os.MkdirAll(filepath.Join(withChanges, "sdd", "changes"), 0o755)

	for _, proj := range []string{withYAML, withChanges} {
		envFile := filepath.Join(t.TempDir(), "env")
		t.Setenv("CLAUDE_ENV_FILE", envFile)
		withHookStdin(t, hookJSON("abc", proj))
		if out, _, err := run(t, "sync", "heartbeat", "--hook"); err != nil || !strings.Contains(out, "abc: lease until") {
			t.Fatalf("%s heartbeat: %q %v", proj, out, err)
		}
		withHookStdin(t, hookJSON("abc", proj))
		if _, _, err := run(t, "sync", "begin", "--hook"); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(envFile); !strings.Contains(string(b), "export QRTX_SESSION=abc") {
			t.Fatalf("%s: env file %q", proj, b)
		}
		if !exists(filepath.Join(proj, ".git", "sdd", "state.json")) {
			t.Fatalf("%s: no state.json", proj)
		}
		t.Setenv(qrtx.EnvRepo, proj)
		out, _, _ := run(t, "sync", "status", "--json")
		t.Setenv(qrtx.EnvRepo, "")
		var st qrtx.SyncStatus
		if err := json.Unmarshal([]byte(out), &st); err != nil || len(st.Sessions) != 1 || st.Sessions[0].ID != "abc" || !st.Sessions[0].Live {
			t.Fatalf("%s status %s %v", proj, out, err)
		}
	}
}

// S6: without --hook, sync keeps working anywhere.
func TestSyncNonHookOutsideProject(t *testing.T) {
	clearRepoEnv(t)
	dir := t.TempDir()
	chdir(t, dir)
	out, _, err := run(t, "sync", "heartbeat", "--session", "x")
	if err != nil || !strings.Contains(out, "x: lease until") {
		t.Fatalf("%q %v", out, err)
	}
	if !exists(filepath.Join(dir, ".sdd", "state.json")) {
		t.Fatal("non-hook heartbeat should still create state")
	}
}

// fakeQrtxOnPath puts an executable named qrtx first on PATH, so hookCommand
// returns the bare name.
func fakeQrtxOnPath(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, qrtx.CLIName), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// S1, S2, S7, S8: --global writes the user settings file only.
func TestHooksGlobal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell-script stand-in for qrtx")
	}
	clearRepoEnv(t)
	fakeQrtxOnPath(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	proj, _ := filepath.EvalSymlinks(t.TempDir()) // RepoRoot reports the cwd's real path
	_ = os.WriteFile(filepath.Join(proj, qrtx.RepoConfigFile), nil, 0o644)
	projSettings := filepath.Join(proj, ".claude", "settings.local.json")
	_ = os.MkdirAll(filepath.Dir(projSettings), 0o755)
	projOrig := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"\"/x/qrtx\" sync heartbeat --hook --quiet"}]}]}}`
	_ = os.WriteFile(projSettings, []byte(projOrig), 0o644)
	chdir(t, proj)

	global := filepath.Join(home, ".claude", "settings.json")
	_ = os.MkdirAll(filepath.Dir(global), 0o755)
	orig := `{"permissions":{"allow":["Bash(ls:*)"]},"hooks":{` +
		`"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo pre"}]}],` +
		`"SessionStart":[{"hooks":[{"type":"command","command":"sdd sync begin --hook"},{"type":"command","command":"sdd change list"}]}],` +
		`"UserPromptSubmit":[{"hooks":[{"type":"command","command":"\"/Users/me/go/bin/sdd\" sync heartbeat --hook --quiet"}]}]}}`
	_ = os.WriteFile(global, []byte(orig), 0o644)

	out, _, err := run(t, "skills", "install", "--hooks", "--global")
	if err != nil || !strings.Contains(out, "updated "+global+" (qrtx sync hooks)") {
		t.Fatalf("install: %q %v", out, err)
	}
	if !strings.Contains(out, "note: "+projSettings) {
		t.Fatalf("no note about the project's sync hooks: %q", out)
	}
	raw, _ := os.ReadFile(global)
	got := string(raw)
	for _, want := range []string{`"permissions"`, "echo pre", "sdd change list", `"sdd sync begin --hook"`, "/Users/me/go/bin/sdd", "qrtx sync begin --hook", "qrtx sync heartbeat --hook --quiet", "qrtx sync inbox --hook --brief", "qrtx sync end --hook"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s:\n%s", want, got)
		}
	}
	if b, _ := os.ReadFile(projSettings); string(b) != projOrig {
		t.Fatalf("project settings edited:\n%s", b)
	}
	for _, p := range []string{filepath.Join(proj, ".claude", "skills"), filepath.Join(proj, ".claude", "settings.json"), filepath.Join(proj, ".gitignore")} {
		if exists(p) {
			t.Fatalf("--global wrote %s", p)
		}
	}

	out, _, err = run(t, "skills", "install", "--hooks", "--global")
	if err != nil || !strings.Contains(out, "unchanged "+global+" (qrtx sync hooks)") {
		t.Fatalf("re-run: %q %v", out, err)
	}
	if again, _ := os.ReadFile(global); string(again) != got {
		t.Fatal("re-run rewrote the file")
	}

	// CLAUDE_CONFIG_DIR wins; a project without sync hooks gets no note.
	clean := t.TempDir()
	chdir(t, clean)
	cfg := filepath.Join(t.TempDir(), "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	out, _, err = run(t, "skills", "install", "--hooks", "--global")
	if err != nil || !strings.Contains(out, "updated "+filepath.Join(cfg, "settings.json")) || strings.Contains(out, "note:") {
		t.Fatalf("CLAUDE_CONFIG_DIR: %q %v", out, err)
	}
}

// S2, S3: conflicting flags and a temporary executable are refused.
func TestHooksGlobalRefusals(t *testing.T) {
	clearRepoEnv(t)
	chdir(t, t.TempDir())
	cfg := filepath.Join(t.TempDir(), "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	settings := filepath.Join(cfg, "settings.json")

	if runtime.GOOS != "windows" {
		fakeQrtxOnPath(t)
	}
	for _, args := range [][]string{
		{"skills", "install", "--hooks", "--global", "--shared"},
		{"skills", "install", "--hooks", "--global", "--dir", "x"},
		{"skills", "install", "--global"},
	} {
		if _, _, err := run(t, args...); err == nil || !strings.Contains(err.Error(), "--global") {
			t.Fatalf("%v: want an error naming --global, got %v", args, err)
		}
		if exists(settings) {
			t.Fatalf("%v wrote %s", args, settings)
		}
	}

	// The test binary lives in a go-build temp dir, like a `go run` build.
	t.Setenv("PATH", t.TempDir())
	_, _, err := run(t, "skills", "install", "--hooks", "--global")
	if err == nil || !strings.Contains(err.Error(), "go install github.com/millerscout/qrtx/cmd/qrtx@latest") {
		t.Fatalf("temp executable: %v", err)
	}
	if exists(settings) {
		t.Fatalf("temp executable wrote %s", settings)
	}
}
