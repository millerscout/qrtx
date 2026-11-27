package qrtx

import (
	"os"
	"strings"
	"testing"
)

// blockedPair makes issue A (in category) block issue B on the file backend.
func blockedPair(t *testing.T, root, category string) (tr *Tracker, a, b Issue) {
	t.Helper()
	tr = openT(t, root)
	var err error
	if a, err = tr.CreateIssue(CreateIssueInput{Title: "Rename", Category: category}); err != nil {
		t.Fatal(err)
	}
	if b, err = tr.CreateIssue(CreateIssueInput{Title: "Release"}); err != nil {
		t.Fatal(err)
	}
	if _, err = SetLink(tr, "file:"+a.ID, "blocks", "file:"+b.ID, false); err != nil {
		t.Fatal(err)
	}
	return tr, a, b
}

func TestStartBlocked(t *testing.T) {
	t.Run("refused while the blocker is open, creates nothing", func(t *testing.T) {
		for _, cat := range []string{CategoryBacklog, CategoryUnstarted, CategoryStarted} {
			root := t.TempDir()
			tr, a, b := blockedPair(t, root, cat)
			_, err := StartFromIssue(root, tr, "file:"+b.ID, "b-slug")
			if err == nil || !strings.Contains(err.Error(), "file:"+a.ID) || !strings.Contains(err.Error(), "--force") {
				t.Fatalf("%s: want a refusal naming %s: %v", cat, a.ID, err)
			}
			if _, err := os.Stat(changeDir(root, "b-slug")); err == nil {
				t.Fatal("a refused start created the change")
			}
			if entries, _ := ListOutbox(root); len(entries) != 0 {
				t.Fatalf("a refused start queued writes: %v", entries)
			}
		}
	})
	t.Run("force starts and says so", func(t *testing.T) {
		root := t.TempDir()
		tr, a, b := blockedPair(t, root, CategoryStarted)
		res, err := StartFromIssueWith(root, tr, "file:"+b.ID, "b-slug", StartOptions{Force: true})
		if err != nil || !res.Forced || len(res.Blockers) != 1 || res.Blockers[0] != "file:"+a.ID {
			t.Fatalf("forced start: %+v %v", res, err)
		}
		if _, err := os.Stat(changeDir(root, "b-slug")); err != nil {
			t.Fatal("forced start created nothing")
		}
	})
	t.Run("closed blockers do not block", func(t *testing.T) {
		for _, cat := range []string{CategoryCompleted, CategoryCanceled} {
			root := t.TempDir()
			tr, _, b := blockedPair(t, root, cat)
			res, err := StartFromIssue(root, tr, "file:"+b.ID, "b-slug")
			if err != nil || res.Forced || len(res.Blockers) != 0 {
				t.Fatalf("%s: %+v %v", cat, res, err)
			}
		}
	})
	t.Run("relates never blocks", func(t *testing.T) {
		root := t.TempDir()
		tr := openT(t, root)
		a, _ := tr.CreateIssue(CreateIssueInput{Title: "A"})
		b, _ := tr.CreateIssue(CreateIssueInput{Title: "B"})
		if _, err := SetLink(tr, "file:"+a.ID, "relates", "file:"+b.ID, false); err != nil {
			t.Fatal(err)
		}
		if _, err := StartFromIssue(root, tr, "file:"+b.ID, "b-slug"); err != nil {
			t.Fatalf("relates must not block: %v", err)
		}
	})
}

// Links in a plugin backend hold prefixed or bare ids; both name the issue.
func TestStartBlockedExec(t *testing.T) {
	seed := func(t *testing.T, linkID string) (string, string) {
		root, dir := fakeRepo(t, "")
		seedFake(t, dir,
			Issue{ID: "F-1", Title: "a", Status: IssueStatus{Category: CategoryStarted}, Links: []IssueLink{{Type: "blocks", ID: linkID}}},
			Issue{ID: "F-2", Title: "b", Status: IssueStatus{Category: CategoryBacklog}})
		return root, dir
	}
	for _, linkID := range []string{"F-2", "fake:F-2"} {
		root, _ := seed(t, linkID)
		tr := openT(t, root)
		if _, err := StartFromIssue(root, tr, "fake:F-2", "b-slug"); err == nil || !strings.Contains(err.Error(), "fake:F-1") {
			t.Fatalf("link id %q: want a refusal naming fake:F-1: %v", linkID, err)
		}
	}

	t.Run("page cap fails closed", func(t *testing.T) {
		old := maxListPages
		maxListPages = 3
		defer func() { maxListPages = old }()
		root, dir := seed(t, "F-9")
		setFakeMode(t, dir, "endless")
		tr := openT(t, root)
		_, err := StartFromIssue(root, tr, "fake:F-2", "b-slug")
		if err == nil || !strings.Contains(err.Error(), "incomplete") || !strings.Contains(err.Error(), "--force") {
			t.Fatalf("want an incomplete-listing refusal: %v", err)
		}
		if _, err := os.Stat(changeDir(root, "b-slug")); err == nil {
			t.Fatal("a refused start created the change")
		}
		res, err := StartFromIssueWith(root, tr, "fake:F-2", "b-slug", StartOptions{Force: true})
		if err != nil || !res.Forced {
			t.Fatalf("force overrides: %+v %v", res, err)
		}
	})
	t.Run("a plugin without list_issues skips the check with a warning", func(t *testing.T) {
		root, dir := seed(t, "F-2")
		setFakeMode(t, dir, "nolist")
		tr := openT(t, root)
		res, err := StartFromIssue(root, tr, "fake:F-2", "b-slug")
		if err != nil || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "not checked") {
			t.Fatalf("want a warning and a start: %+v %v", res, err)
		}
		for _, op := range fakeLog(dir) {
			if op == OpListIssues {
				t.Fatal("list_issues was called though it is not declared")
			}
		}
	})
}
