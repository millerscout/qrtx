package cli

import (
	"encoding/json"
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

func getJSON(t *testing.T, ref string) qrtx.Issue {
	t.Helper()
	out, _, err := run(t, "tracker", "get", ref, "--json")
	if err != nil {
		t.Fatalf("get %s: %v", ref, err)
	}
	var is qrtx.Issue
	if err := json.Unmarshal([]byte(out), &is); err != nil {
		t.Fatalf("get %s: %v\n%s", ref, err, out)
	}
	return is
}

func TestTrackerLink(t *testing.T) {
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	a := addIssue(t, "Rename")
	b := addIssue(t, "Release")
	bare := strings.TrimPrefix(b, "file:")
	for i := 0; i < 2; i++ {
		if out, _, err := run(t, "tracker", "link", a, "blocks", b); err != nil || !strings.Contains(out, "linked") {
			t.Fatalf("link %d: %q %v", i, out, err)
		}
	}
	got := getJSON(t, a)
	if len(got.Links) != 1 || got.Links[0].Type != "blocks" || got.Links[0].ID != bare {
		t.Fatalf("links after two link calls: %+v", got.Links)
	}
	if out, _, _ := run(t, "tracker", "get", a); !strings.Contains(out, "blocks "+b) {
		t.Fatalf("get text should list the link:\n%s", out)
	}
	if out, _, err := run(t, "tracker", "unlink", a, "blocks", b); err != nil || !strings.Contains(out, "unlinked") {
		t.Fatalf("unlink: %q %v", out, err)
	}
	if got := getJSON(t, a); len(got.Links) != 0 {
		t.Fatalf("links after unlink: %+v", got.Links)
	}
	if _, _, err := run(t, "tracker", "link", a, "precedes", b); err == nil || !strings.Contains(err.Error(), "blocks") {
		t.Fatalf("unknown link type: %v", err)
	}
	if _, _, err := run(t, "tracker", "link", a, "blocks", "github:7"); err == nil || !strings.Contains(err.Error(), "same backend") {
		t.Fatalf("cross-backend link: %v", err)
	}
	if _, _, err := run(t, "tracker", "link", a, "blocks", a); err == nil {
		t.Fatal("an issue cannot block itself")
	}
}

func TestTrackerPriority(t *testing.T) {
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	out, _, err := run(t, "tracker", "add", "Urgent thing", "--priority", "1")
	if err != nil || !strings.HasPrefix(out, "added file:sdd-") {
		t.Fatalf("add --priority: %q %v", out, err)
	}
	ref := strings.TrimSpace(strings.TrimPrefix(out, "added "))
	if got := getJSON(t, ref); got.Title != "Urgent thing" || got.Priority == nil || *got.Priority != 1 {
		t.Fatalf("add --priority: %+v", got)
	}
	if out, _, err := run(t, "tracker", "priority", ref, "3"); err != nil || !strings.Contains(out, "3") {
		t.Fatalf("priority: %q %v", out, err)
	}
	if got := getJSON(t, ref); got.Priority == nil || *got.Priority != 3 {
		t.Fatalf("priority: %+v", got)
	}
	if out, _, _ := run(t, "tracker", "get", ref); !strings.Contains(out, "priority: 3") {
		t.Fatalf("get text should show priority:\n%s", out)
	}
	for _, bad := range []string{"5", "-1", "high"} {
		if _, _, err := run(t, "tracker", "priority", ref, bad); err == nil {
			t.Fatalf("priority %s accepted", bad)
		}
	}
	if _, _, err := run(t, "tracker", "add", "Bad", "--priority", "9"); err == nil {
		t.Fatal("add --priority 9 accepted")
	}
}

// S2: link and priority on the GitHub and Jira providers, through the plugin
// binaries and their fake APIs.
func TestTrackerLinkProviders(t *testing.T) {
	pluginPath(t, "qrtx-tracker-github", "qrtx-tracker-jira")
	gh := ghfake.New("o/r")
	defer gh.Close()
	jr := jirafake.New("SHOP")
	defer jr.Close()
	ghu, _ := url.Parse(gh.URL)
	jru, _ := url.Parse(jr.URL)
	creds := filepath.Join(t.TempDir(), "credentials.yaml")
	_ = os.WriteFile(creds, []byte(fmt.Sprintf("hosts:\n  %s: {token_env: T_GH, scopes: [o/r]}\n  %s: {email_env: T_EMAIL, token_env: T_JIRA, scopes: [SHOP]}\n", ghu.Host, jru.Host)), 0o600)
	t.Setenv("QRTX_CREDENTIALS", creds)
	t.Setenv("T_GH", "tok")
	t.Setenv("T_EMAIL", "me@example.com")
	t.Setenv("T_JIRA", "tok")
	for _, tc := range []struct{ backend, config string }{
		{"github", fmt.Sprintf("{repo: o/r, api_url: %q}", gh.URL)},
		{"jira", fmt.Sprintf("{url: %q, project: SHOP}", jr.URL)},
	} {
		t.Run(tc.backend, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv(qrtx.EnvRepo, root)
			_ = os.WriteFile(filepath.Join(root, ".sdd.yaml"), []byte(fmt.Sprintf("tracker:\n  backend: %s\n  config: %s\n", tc.backend, tc.config)), 0o644)
			a := addIssue(t, "Rename")
			b := addIssue(t, "Release")
			if out, _, err := run(t, "tracker", "link", a, "blocks", b); err != nil {
				t.Fatalf("link: %q %v", out, err)
			}
			if out, _, err := run(t, "tracker", "priority", b, "2"); err != nil {
				t.Fatalf("priority: %q %v", out, err)
			}
			ga, gb := getJSON(t, a), getJSON(t, b)
			if len(ga.Links) != 1 || ga.Links[0].Type != "blocks" || ga.Links[0].ID != strings.TrimPrefix(b, tc.backend+":") {
				t.Fatalf("get A should list the link: %+v", ga.Links)
			}
			if gb.Priority == nil || *gb.Priority != 2 {
				t.Fatalf("get B should show priority 2: %+v", gb.Priority)
			}
			if out, _, _ := run(t, "tracker", "get", a); !strings.Contains(out, "blocks "+b) {
				t.Fatalf("get text:\n%s", out)
			}
		})
	}
}

// S8: a plugin that declares update_issue but drops the new fields.
func TestTrackerLinkDropped(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("needs sh")
	}
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	bin := t.TempDir()
	script := `#!/bin/sh
cat >/dev/null
case "$1" in
capabilities) echo '{"result":{"protocols":["sdd.adapter/v1"],"ops":["capabilities","get_issue","update_issue"]}}' ;;
*) echo '{"result":{"id":"X-1","title":"t","status":{"category":"backlog"}}}' ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "qrtx-tracker-dropper"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = os.WriteFile(filepath.Join(root, ".sdd.yaml"), []byte("tracker:\n  backend: dropper\n"), 0o644)
	if _, _, err := run(t, "tracker", "link", "X-1", "blocks", "X-2"); err == nil || !strings.Contains(err.Error(), "did not store") {
		t.Fatalf("link: want 'did not store', got %v", err)
	}
	if _, _, err := run(t, "tracker", "priority", "X-1", "2"); err == nil || !strings.Contains(err.Error(), "did not store") {
		t.Fatalf("priority: want 'did not store', got %v", err)
	}
}

// S3, S4, S5: start refuses a blocked issue, --force overrides, closing the
// blocker lifts it.
func TestTrackerStartBlocked(t *testing.T) {
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	a := addIssue(t, "Rename")
	b := addIssue(t, "Release")
	if _, _, err := run(t, "tracker", "link", a, "blocks", b); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "tracker", "start", a, "a-slug"); err != nil {
		t.Fatalf("the blocker itself starts: %v", err)
	}
	_, _, err := run(t, "tracker", "start", b, "b-slug")
	if err == nil || !strings.Contains(err.Error(), a) {
		t.Fatalf("S3: want a refusal naming %s: %v", a, err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "sdd", "changes", "b-slug")); statErr == nil {
		t.Fatal("S3: a refused start created the change")
	}
	out, _, err := run(t, "tracker", "start", b, "b-slug", "--force")
	if err != nil || !strings.Contains(out, "forced") || !strings.Contains(out, a) {
		t.Fatalf("S4: %q %v", out, err)
	}
	for _, closeFlag := range []string{"", "--canceled"} {
		slug := "c-slug" + strings.TrimPrefix(closeFlag, "--")
		c := addIssue(t, "Other"+closeFlag)
		d := addIssue(t, "Dependent"+closeFlag)
		if _, _, err := run(t, "tracker", "link", c, "blocks", d); err != nil {
			t.Fatal(err)
		}
		args := []string{"tracker", "close", c}
		if closeFlag != "" {
			args = append(args, closeFlag)
		}
		if _, _, err := run(t, args...); err != nil {
			t.Fatal(err)
		}
		if out, _, err := run(t, "tracker", "start", d, slug); err != nil || strings.Contains(out, "forced") {
			t.Fatalf("S5 (%q): %q %v", closeFlag, out, err)
		}
	}
}

// S6: the backlog holds B (blocked by open A), C (priority 1) and D.
func TestTrackerListPrecedence(t *testing.T) {
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	a := addIssue(t, "Rename")
	b := addIssue(t, "Release")
	c := addIssue(t, "Urgent")
	d := addIssue(t, "Someday")
	if _, _, err := run(t, "tracker", "link", a, "blocks", b); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "tracker", "priority", c, "1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "tracker", "priority", d, "0"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "tracker", "close", a, "--canceled"); err != nil { // A closed: B is free
		t.Fatal(err)
	}
	if out, _, _ := run(t, "tracker", "list"); strings.Contains(out, "blocked by") {
		t.Fatalf("a closed blocker must not show:\n%s", out)
	}
	if _, _, err := run(t, "tracker", "start", a, "a-slug", "--force"); err != nil { // reopen A as started
		t.Fatal(err)
	}
	if _, _, err := run(t, "tracker", "list"); err != nil { // drains the outbox: A is started
		t.Fatal(err)
	}

	check := func(name string, args ...string) {
		t.Helper()
		out, _, err := run(t, args...)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		pos := map[string]int{}
		for i, l := range lines {
			pos[strings.Fields(l)[0]] = i
			if strings.HasPrefix(l, b+"\t") && !strings.HasSuffix(l, "blocked by "+a) {
				t.Fatalf("%s: B's line must end with 'blocked by %s': %q", name, a, l)
			}
			if !strings.HasPrefix(l, b+"\t") && strings.Contains(l, "blocked by") {
				t.Fatalf("%s: only B is blocked: %q", name, l)
			}
		}
		if _, ok := pos[b]; !ok {
			t.Fatalf("%s: B missing:\n%s", name, out)
		}
		if pos[c] != 0 {
			t.Fatalf("%s: C (priority 1) must come first:\n%s", name, out)
		}
		if pos[d] < pos[c] {
			t.Fatalf("%s: D after the prioritised items:\n%s", name, out)
		}
	}
	check("default", "tracker", "list")
	check("--all", "tracker", "list", "--all")

	// With --status backlog, A (started) is filtered out and B still shows it.
	out, _, err := run(t, "tracker", "list", "--status", "backlog")
	if err != nil || strings.Contains(out, a+"\t") || !strings.Contains(out, "blocked by "+a) {
		t.Fatalf("--status backlog: %v\n%s", err, out)
	}
	first := strings.Fields(strings.Split(strings.TrimSpace(out), "\n")[0])[0]
	if first != c {
		t.Fatalf("--status backlog: C first, got %s\n%s", first, out)
	}

	// --json keeps the issue schema and the same order.
	out, _, err = run(t, "tracker", "list", "--json")
	var list []qrtx.Issue
	if err != nil || json.Unmarshal([]byte(out), &list) != nil || len(list) != 4 || "file:"+list[0].ID != c {
		t.Fatalf("--json: %v\n%s", err, out)
	}
	if strings.Contains(out, "blocked_by") {
		t.Fatal("--json must not grow a blocked_by field")
	}
}
