//go:build e2e && !windows

// Package e2e drives the built qrtx binaries the way Claude Code does: hook
// commands get hook JSON on stdin with CLAUDE_ENV_FILE set, and agent
// commands run in a shell that sourced that env file. Run it with
//
//	go test -tags e2e ./e2e/
//
// Scenarios for known, unfixed bugs skip unless QRTX_E2E_KNOWN_BUGS=1.
package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var binDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "qrtx-e2e-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binDir = dir
	for name, pkg := range map[string]string{
		"qrtx":                "../cmd/qrtx",
		"qrtx-tracker-github": "../cmd/qrtx-tracker-github",
		"qrtx-tracker-fake":   "./fakeplugin",
	} {
		out, err := exec.Command("go", "build", "-o", filepath.Join(dir, name), pkg).CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build %s: %v\n%s", name, err, out)
			os.Exit(1)
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func qrtxBin() string { return filepath.Join(binDir, "qrtx") }

// knownBug skips a scenario for a bug sync-hardening fixes, unless
// QRTX_E2E_KNOWN_BUGS=1 asks to run it (it then fails until the fix lands).
func knownBug(t *testing.T, msg string) {
	t.Helper()
	if os.Getenv("QRTX_E2E_KNOWN_BUGS") != "1" {
		t.Skipf("known bug, fixed by sync-hardening: %s", msg)
	}
}

// repo is a qrtx project with a second worktree and a subproject.
type repo struct {
	t     *testing.T
	main  string // main worktree
	wt    string // second worktree, branch "feature"
	svc   string // subproject with its own .sdd.yaml
	home  string
	creds string
	fake  string // QRTX_FAKE_DIR
	extra []string
}

const syncYAML = "sync:\n  lease: %s\n  tick: 100ms\n  interval: 300ms\n"

// newRepo commits .sdd.yaml and a demo change before adding the worktree, so
// both checkouts are qrtx projects (hooks act only inside projects).
func newRepo(t *testing.T, lease string, trackerYAML string) *repo {
	t.Helper()
	base := t.TempDir()
	r := &repo{t: t, main: filepath.Join(base, "main"), wt: filepath.Join(base, "wt"),
		home: filepath.Join(base, "home"), fake: filepath.Join(base, "fake")}
	r.svc = filepath.Join(r.main, "svc")
	r.creds = filepath.Join(r.home, "credentials.yaml")
	for _, d := range []string{r.main, r.home, r.fake, r.svc} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	r.git(r.main, "init", "-q", "-b", "main")
	r.write(r.main, ".sdd.yaml", fmt.Sprintf(syncYAML, lease)+trackerYAML)
	r.write(r.svc, ".sdd.yaml", fmt.Sprintf(syncYAML, lease))
	r.mustQ(r.main, nil, "change", "new", "demo")
	r.git(r.main, "add", "-A")
	r.git(r.main, "commit", "-q", "-m", "demo")
	r.git(r.main, "worktree", "add", "-q", "-b", "feature", r.wt)
	t.Cleanup(r.stopDaemon)
	return r
}

func (r *repo) write(dir, name, body string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) git(dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = r.env(nil)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// env is an isolated environment: no CI or daemon switches, no inherited
// session or repo, a private HOME, credentials and Claude config dir.
func (r *repo) env(extra []string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k := kv[:strings.IndexByte(kv, '=')+1]
		switch {
		case k == "CI=", k == "QRTX_NO_DAEMON=", k == "QRTX_SESSION=", k == "QRTX_REPO=", k == "HOME=", k == "PATH=",
			strings.HasPrefix(k, "CLAUDE_"), strings.HasPrefix(k, "QRTX_"), strings.HasPrefix(k, "GIT_"):
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HOME="+r.home, "QRTX_CREDENTIALS="+r.creds, "CLAUDE_CONFIG_DIR="+r.home,
		"QRTX_FAKE_DIR="+r.fake, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_AUTHOR_NAME=e2e", "GIT_AUTHOR_EMAIL=e2e@example.com", "GIT_COMMITTER_NAME=e2e", "GIT_COMMITTER_EMAIL=e2e@example.com",
		"GIT_CONFIG_NOSYSTEM=1")
	env = append(env, r.extra...)
	return append(env, extra...)
}

type result struct {
	out, errOut string
	code        int
}

func (r *repo) q(dir string, extra []string, stdin string, args ...string) result {
	r.t.Helper()
	cmd := exec.Command(qrtxBin(), args...)
	cmd.Dir = dir
	cmd.Env = r.env(extra)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	res := result{out: out.String(), errOut: errOut.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		res.code = ee.ExitCode()
	} else if err != nil {
		r.t.Fatalf("qrtx %v: %v", args, err)
	}
	return res
}

func (r *repo) mustQ(dir string, extra []string, args ...string) string {
	r.t.Helper()
	res := r.q(dir, extra, "", args...)
	if res.code != 0 {
		r.t.Fatalf("qrtx %v: exit %d\n%s%s", args, res.code, res.out, res.errOut)
	}
	return res.out
}

// session is one Claude Code session: an id, a folder and its env file.
type session struct {
	r       *repo
	id, dir string
	envFile string
}

func (r *repo) session(id, dir string) *session {
	f := filepath.Join(r.t.TempDir(), "claude-env")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		r.t.Fatal(err)
	}
	return &session{r: r, id: id, dir: dir, envFile: f}
}

// hook runs `qrtx sync <sub> --hook <flags>` with Claude Code's hook JSON.
func (s *session) hook(event, sub string, flags ...string) result {
	s.r.t.Helper()
	in, _ := json.Marshal(map[string]string{"session_id": s.id, "cwd": s.dir, "hook_event_name": event})
	args := append([]string{"sync", sub, "--hook"}, flags...)
	return s.r.q(s.dir, []string{"CLAUDE_ENV_FILE=" + s.envFile}, string(in), args...)
}

func (s *session) begin() { s.hook("SessionStart", "begin") }

// agent runs a shell command the way an agent's Bash tool does: the env
// file the SessionStart hook wrote is sourced first.
func (s *session) agent(cmdline string, extra ...string) result {
	s.r.t.Helper()
	cmd := exec.Command("sh", "-c", ". "+shellQuote(s.envFile)+"; "+cmdline)
	cmd.Dir = s.dir
	cmd.Env = s.r.env(extra)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	res := result{out: out.String(), errOut: errOut.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		res.code = ee.ExitCode()
	}
	return res
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (r *repo) stateDir() string { return filepath.Join(r.main, ".git", "sdd") }

type statusJSON struct {
	Enabled bool `json:"enabled"`
	Daemon  *struct {
		PID int `json:"pid"`
	} `json:"daemon"`
	Sessions []struct {
		ID   string `json:"id"`
		Live bool   `json:"live"`
	} `json:"sessions"`
	Claims []struct {
		Session string `json:"session"`
		Key     string `json:"key"`
	} `json:"claims"`
	Outbox int `json:"outbox"`
}

func (r *repo) status() statusJSON {
	r.t.Helper()
	var st statusJSON
	out := r.mustQ(r.main, []string{"QRTX_NO_DAEMON=1"}, "sync", "status", "--json")
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		r.t.Fatalf("status json: %v\n%s", err, out)
	}
	return st
}

type event struct {
	Seq      int64  `json:"seq"`
	Session  string `json:"session"`
	Kind     string `json:"kind"`
	Root     string `json:"root"`
	Key      string `json:"key"`
	Change   string `json:"change"`
	Task     string `json:"task"`
	Worktree string `json:"worktree"`
}

func (r *repo) events() []event {
	raw, _ := os.ReadFile(filepath.Join(r.stateDir(), "events.jsonl"))
	var out []event
	for _, line := range strings.Split(string(raw), "\n") {
		var e event
		if json.Unmarshal([]byte(line), &e) == nil && e.Kind != "" {
			out = append(out, e)
		}
	}
	return out
}

func (r *repo) countEvents(kind, key string) int {
	n := 0
	for _, e := range r.events() {
		if e.Kind == kind && (key == "" || e.Key == key) {
			n++
		}
	}
	return n
}

func waitFor(t *testing.T, what string, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", d, what)
}

func pidAlive(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil }

func (r *repo) daemonPID() int {
	var info struct {
		PID int `json:"pid"`
	}
	b, err := os.ReadFile(filepath.Join(r.stateDir(), "daemon.json"))
	if err != nil || json.Unmarshal(b, &info) != nil {
		return 0
	}
	return info.PID
}

// stopDaemon stops any daemon the scenario started and fails if its pid
// survives.
func (r *repo) stopDaemon() {
	pid := r.daemonPID()
	r.q(r.main, nil, "", "sync", "stop")
	deadline := time.Now().Add(5 * time.Second)
	for pidAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if pidAlive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		r.t.Errorf("daemon pid %d still alive after sync stop", pid)
	}
}

func (r *repo) waitDaemon(up bool) {
	r.t.Helper()
	waitFor(r.t, fmt.Sprintf("daemon running=%v", up), 10*time.Second, func() bool { return (r.status().Daemon != nil) == up })
}

func canonical(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// legacyClaimKey is the pre-syncScope claim key: sha256(root)[:12] + "|" + key.
func legacyClaimKey(root, key string) string {
	sum := sha256.Sum256([]byte(canonical(root)))
	return hex.EncodeToString(sum[:])[:12] + "|" + key
}

func flipTask(t *testing.T, dir string) {
	t.Helper()
	p := filepath.Join(dir, "sdd", "changes", "demo", "change.sdd.md")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	next := strings.Replace(string(raw), "- [ ] 1.1 First implementation task", "- [x] 1.1 First implementation task", 1)
	if next == string(raw) {
		t.Fatal("task 1.1 not found to flip")
	}
	if err := os.WriteFile(p, []byte(next), 0o644); err != nil {
		t.Fatal(err)
	}
}

// watching reports whether the daemon has snapshotted path.
func (r *repo) watching(path string) bool {
	raw, _ := os.ReadFile(filepath.Join(r.stateDir(), "state.json"))
	return bytes.Contains(raw, []byte(canonical(path)))
}

func TestHarness(t *testing.T) {
	r := newRepo(t, "1h", "")
	for _, d := range []string{r.main, r.wt} {
		if _, err := os.Stat(filepath.Join(d, "sdd", "changes", "demo", "change.sdd.md")); err != nil {
			t.Fatalf("%s lacks the demo change: %v", d, err)
		}
	}
	a := r.session("A", r.main)
	a.begin()
	r.waitDaemon(true)
	if got := strings.TrimSpace(mustRead(t, a.envFile)); got != "export QRTX_SESSION=A" {
		t.Fatalf("env file %q", got)
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
