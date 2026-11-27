package qrtx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func who(id string) SessionInfo { return SessionInfo{ID: id, Worktree: "/wt/" + id, Branch: "b-" + id} }

func stateBytes(t *testing.T, root string) []byte {
	t.Helper()
	dir, _ := trackerStateDir(root)
	b, err := os.ReadFile(filepath.Join(dir, syncStateFile))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func kinds(evs []SyncEvent) string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Kind+":"+e.Key)
	}
	return strings.Join(out, ",")
}

func TestSyncState(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	// Reads create nothing where sync never ran.
	if evs, err := Inbox(root, who("B"), now, 0); err != nil || evs != nil {
		t.Fatalf("inbox before sync: %v %v", evs, err)
	}
	if st, err := ReadSyncStatus(root, now); err != nil || st.Enabled {
		t.Fatalf("status: %+v %v", st, err)
	}
	if err := EndSession(root, who("A"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".sdd")); err == nil {
		t.Fatal("a read created the state directory")
	}
	// A claims two keys, then its lease runs out.
	if _, err := Heartbeat(root, who("A"), now, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Heartbeat(root, who("B"), now, false); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"github:7", "demo/2.1"} {
		if err := Claim(root, who("A"), k, false, now, false); err != nil {
			t.Fatal(err)
		}
	}
	later := now.Add(2 * time.Hour)
	if _, err := Heartbeat(root, who("B"), later, false); err != nil {
		t.Fatal(err)
	}
	evs, err := Inbox(root, who("B"), later, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(evs); got != "claim:github:7,claim:demo/2.1,claim_expired:demo/2.1,claim_expired:github:7" {
		t.Fatalf("events %s", got)
	}
	if err := Claim(root, who("B"), "github:7", false, later, false); err != nil {
		t.Fatalf("B after expiry: %v", err)
	}
}

func TestSyncStateLockTimeout(t *testing.T) {
	root := t.TempDir()
	dir, _ := ensureTrackerStateDir(root)
	unlock, err := lockFile(filepath.Join(dir, syncLockName+".lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	start := time.Now()
	_, err = Heartbeat(root, who("A"), time.Now(), false)
	if err == nil || !strings.Contains(err.Error(), "busy") || time.Since(start) > 3*time.Second {
		t.Fatalf("hook lock wait: %v after %v", err, time.Since(start))
	}
}

func TestSyncHeartbeat(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	res, err := Heartbeat(root, who("A"), now, false)
	if err != nil || res.Debounced || res.LeaseUntil.Sub(now) != time.Hour {
		t.Fatalf("%+v %v", res, err)
	}
	before := stateBytes(t, root)
	res, err = Heartbeat(root, who("A"), now.Add(10*time.Second), false)
	if err != nil || !res.Debounced {
		t.Fatalf("second heartbeat not debounced: %+v %v", res, err)
	}
	if string(stateBytes(t, root)) != string(before) {
		t.Fatal("debounced heartbeat rewrote state.json")
	}
	// A short lease shortens the debounce.
	_ = os.WriteFile(filepath.Join(root, RepoConfigFile), []byte("sync:\n  lease: 8s\n"), 0o644)
	if res, _ := Heartbeat(root, who("A"), now.Add(3*time.Second), false); res.Debounced {
		t.Fatal("8s lease should debounce for 2s only")
	}
	// Session resolution.
	t.Setenv("QRTX_SESSION", "env-id")
	if got := ResolveSession(root, "", "hook-id").ID; got != "hook-id" {
		t.Fatalf("hook first: %s", got)
	}
	if got := ResolveSession(root, "", "").ID; got != "env-id" {
		t.Fatalf("env: %s", got)
	}
	if got := ResolveSession(root, "flag", "hook-id").ID; got != "flag" {
		t.Fatalf("flag: %s", got)
	}
	t.Setenv("QRTX_SESSION", "")
	if got := ResolveSession(root, "", "").ID; !strings.Contains(got, ":") {
		t.Fatalf("fallback: %s", got)
	}
}

func TestSyncEnd(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	_, _ = Heartbeat(root, who("B"), now, false)
	_, _ = Heartbeat(root, who("A"), now, false)
	_ = Claim(root, who("A"), "demo/2.1", false, now, false)
	if err := EndSession(root, who("A"), now); err != nil {
		t.Fatal(err)
	}
	if err := Claim(root, who("B"), "demo/2.1", false, now, false); err != nil {
		t.Fatalf("B after A ended: %v", err)
	}
	evs, _ := Inbox(root, who("B"), now, 0)
	if !strings.Contains(kinds(evs), "release:demo/2.1,session_end:") {
		t.Fatalf("events %s", kinds(evs))
	}
	st, _ := ReadSyncStatus(root, now)
	for _, s := range st.Sessions {
		if s.ID == "A" {
			t.Fatal("A still listed")
		}
	}
	dir, _ := trackerStateDir(root)
	if _, err := os.Stat(markerPath(dir, "A")); err == nil {
		t.Fatal("end left the heartbeat marker")
	}
}

func TestSyncBegin(t *testing.T) {
	root := t.TempDir()
	env := filepath.Join(t.TempDir(), "env")
	if err := BeginSession(root, who("abc"), time.Now(), env, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env)
	if !strings.HasSuffix(string(b), "export QRTX_SESSION=abc\n") {
		t.Fatalf("env file %q", b)
	}
	t.Setenv("QRTX_SESSION", "abc")
	me := ResolveSession(root, "", "")
	if err := Claim(root, me, "demo/1.1", false, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	st, _ := ReadSyncStatus(root, time.Now())
	if len(st.Claims) != 1 || st.Claims[0].Session != "abc" {
		t.Fatalf("claims %+v", st.Claims)
	}
	if shellQuote("a b'c") != `'a b'\''c'` {
		t.Fatal(shellQuote("a b'c"))
	}
}

func TestSyncClaim(t *testing.T) {
	fresh := t.TempDir()
	if err := Claim(fresh, who("A"), "demo/1.1", false, time.Now(), false); !errors.Is(err, ErrSyncNotEnabled) {
		t.Fatalf("fresh repo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fresh, ".sdd")); err == nil {
		t.Fatal("claim in a fresh repo created the state dir")
	}
	root := t.TempDir()
	now := time.Now()
	_, _ = Heartbeat(root, who("A"), now, false)
	_, _ = Heartbeat(root, who("B"), now, false)
	if err := Claim(root, who("A"), "demo/2.1", false, now, false); err != nil {
		t.Fatal(err)
	}
	err := Claim(root, who("B"), "demo/2.1", false, now, false)
	var held *ClaimHeldError
	if !errors.As(err, &held) || held.Session != "A" || !strings.Contains(err.Error(), "/wt/A") || !strings.Contains(err.Error(), "b-A") {
		t.Fatalf("conflict: %v", err)
	}
	dir, _ := trackerStateDir(root)
	before, _ := lastEventSeq(dir)
	if err := Claim(root, who("A"), "demo/2.1", false, now, false); err != nil {
		t.Fatal(err)
	}
	if after, _ := lastEventSeq(dir); after != before {
		t.Fatal("re-claim added an event")
	}
	if err := Claim(root, who("A"), "demo/2.1", true, now, false); err != nil {
		t.Fatal(err)
	}
	if err := Claim(root, who("B"), "demo/2.1", false, now, false); err != nil {
		t.Fatalf("after release: %v", err)
	}
}

func TestSyncInbox(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	_, _ = Heartbeat(root, who("A"), now, false)
	_, _ = Heartbeat(root, who("B"), now, false)
	_ = Claim(root, who("A"), "github:7", false, now, false)
	evs, _ := Inbox(root, who("B"), now, 0)
	if len(evs) != 1 || evs[0].Summary() != "A claimed github:7 in A (b-A)" {
		t.Fatalf("B inbox %+v", evs)
	}
	if evs, _ := Inbox(root, who("B"), now, 0); len(evs) != 0 {
		t.Fatal("second inbox repeated events")
	}
	if evs, _ := Inbox(root, who("A"), now, 0); len(evs) != 0 {
		t.Fatalf("A sees its own claim: %+v", evs)
	}
	// 50 earlier events, then a new session C sees only later ones.
	for i := 0; i < 50; i++ {
		_ = Claim(root, who("A"), "k/"+string(rune('a'+i%26))+string(rune('a'+i/26)), false, now, false)
	}
	_, _ = Heartbeat(root, who("C"), now, false)
	if evs, _ := Inbox(root, who("C"), now, 0); len(evs) != 0 {
		t.Fatalf("new session got history: %d", len(evs))
	}
	for i := 0; i < 25; i++ {
		_ = Claim(root, who("A"), "late/"+string(rune('a'+i)), false, now, false)
	}
	if evs, _ := Inbox(root, who("C"), now, 0); len(evs) != 25 {
		t.Fatalf("C later events: %d", len(evs))
	}
}

func TestSyncTrim(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	_, _ = Heartbeat(root, who("B"), now, false)
	dir, _ := trackerStateDir(root)
	var evs []SyncEvent
	for i := 0; i < 6000; i++ {
		evs = append(evs, SyncEvent{Kind: EventClaim, Key: "k", Session: "A", At: now})
	}
	if err := withSync(root, now, 0, true, func(tx *syncTx) error {
		tx.events = evs
		tx.state.Cursors["B"] = 10
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := withSync(root, now, 0, true, func(tx *syncTx) error { return trimEventsLocked(tx.dir, now) }); err != nil {
		t.Fatal(err)
	}
	kept, _ := readEvents(dir)
	if len(kept) != maxSyncEvents || kept[0].Seq != 1001 {
		t.Fatalf("kept %d first %d", len(kept), kept[0].Seq)
	}
	got, err := Inbox(root, who("B"), now, 0)
	if err != nil || len(got) != maxSyncEvents {
		t.Fatalf("cursor before trimmed start: %d %v", len(got), err)
	}
}

func TestDaemonLock(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "daemon.lock")
	unlock, ok, err := tryLockFile(p)
	if err != nil || !ok {
		t.Fatalf("first: %v %v", ok, err)
	}
	if !daemonRunning(dir) {
		t.Fatal("held lock not seen as a running daemon")
	}
	_, ok2, err := tryLockFile(p)
	if err != nil || ok2 {
		t.Fatalf("second try succeeded while held: %v %v", ok2, err)
	}
	unlock()
	if daemonRunning(dir) {
		t.Fatal("released lock still seen as running")
	}
	u3, ok3, _ := tryLockFile(p)
	if !ok3 {
		t.Fatal("not free after release")
	}
	u3()
}

func TestPoll(t *testing.T) {
	root, dir := fakeRepo(t, "")
	seedFake(t, dir, Issue{ID: "F-1", Title: "one", Status: IssueStatus{Category: CategoryUnstarted}, UpdatedAt: "1"})
	_ = os.WriteFile(filepath.Join(root, RepoConfigFile), []byte("tracker:\n  backend: fake\n  timeout: 2\nsync:\n  interval: 1ms\n"), 0o644)
	if _, err := Heartbeat(root, who("A"), time.Now(), false); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	count := func() int {
		evs, _ := EventsAfter(root, 0)
		n := 0
		for _, e := range evs {
			if e.Kind == EventRemoteChange {
				n++
			}
		}
		return n
	}
	pollRoot(root, &log)
	time.Sleep(5 * time.Millisecond)
	pollRoot(root, &log)
	if n := count(); n != 0 {
		t.Fatalf("first/unchanged poll emitted %d (%s)", n, log.String())
	}
	s := loadFake()
	s.Issues[0].UpdatedAt = "2"
	saveFake(s)
	time.Sleep(5 * time.Millisecond)
	pollRoot(root, &log)
	if n := count(); n != 1 {
		t.Fatalf("changed poll emitted %d (%s)", n, log.String())
	}
	evs, _ := EventsAfter(root, 0)
	if last := evs[len(evs)-1]; last.Key != "fake:F-1" || !strings.Contains(last.Detail, "one") {
		t.Fatalf("event %+v", last)
	}
	st, _ := ReadSyncStatus(root, time.Now())
	if len(st.LastPoll) != 1 {
		t.Fatalf("last_poll %v", st.LastPoll)
	}
}

func TestSyncClaimAcrossWorktrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git")
	}
	repo := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run(repo, "init", "-q", "-b", "main")
	_ = os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(repo, "svc"), 0o755)
	_ = os.WriteFile(filepath.Join(repo, "svc", ".sdd.yaml"), []byte("sync:\n  lease: 1h\n"), 0o644)
	run(repo, "add", "-A")
	run(repo, "commit", "-q", "-m", "x")
	wt := filepath.Join(t.TempDir(), "wt")
	run(repo, "worktree", "add", "-q", "-b", "other", wt)
	now := time.Now()
	_, _ = Heartbeat(repo, who("A"), now, false)
	_, _ = Heartbeat(wt, who("B"), now, false)
	if err := Claim(repo, who("A"), "demo/1.1", false, now, false); err != nil {
		t.Fatal(err)
	}
	var held *ClaimHeldError
	if err := Claim(wt, who("B"), "demo/1.1", false, now, false); !errors.As(err, &held) || held.Session != "A" {
		t.Fatalf("worktrees of one project must share claims: %v", err)
	}
	// A subproject is its own scope.
	if err := Claim(filepath.Join(repo, "svc"), who("B"), "demo/1.1", false, now, false); err != nil {
		t.Fatalf("subproject should not conflict: %v", err)
	}
	if syncScope(repo) != syncScope(wt) || syncScope(repo) == syncScope(filepath.Join(repo, "svc")) {
		t.Fatalf("scopes %s %s %s", syncScope(repo), syncScope(wt), syncScope(filepath.Join(repo, "svc")))
	}
}
