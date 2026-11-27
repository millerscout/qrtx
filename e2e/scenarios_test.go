//go:build e2e && !windows

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// S3: SessionStart exports QRTX_SESSION, and the agent's own commands use it.
func TestSessionIdentity(t *testing.T) {
	r := newRepo(t, "1h", "")
	s := r.session("s-1", r.main)
	s.begin()
	if res := s.agent("qrtx sync claim demo/1.1"); res.code != 0 {
		t.Fatalf("claim: %d %s%s", res.code, res.out, res.errOut)
	}
	st := r.status()
	if len(st.Claims) != 1 || st.Claims[0].Session != "s-1" || st.Claims[0].Key != "demo/1.1" {
		t.Fatalf("claims %+v", st.Claims)
	}
}

// S4: worktrees of one project share claims; a subproject is separate.
func TestClaimsAcrossWorktrees(t *testing.T) {
	r := newRepo(t, "1h", "")
	a, b, c := r.session("A", r.main), r.session("B", r.wt), r.session("C", r.svc)
	a.begin()
	b.begin()
	c.begin()
	if res := a.agent("qrtx sync claim demo/1.1"); res.code != 0 {
		t.Fatalf("A: %s%s", res.out, res.errOut)
	}
	res := b.agent("qrtx sync claim demo/1.1")
	if res.code != 1 || !strings.Contains(res.errOut, "claimed by session A") {
		t.Fatalf("B should be refused naming A: %d %s%s", res.code, res.out, res.errOut)
	}
	if res := c.agent("qrtx sync claim demo/1.1"); res.code != 0 {
		t.Fatalf("subproject claim conflicted: %s%s", res.out, res.errOut)
	}
}

// S5: a task checked in worktree B reaches A's inbox.
func TestTaskWatchInbox(t *testing.T) {
	r := newRepo(t, "1h", "")
	a, b := r.session("A", r.main), r.session("B", r.wt)
	a.begin()
	b.begin()
	bFile := filepath.Join(r.wt, "sdd", "changes", "demo", "change.sdd.md")
	waitFor(t, "watch snapshot of B's change", 10*time.Second, func() bool { return r.watching(bFile) })
	a.hook("UserPromptSubmit", "inbox", "--brief")
	flipTask(t, r.wt)
	want := "demo/1.1 done in " + filepath.Base(r.wt) + " (feature)"
	var seen strings.Builder
	waitFor(t, "task progress in A's inbox", 5*time.Second, func() bool {
		seen.WriteString(a.hook("UserPromptSubmit", "inbox", "--brief").out)
		return strings.Contains(seen.String(), want)
	})
}

// S6: leases end, the daemon exits, a heartbeat restarts it, stop is fast.
func TestLeaseLifecycle(t *testing.T) {
	r := newRepo(t, "2s", "")
	a := r.session("A", r.main)
	a.begin()
	r.waitDaemon(true)
	r.waitDaemon(false)
	if log := mustRead(t, filepath.Join(r.stateDir(), "sync.log")); !strings.Contains(log, "exiting: no live sessions") {
		t.Fatalf("sync.log:\n%s", log)
	}
	b := r.session("B", r.main)
	b.hook("UserPromptSubmit", "heartbeat", "--quiet")
	r.waitDaemon(true)
	start := time.Now()
	r.mustQ(r.main, nil, "sync", "stop")
	if d := time.Since(start); d > time.Second {
		t.Fatalf("stop took %s", d)
	}
}

// S7: a write queued while the plugin is down is drained by the daemon.
func TestOutboxDrain(t *testing.T) {
	r := newRepo(t, "1h", "tracker:\n  backend: fake\n")
	r.write(r.fake, "mode", "down")
	// Start the session first: `sync status` reports the outbox only once
	// sync has state (filed for sync-hardening).
	a := r.session("A", r.main)
	a.begin()
	manifest := filepath.Join(r.main, "sdd", "changes", "demo", "change.sdd.md")
	raw := mustRead(t, manifest)
	r.write(filepath.Dir(manifest), "change.sdd.md", strings.Replace(raw, "(Optional — other slugs that own split acceptance.)", "- Export to CSV later.", 1))
	res := r.q(r.main, nil, "", "tracker", "from-deferred", "--change", "demo")
	if st := r.status(); st.Outbox != 1 {
		t.Fatalf("outbox %d after from-deferred: %d %s%s", st.Outbox, res.code, res.out, res.errOut)
	}
	r.write(r.fake, "mode", "up")
	waitFor(t, "daemon drain", 10*time.Second, func() bool { return r.status().Outbox == 0 })
	if log := mustRead(t, filepath.Join(r.fake, "log")); !strings.Contains(log, "create_issue") {
		t.Fatalf("fake never got the create:\n%s", log)
	}
}

// Known bug 1: a hook-started daemon under a minimal PATH cannot find the
// tracker plugin installed next to qrtx.
func TestPluginMinimalPath(t *testing.T) {
	knownBug(t, "the daemon resolves qrtx-tracker-* on PATH only")
	r := newRepo(t, "1h", "tracker:\n  backend: fake\n")
	r.write(r.fake, "state.json", `{"issues":[{"id":"F-1","title":"one","status":{"category":"unstarted"},"updated_at":"0"}]}`)
	r.write(r.fake, "mode", "bump")
	a := r.session("A", r.main)
	minimal := []string{"PATH=/usr/bin:/bin"}
	in, _ := json.Marshal(map[string]string{"session_id": "A", "cwd": r.main, "hook_event_name": "SessionStart"})
	r.q(r.main, append(minimal, "CLAUDE_ENV_FILE="+a.envFile), string(in), "sync", "begin", "--hook")
	deadline := time.Now().Add(10 * time.Second)
	for r.countEvents("remote_change", "") == 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	log := mustRead(t, filepath.Join(r.stateDir(), "sync.log"))
	if strings.Contains(log, "not found") || r.countEvents("remote_change", "") == 0 {
		t.Fatalf("no remote_change under a minimal PATH; sync.log:\n%s", log)
	}
}

// Known bug 2: task_progress is stamped with the scanning root, not the
// worktree the file is in.
func TestWatchAttribution(t *testing.T) {
	knownBug(t, "task_progress root is the scanning root")
	r := newRepo(t, "1h", "")
	a := r.session("A", r.main)
	a.begin()
	bFile := filepath.Join(r.wt, "sdd", "changes", "demo", "change.sdd.md")
	waitFor(t, "watch snapshot of B's change", 10*time.Second, func() bool { return r.watching(bFile) })
	flipTask(t, r.wt)
	waitFor(t, "task_progress", 5*time.Second, func() bool { return r.countEvents("task_progress", "") > 0 })
	for _, e := range r.events() {
		if e.Kind == "task_progress" && canonical(e.Root) != canonical(r.wt) {
			t.Fatalf("task_progress root %s, want worktree %s", e.Root, r.wt)
		}
	}
}

// Known bug 3: a claim stored under the old key format expires twice once
// the same session claims the key again.
func TestLegacyClaimMigration(t *testing.T) {
	knownBug(t, "legacy claim keys are never migrated")
	r := newRepo(t, "2s", "")
	a := r.session("A", r.main)
	a.begin()
	r.stopDaemon()
	statePath := filepath.Join(r.stateDir(), "state.json")
	var st map[string]any
	if err := json.Unmarshal([]byte(mustRead(t, statePath)), &st); err != nil {
		t.Fatal(err)
	}
	claims, _ := st["claims"].(map[string]any)
	if claims == nil {
		claims = map[string]any{}
	}
	claims[legacyClaimKey(r.main, "demo/1.1")] = map[string]any{"session": "A", "root": canonical(r.main), "key": "demo/1.1", "at": time.Now().UTC().Format(time.RFC3339)}
	st["claims"] = claims
	b, _ := json.Marshal(st)
	r.write(r.stateDir(), "state.json", string(b))
	if res := a.agent("qrtx sync claim demo/1.1"); res.code != 0 {
		t.Fatalf("re-claim: %s%s", res.out, res.errOut)
	}
	waitFor(t, "the claim to expire", 10*time.Second, func() bool { return r.countEvents("claim_expired", "demo/1.1") > 0 })
	time.Sleep(300 * time.Millisecond)
	if n := r.countEvents("claim_expired", "demo/1.1"); n != 1 {
		t.Fatalf("%d claim_expired events for one claim", n)
	}
}

// Known bug 4: a command without a session id records <path>:<pid> silently.
func TestUnattributedSessionWarns(t *testing.T) {
	knownBug(t, "the <path>:<pid> session fallback gives no warning")
	r := newRepo(t, "1h", "")
	a := r.session("A", r.main)
	a.begin()
	res := a.agent("unset QRTX_SESSION; qrtx sync claim demo/2.1")
	if !strings.Contains(res.errOut, "no session id") {
		t.Fatalf("no warning for an unattributed command: %q", res.errOut)
	}
	out := r.mustQ(r.main, []string{"QRTX_NO_DAEMON=1"}, "sync", "status")
	if !strings.Contains(out, "unattributed") {
		t.Fatalf("status does not mark the session:\n%s", out)
	}
}

func init() {
	// Keep the test binary from inheriting a developer's real session id.
	os.Unsetenv("QRTX_SESSION")
}
