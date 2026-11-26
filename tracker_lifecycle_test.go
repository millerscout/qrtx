package sdd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readyChange writes a complete, archivable change linked to issues.
func readyChange(t *testing.T, root, slug string, issues []string, deferred ...string) {
	t.Helper()
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(changeFilePath(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "Ship "+slug+".", 1)
	text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", "- "+slug+" works.", 1)
	text = RemovePlaceholderSpecSections(text)
	text = strings.Replace(text, "- [ ] 1.1 First implementation task", "- [x] 1.1 Do it", 1)
	text = strings.Replace(text, "- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)", "- [x] V1.1 manual: done", 1)
	text = strings.Replace(text, "- [ ] V1.2 (optional) golden row in scripts/fixtures/ if behavior is user-visible", "- [x] V1.2 manual: n/a", 1)
	text = strings.Replace(text, "status: planning", "status: complete", 1)
	if len(deferred) > 0 {
		text = replaceSection(text, "Deferred", "- "+strings.Join(deferred, "\n- "))
	}
	if text, err = setManifestIssues(text, issues); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changeFilePath(root, slug), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChangeManifest(root, slug); err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
}

func openT(t *testing.T, root string) *Tracker {
	t.Helper()
	tr, err := OpenTracker(root)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func archivedDir(root, slug string) string {
	return filepath.Join(changesRoot(root), archiveDirName, time.Now().UTC().Format("2006-01-02")+"-"+slug)
}

func fakeIssue(t *testing.T, id string) Issue {
	t.Helper()
	for _, is := range loadFake().Issues {
		if is.ID == id {
			return is
		}
	}
	t.Fatalf("fake has no %s", id)
	return Issue{}
}

func TestTrackerStart(t *testing.T) {
	root := t.TempDir()
	tr := openT(t, root)
	is, err := tr.CreateIssue(CreateIssueInput{Title: "Dark-mode toggle", Description: "On the settings page."})
	if err != nil {
		t.Fatal(err)
	}
	res, err := StartFromIssue(root, tr, "file:"+is.ID, "dark-mode")
	if err != nil || !res.Created {
		t.Fatalf("start: %+v %v", res, err)
	}
	if rep := DrainOutbox(root, tr); rep.Pending != 0 || rep.Sent != 2 {
		t.Fatalf("drain: %+v", rep)
	}
	raw, _ := os.ReadFile(changeFilePath(root, "dark-mode"))
	text := string(raw)
	man, err := ParseChangeMarkdown(text, "x")
	if err != nil || len(man.Issues) != 1 || man.Issues[0] != "file:"+is.ID {
		t.Fatalf("manifest issues %v %v", man, err)
	}
	if ur := sectionMarkdown(text, "User request"); !strings.Contains(ur, "Dark-mode toggle") || !strings.Contains(ur, "On the settings page.") {
		t.Fatalf("user request %q", ur)
	}
	got, _ := tr.GetIssue(is.ID)
	if got.Status.Category != CategoryStarted || len(got.Refs) == 0 || got.Refs[len(got.Refs)-1].Change != "dark-mode" {
		t.Fatalf("issue after start: %+v", got)
	}

	// An existing, written change keeps its User request.
	readyChange(t, root, "written", nil)
	before := sectionMarkdown(mustRead(t, changeFilePath(root, "written")), "User request")
	other, _ := tr.CreateIssue(CreateIssueInput{Title: "Other"})
	if res, err := StartFromIssue(root, tr, other.ID, "written"); err != nil || res.Created {
		t.Fatalf("start existing: %+v %v", res, err)
	}
	after := mustRead(t, changeFilePath(root, "written"))
	if sectionMarkdown(after, "User request") != before || !strings.Contains(after, `issues: ["file:`+other.ID+`"]`) {
		t.Fatalf("existing change changed wrongly:\n%s", after)
	}

	// A missing issue creates nothing.
	if _, err := StartFromIssue(root, tr, "file:sdd-nope00", "ghost"); err == nil {
		t.Fatal("missing issue accepted")
	}
	if _, err := os.Stat(changeDir(root, "ghost")); err == nil {
		t.Fatal("ghost change created")
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

func TestTrackerArchive(t *testing.T) {
	t.Run("failing tracker never blocks archive", func(t *testing.T) {
		root, dir := fakeRepo(t, "")
		seedFake(t, dir, Issue{ID: "F-1", Title: "X", Status: IssueStatus{Category: CategoryStarted}})
		readyChange(t, root, "demo", []string{"fake:F-1"})
		setFakeMode(t, dir, "down")
		if err := ArchiveChange(root, "demo"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(archivedDir(root, "demo")); err != nil {
			t.Fatal("not archived")
		}
		rep := DrainOutbox(root, openT(t, root))
		if rep.Pending != 2 {
			t.Fatalf("want 2 pending: %+v", rep)
		}
		if list, _ := ListOutbox(root); len(list) != 2 || list[0].Op != OpComment || list[1].Op != OpTransitionIssue {
			t.Fatalf("outbox %+v", list)
		}
		setFakeMode(t, dir, "")
		if rep := DrainOutbox(root, openT(t, root)); rep.Pending != 0 || rep.Sent != 2 {
			t.Fatalf("recovered drain %+v", rep)
		}
		if is := fakeIssue(t, "F-1"); is.Status.Category != CategoryCompleted || loadFake().Comments["F-1"] != 1 {
			t.Fatalf("after recovery %+v comments %v", is, loadFake().Comments)
		}
	})
	t.Run("undeclared comment dropped", func(t *testing.T) {
		root, dir := fakeRepo(t, "")
		seedFake(t, dir, Issue{ID: "F-1", Title: "X", Status: IssueStatus{Category: CategoryStarted}})
		readyChange(t, root, "demo", []string{"fake:F-1"})
		setFakeMode(t, dir, "nocomment")
		if err := ArchiveChange(root, "demo"); err != nil {
			t.Fatal(err)
		}
		if rep := DrainOutbox(root, openT(t, root)); rep.Pending != 0 {
			t.Fatalf("%+v", rep)
		}
		for _, op := range fakeLog(dir) {
			if op == OpComment {
				t.Fatal("comment sent to a plugin without comment")
			}
		}
		if fakeIssue(t, "F-1").Status.Category != CategoryCompleted {
			t.Fatal("not completed")
		}
	})
	t.Run("issue shared by two changes", func(t *testing.T) {
		root, dir := fakeRepo(t, "")
		seedFake(t, dir, Issue{ID: "F-1", Title: "X", Status: IssueStatus{Category: CategoryStarted}})
		readyChange(t, root, "a", []string{"fake:F-1"})
		readyChange(t, root, "b", []string{"F-1"})
		if err := ArchiveChange(root, "a"); err != nil {
			t.Fatal(err)
		}
		DrainOutbox(root, openT(t, root))
		if c := fakeIssue(t, "F-1").Status.Category; c != CategoryStarted || loadFake().Comments["F-1"] != 1 {
			t.Fatalf("after a: %s comments %v", c, loadFake().Comments)
		}
		if err := ArchiveChange(root, "b"); err != nil {
			t.Fatal(err)
		}
		DrainOutbox(root, openT(t, root))
		if c := fakeIssue(t, "F-1").Status.Category; c != CategoryCompleted {
			t.Fatalf("after b: %s", c)
		}
	})
	t.Run("comments off and mirror config", func(t *testing.T) {
		root, dir := fakeRepo(t, "  comments: off\n")
		seedFake(t, dir, Issue{ID: "F-1", Title: "X", Status: IssueStatus{Category: CategoryStarted}})
		readyChange(t, root, "demo", []string{"fake:F-1"})
		if err := ArchiveChange(root, "demo"); err != nil {
			t.Fatal(err)
		}
		if list, _ := ListOutbox(root); len(list) != 1 || list[0].Op != OpTransitionIssue {
			t.Fatalf("comments off should queue only the transition: %+v", list)
		}
		r2 := t.TempDir()
		readyChange(t, r2, "m", nil)
		_ = os.WriteFile(filepath.Join(r2, RepoConfigFile), []byte("tracker:\n  mirror: tasks\n"), 0o644)
		if err := ValidateChangeManifest(r2, "m"); err != nil {
			t.Fatalf("verify must ignore the tracker block: %v", err)
		}
	})
	t.Run("no tracker, no issues: unchanged", func(t *testing.T) {
		root := t.TempDir()
		readyChange(t, root, "plain", nil)
		if err := ArchiveChange(root, "plain"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(root, ".sdd")); err == nil {
			t.Fatal("state directory created for an unlinked archive")
		}
	})
	t.Run("failed move leaves no entries", func(t *testing.T) {
		root, dir := fakeRepo(t, "")
		seedFake(t, dir, Issue{ID: "F-1", Title: "X", Status: IssueStatus{Category: CategoryStarted}})
		readyChange(t, root, "demo", []string{"fake:F-1"})
		_ = os.MkdirAll(archivedDir(root, "demo"), 0o755)
		if err := ArchiveChange(root, "demo"); err == nil {
			t.Fatal("archive over an existing destination should fail")
		}
		if list, _ := ListOutbox(root); len(list) != 0 {
			t.Fatalf("failed archive left entries: %+v", list)
		}
		_ = os.RemoveAll(archivedDir(root, "demo"))
		setFakeMode(t, dir, "down")
		if err := ArchiveChange(root, "demo"); err != nil {
			t.Fatal(err)
		}
		if list, _ := ListOutbox(root); len(list) != 2 {
			t.Fatalf("want each entry once: %+v", list)
		}
	})
	t.Run("rename failure removes entries", func(t *testing.T) {
		root, dir := fakeRepo(t, "")
		seedFake(t, dir, Issue{ID: "F-1", Title: "X", Status: IssueStatus{Category: CategoryStarted}})
		readyChange(t, root, "demo", []string{"fake:F-1"})
		// The archive parent is a file, so MkdirAll fails after queueing.
		_ = os.WriteFile(filepath.Join(changesRoot(root), archiveDirName), []byte("x"), 0o644)
		if err := ArchiveChange(root, "demo"); err == nil {
			t.Fatal("want failure")
		}
		if list, _ := ListOutbox(root); len(list) != 0 {
			t.Fatalf("entries left after failed move: %+v", list)
		}
	})
}

func TestTrackerDeferred(t *testing.T) {
	t.Run("file backend, twice", func(t *testing.T) {
		root := t.TempDir()
		readyChange(t, root, "demo", nil, "Export to CSV later.", "`tracker-sync` owns the daemon.")
		before := mustRead(t, changeFilePath(root, "demo"))
		for i := 0; i < 2; i++ {
			tr := openT(t, root)
			if _, err := QueueDeferredIssues(root, tr, "demo"); err != nil {
				t.Fatal(err)
			}
			DrainOutbox(root, tr)
		}
		list, _ := openT(t, root).ListIssues(IssueFilter{})
		if len(list) != 2 {
			t.Fatalf("want 2 issues: %+v", list)
		}
		for _, is := range list {
			if len(is.Refs) != 1 || is.Refs[0].Change != "demo" || is.Refs[0].Role != "deferred" {
				t.Fatalf("refs %+v", is.Refs)
			}
		}
		if mustRead(t, changeFilePath(root, "demo")) != before {
			t.Fatal("from-deferred changed the manifest")
		}
	})
	t.Run("re-run while pending", func(t *testing.T) {
		root, dir := fakeRepo(t, "")
		readyChange(t, root, "demo", nil, "one", "two")
		setFakeMode(t, dir, "down")
		tr := openT(t, root)
		if n, _ := QueueDeferredIssues(root, tr, "demo"); n != 2 {
			t.Fatalf("queued %d", n)
		}
		DrainOutbox(root, tr)
		if n, _ := QueueDeferredIssues(root, openT(t, root), "demo"); n != 0 {
			t.Fatalf("re-run queued %d more while pending", n)
		}
		setFakeMode(t, dir, "")
		DrainOutbox(root, openT(t, root))
		if n, _ := QueueDeferredIssues(root, openT(t, root), "demo"); n != 2 {
			t.Fatalf("third run queued %d", n)
		}
		DrainOutbox(root, openT(t, root))
		if got := len(loadFake().Issues); got != 2 {
			t.Fatalf("want exactly 2 issues, got %d", got)
		}
	})
}

func TestTrackerOutbox(t *testing.T) {
	t.Run("killed mid-create", func(t *testing.T) {
		root, dir := fakeRepo(t, "")
		readyChange(t, root, "demo", nil, "only one")
		setFakeMode(t, dir, "block")
		cmd := exec.Command("sdd-child")
		cmd.Env = append(os.Environ(), EnvRepo+"="+root, "SDD_CHILD_SLUG=demo")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for {
			if _, err := os.Stat(filepath.Join(dir, "signal")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				_ = cmd.Process.Kill()
				t.Fatal("plugin never signalled")
			}
			time.Sleep(20 * time.Millisecond)
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		setFakeMode(t, dir, "")
		if list, _ := ListOutbox(root); len(list) != 1 {
			t.Fatalf("entry should still be queued: %+v", list)
		}
		DrainOutbox(root, openT(t, root))
		if list, _ := ListOutbox(root); len(list) != 0 {
			t.Fatalf("not drained: %+v", list)
		}
		if got := len(loadFake().Issues); got != 1 {
			t.Fatalf("want exactly one issue, got %d", got)
		}
	})
	t.Run("roots sharing a git dir drain only their own", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("needs git")
		}
		repo := t.TempDir()
		if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		a, b := filepath.Join(repo, "a"), filepath.Join(repo, "b")
		_, dir := fakeRepo(t, "")
		for _, r := range []string{a, b} {
			_ = os.MkdirAll(r, 0o755)
			_ = os.WriteFile(filepath.Join(r, RepoConfigFile), []byte("tracker:\n  backend: fake\n  timeout: 2\n"), 0o644)
		}
		sa, _ := trackerStateDir(a)
		sb, _ := trackerStateDir(b)
		if sa != sb {
			t.Fatalf("state dirs differ: %s %s", sa, sb)
		}
		readyChange(t, a, "demo", nil, "from a")
		setFakeMode(t, dir, "down")
		if _, err := QueueDeferredIssues(a, openT(t, a), "demo"); err != nil {
			t.Fatal(err)
		}
		setFakeMode(t, dir, "")
		if list, _ := ListOutbox(b); len(list) != 0 {
			t.Fatalf("b sees a's entries: %+v", list)
		}
		if rep := DrainOutbox(b, openT(t, b)); rep.Sent != 0 {
			t.Fatalf("b drained a's entries: %+v", rep)
		}
		if rep := DrainOutbox(a, openT(t, a)); rep.Sent != 1 {
			t.Fatalf("a drain: %+v", rep)
		}
	})
}

func TestProposeContextBacklog(t *testing.T) {
	t.Run("capped at 20", func(t *testing.T) {
		root := t.TempDir()
		tr := openT(t, root)
		for i := 0; i < 25; i++ {
			if _, err := tr.CreateIssue(CreateIssueInput{Title: "issue " + string(rune('a'+i))}); err != nil {
				t.Fatal(err)
			}
		}
		if err := NewChange(root, "demo"); err != nil {
			t.Fatal(err)
		}
		rep, err := ProposeContext(root, "demo", ProposeContextOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Backlog) != 20 {
			t.Fatalf("backlog %d", len(rep.Backlog))
		}
	})
	t.Run("tracker down", func(t *testing.T) {
		root, dir := fakeRepo(t, "")
		setFakeMode(t, dir, "down")
		if err := NewChange(root, "demo"); err != nil {
			t.Fatal(err)
		}
		rep, err := ProposeContext(root, "demo", ProposeContextOpts{})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(rep)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if _, ok := m["backlog"]; ok {
			t.Fatal("backlog present on failure")
		}
		for _, k := range []string{"slug", "score", "domain", "packages", "overlaps", "graph_neighbors", "warnings"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("missing key %s", k)
			}
		}
		if !strings.Contains(strings.Join(rep.Warnings, "\n"), "tracker") {
			t.Fatalf("warnings %v", rep.Warnings)
		}
	})
	t.Run("no tracker, no file", func(t *testing.T) {
		root := t.TempDir()
		_ = NewChange(root, "demo")
		rep, err := ProposeContext(root, "demo", ProposeContextOpts{})
		if err != nil || rep.Backlog != nil || strings.Contains(strings.Join(rep.Warnings, " "), "tracker") {
			t.Fatalf("%+v %v", rep, err)
		}
		if _, err := os.Stat(filepath.Join(root, ".sdd")); err == nil {
			t.Fatal("propose-context created the state directory")
		}
	})
}
