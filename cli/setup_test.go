package cli

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/millerscout/qrtx"
	ghfake "github.com/millerscout/qrtx/providers/github/fakeapi"
	jirafake "github.com/millerscout/qrtx/providers/jira/fakeapi"
)

func tempCreds(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sdd", "credentials.yaml")
	t.Setenv("QRTX_CREDENTIALS", p)
	return p
}

func noTerminal(t *testing.T) {
	t.Helper()
	prev, prevIn := isTerminal, promptIn
	isTerminal = func() bool { return false }
	t.Cleanup(func() { isTerminal, promptIn, promptReader = prev, prevIn, nil })
}

func pluginPath(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		buildBin(t, n)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestTrackerSetup(t *testing.T) {
	noTerminal(t)
	pluginPath(t, "qrtx-tracker-github")
	fake := ghfake.New("o/r")
	defer fake.Close()
	u, _ := url.Parse(fake.URL)
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	creds := tempCreds(t)
	_ = os.WriteFile(filepath.Join(root, ".sdd.yaml"), []byte("# team\nproject: shop\n"), 0o644)
	args := []string{"tracker", "setup", "github", "--repo", "o/r", "--api-url", fake.URL, "--token-command", "printf tok", "--yes"}
	out, _, err := run(t, args...)
	if err != nil {
		t.Fatalf("setup: %v\n%s", err, out)
	}
	cfgRaw, _ := os.ReadFile(filepath.Join(root, ".sdd.yaml"))
	for _, want := range []string{"# team", "project: shop", "backend: github", "repo: o/r"} {
		if !strings.Contains(string(cfgRaw), want) {
			t.Fatalf(".sdd.yaml missing %q:\n%s", want, cfgRaw)
		}
	}
	credRaw, _ := os.ReadFile(creds)
	if !strings.Contains(string(credRaw), u.Host) || !strings.Contains(string(credRaw), "token_command: printf tok") || !strings.Contains(string(credRaw), "o/r") {
		t.Fatalf("credentials:\n%s", credRaw)
	}
	if strings.Contains(string(credRaw), "token:") {
		t.Fatalf("token value written:\n%s", credRaw)
	}
	if !strings.Contains(out, "pass capabilities") || !strings.Contains(out, "pass list_issues") {
		t.Fatalf("check missing:\n%s", out)
	}
	// Re-run: nothing changes.
	out, _, err = run(t, args...)
	cfg2, _ := os.ReadFile(filepath.Join(root, ".sdd.yaml"))
	cred2, _ := os.ReadFile(creds)
	if err != nil || !bytes.Equal(cfg2, cfgRaw) || !bytes.Equal(cred2, credRaw) || strings.Contains(out, "updated") {
		t.Fatalf("re-run changed something: %v\n%s", err, out)
	}
	// A second repo on the same host merges its scope.
	fake.Mu.Lock()
	fake.Repo = "o/other"
	fake.Mu.Unlock()
	if _, _, err := run(t, "tracker", "setup", "github", "--repo", "o/other", "--api-url", fake.URL, "--token-command", "printf tok", "--yes"); err != nil {
		t.Fatal(err)
	}
	if cred3, _ := os.ReadFile(creds); !strings.Contains(string(cred3), "o/other") || !strings.Contains(string(cred3), "o/r") {
		t.Fatalf("scope not merged:\n%s", cred3)
	}
}

func TestTrackerSetupNoTerminal(t *testing.T) {
	noTerminal(t)
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	creds := tempCreds(t)
	if _, _, err := run(t, "tracker", "setup", "github", "--token-command", "x", "--yes"); err == nil || !strings.Contains(err.Error(), "--repo") {
		t.Fatalf("want missing --repo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".sdd.yaml")); err == nil {
		t.Fatal(".sdd.yaml written")
	}
	if _, err := os.Stat(creds); err == nil {
		t.Fatal("credentials written")
	}
}

func TestTrackerSetupDeclined(t *testing.T) {
	noTerminal(t)
	isTerminal = func() bool { return true }
	promptIn = strings.NewReader("n\n")
	promptReader = nil
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	creds := tempCreds(t)
	t.Setenv("PATH", t.TempDir()) // no plugin, no git
	out, _, err := run(t, "tracker", "setup", "github", "--repo", "o/r", "--token-command", "gh auth token")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := os.Stat(creds); err == nil {
		t.Fatal("declined binding was written")
	}
	if !strings.Contains(out, "not written") || !strings.Contains(out, "api.github.com") {
		t.Fatalf("no manual instructions:\n%s", out)
	}
}

func TestTrackerSetupMissingPlugin(t *testing.T) {
	noTerminal(t)
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	tempCreds(t)
	t.Setenv("PATH", t.TempDir())
	out, _, err := run(t, "tracker", "setup", "jira", "--url", "https://acme.atlassian.net", "--project", "SHOP", "--email-env", "E", "--token-env", "T", "--yes")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "go install github.com/millerscout/qrtx/cmd/qrtx-tracker-jira@latest") {
		t.Fatalf("no install line:\n%s", out)
	}
}

func TestTrackerInitUnchanged(t *testing.T) {
	repo := newGitRepo(t, nil)
	t.Setenv(qrtx.EnvRepo, repo)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if out, _, err := run(t, "tracker", "init"); err != nil || !strings.Contains(out, "registered") {
		t.Fatalf("%q %v", out, err)
	}
	if out, _, err := run(t, "tracker", "init"); err != nil || !strings.Contains(out, "unchanged") {
		t.Fatalf("second: %q %v", out, err)
	}
}

func snapshot(t *testing.T, root string, rel ...string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, r := range rel {
		b, _ := os.ReadFile(filepath.Join(root, r))
		m[r] = string(b)
	}
	return m
}

func TestInit(t *testing.T) {
	noTerminal(t)
	t.Run("fresh git repo, then re-run", func(t *testing.T) {
		repo := newGitRepo(t, nil)
		t.Setenv(qrtx.EnvRepo, repo)
		tempCreds(t)
		out, _, err := run(t, "init")
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		files := []string{".claude/skills/sdd-propose/SKILL.md", "sdd/constitution.md", ".gitattributes"}
		before := snapshot(t, repo, files...)
		for f, body := range before {
			if body == "" {
				t.Fatalf("%s missing\n%s", f, out)
			}
		}
		if drv := mustGit(t, repo, "config", "merge.sdd-issues.driver"); !strings.Contains(drv, "merge-driver") {
			t.Fatal("merge driver not registered")
		}
		out, _, err = run(t, "init")
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			// A warning (the test binary is not the qrtx on PATH) changes nothing.
			if !strings.HasPrefix(line, "unchanged") && !strings.HasPrefix(line, "warning:") {
				t.Fatalf("re-run changed something: %q\n%s", line, out)
			}
		}
		if after := snapshot(t, repo, files...); fmt.Sprint(after) != fmt.Sprint(before) {
			t.Fatal("re-run rewrote files")
		}
	})
	t.Run("listed constitution, outside git, edited skill", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv(qrtx.EnvRepo, root)
		_ = os.MkdirAll(filepath.Join(root, "docs"), 0o755)
		_ = os.WriteFile(filepath.Join(root, "docs", "rules.md"), []byte("## Rule 1: x\n"), 0o644)
		_ = os.WriteFile(filepath.Join(root, ".sdd.yaml"), []byte("constitution: [docs/rules.md]\n"), 0o644)
		_ = os.MkdirAll(filepath.Join(root, ".claude", "skills", "sdd-apply"), 0o755)
		_ = os.WriteFile(filepath.Join(root, ".claude", "skills", "sdd-apply", "SKILL.md"), []byte("mine\n"), 0o644)
		out, _, err := run(t, "init")
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		for _, want := range []string{"unchanged  constitution (docs/rules.md)", "skipped    issues.jsonl merge driver (not a git repository)", "skipped    skill sdd-apply (local edits", "created    skill sdd-propose"} {
			if !strings.Contains(out, want) {
				t.Fatalf("missing %q:\n%s", want, out)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "sdd", "constitution.md")); err == nil {
			t.Fatal("wrote a second constitution")
		}
	})
	t.Run("with tracker", func(t *testing.T) {
		pluginPath(t, "qrtx-tracker-github")
		fake := ghfake.New("o/r")
		defer fake.Close()
		repo := newGitRepo(t, nil)
		t.Setenv(qrtx.EnvRepo, repo)
		creds := tempCreds(t)
		args := []string{"init", "--tracker", "github", "--repo", "o/r", "--api-url", fake.URL, "--token-command", "printf tok", "--yes"}
		out, _, err := run(t, args...)
		if err != nil || !strings.Contains(out, "pass capabilities") {
			t.Fatalf("%v\n%s", err, out)
		}
		before := snapshot(t, repo, ".sdd.yaml", ".gitattributes")
		credBefore, _ := os.ReadFile(creds)
		if out, _, err = run(t, args...); err != nil || strings.Contains(out, "updated") || strings.Contains(out, "created") {
			t.Fatalf("re-run: %v\n%s", err, out)
		}
		credAfter, _ := os.ReadFile(creds)
		if fmt.Sprint(snapshot(t, repo, ".sdd.yaml", ".gitattributes")) != fmt.Sprint(before) || !bytes.Equal(credBefore, credAfter) {
			t.Fatal("re-run rewrote files")
		}
	})
}

func TestProviderBinaries(t *testing.T) {
	bin := qrtxBinary(t)
	pluginPath(t, "qrtx-tracker-github", "qrtx-tracker-jira")
	gh := ghfake.New("o/r")
	defer gh.Close()
	jr := jirafake.New("SHOP")
	defer jr.Close()
	ghu, _ := url.Parse(gh.URL)
	jru, _ := url.Parse(jr.URL)
	creds := filepath.Join(t.TempDir(), "credentials.yaml")
	_ = os.WriteFile(creds, []byte(fmt.Sprintf("hosts:\n  %s: {token_env: T_GH, scopes: [o/r]}\n  %s: {email_env: T_EMAIL, token_env: T_JIRA, scopes: [SHOP]}\n", ghu.Host, jru.Host)), 0o600)
	for _, tc := range []struct{ backend, config string }{
		{"github", fmt.Sprintf("{repo: o/r, api_url: %q}", gh.URL)},
		{"jira", fmt.Sprintf("{url: %q, project: SHOP}", jr.URL)},
	} {
		root := t.TempDir()
		_ = os.WriteFile(filepath.Join(root, ".sdd.yaml"), []byte(fmt.Sprintf("tracker:\n  backend: %s\n  config: %s\n", tc.backend, tc.config)), 0o644)
		cmd := exec.Command(bin, "tracker", "check", "--backend", tc.backend, "--allow-writes")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "QRTX_CHECK_RETRY_DELAY=1s", "QRTX_REPO="+root, "QRTX_CREDENTIALS="+creds, "T_GH=tok", "T_EMAIL=me@example.com", "T_JIRA=tok", "PATH="+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		if err != nil || strings.Contains(string(out), "FAIL") {
			t.Fatalf("%s: %v\n%s", tc.backend, err, out)
		}
		for _, op := range []string{"create_issue (idempotent)", "transition_issue (cleanup: canceled)", "comment"} {
			if !strings.Contains(string(out), "pass  "+op) {
				t.Fatalf("%s: no pass for %s\n%s", tc.backend, op, out)
			}
		}
	}
}
