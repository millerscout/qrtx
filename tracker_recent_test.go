package sdd

import (
	"os/exec"
	"testing"
	"time"
)

func TestPluginDeadlineLeavesMargin(t *testing.T) {
	if d := pluginDeadline(30 * time.Second); d != 27*time.Second {
		t.Fatalf("30s -> %s", d)
	}
	if d := pluginDeadline(2 * time.Second); d != 1800*time.Millisecond {
		t.Fatalf("2s -> %s", d)
	}
}

func TestWithRecentCreates(t *testing.T) {
	root := t.TempDir()
	if err := exec.Command("git", "-C", root, "init", "-q").Run(); err != nil {
		t.Skip("git unavailable")
	}
	is := Issue{ID: "13", Title: "new", Status: IssueStatus{Category: CategoryUnstarted}}
	RecordCreated(root, "github", is)
	get := func(id string) (Issue, error) { return is, nil }
	open := IssueFilter{Categories: []string{CategoryBacklog, CategoryUnstarted, CategoryStarted}}
	got := WithRecentCreates(root, "github", open, []Issue{{ID: "12"}}, get)
	if len(got) != 2 || got[0].ID != "13" {
		t.Fatalf("lagged issue not merged: %+v", got)
	}
	if got := WithRecentCreates(root, "github", open, []Issue{{ID: "13"}, {ID: "12"}}, get); len(got) != 2 {
		t.Fatalf("duplicate merged: %+v", got)
	}
	if got := WithRecentCreates(root, "jira", open, nil, get); len(got) != 0 {
		t.Fatalf("other backend merged: %+v", got)
	}
	is.Status.Category = CategoryCompleted // closed since it was created
	if got := WithRecentCreates(root, "github", open, nil, get); len(got) != 0 {
		t.Fatalf("stale status merged: %+v", got)
	}
	is.Status.Category = CategoryUnstarted
	done := IssueFilter{Categories: []string{CategoryCompleted}}
	if got := WithRecentCreates(root, "github", done, nil, get); len(got) != 0 {
		t.Fatalf("category filter ignored: %+v", got)
	}
	if got := WithRecentCreates(root, "github", IssueFilter{Change: "x"}, nil, get); len(got) != 0 {
		t.Fatalf("change filter ignored: %+v", got)
	}
}
