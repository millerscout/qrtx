package qrtx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTrackerExec(t *testing.T) {
	root, dir := fakeRepo(t, "")
	seedFake(t, dir, Issue{ID: "F-1", Title: "from fake", Status: IssueStatus{Category: CategoryUnstarted}})
	tr, err := OpenTracker(root)
	if err != nil {
		t.Fatal(err)
	}
	list, err := tr.ListIssues(IssueFilter{})
	if err != nil || len(list) != 1 || list[0].Title != "from fake" {
		t.Fatalf("list %v %v", list, err)
	}
	if log := fakeLog(dir); len(log) < 2 || log[0] != OpCapabilities {
		t.Fatalf("capabilities must come first: %v", log)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "deadline")); len(b) == 0 {
		t.Fatal("plugin did not get QRTX_TRACKER_DEADLINE")
	} else if d, err := time.Parse(time.RFC3339Nano, string(b)); err != nil || time.Until(d) > 3*time.Second {
		t.Fatalf("deadline %q %v (timeout is 2s)", b, err)
	}

	t.Run("no shared protocol", func(t *testing.T) {
		setFakeMode(t, dir, "v2")
		defer setFakeMode(t, dir, "")
		tr, _ := OpenTracker(root)
		_, err := tr.ListIssues(IssueFilter{})
		if err == nil || !strings.Contains(err.Error(), "sdd.adapter/v2") || !strings.Contains(err.Error(), AdapterProtocol) {
			t.Fatalf("want both versions named: %v", err)
		}
	})
	for _, mode := range []string{"sleep", "nonjson", "auth"} {
		t.Run(mode, func(t *testing.T) {
			setFakeMode(t, dir, mode)
			defer setFakeMode(t, dir, "")
			tr, _ := OpenTracker(root)
			_, err := tr.GetIssue("F-1")
			if err == nil || !strings.Contains(err.Error(), "tracker fake") {
				t.Fatalf("%s: %v", mode, err)
			}
			if mode == "auth" && adapterCode(err) != ErrCodeAuth {
				t.Fatalf("auth code lost: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, IssuesFile)); err == nil {
				t.Fatal("exec failure wrote sdd/issues.jsonl")
			}
		})
	}
	t.Run("plugin reports its own deadline", func(t *testing.T) {
		// Race-enabled binaries (the plugin is the test binary) sleep 1s at
		// exit by default, which would outlast the 200ms margin under -race.
		t.Setenv("GORACE", "atexit_sleep_ms=0")
		setFakeMode(t, dir, "stall")
		defer setFakeMode(t, dir, "")
		tr, _ := OpenTracker(root)
		_, err := tr.GetIssue("F-1")
		if err == nil || !strings.Contains(err.Error(), "GET /issues") || strings.Contains(err.Error(), "timed out after") {
			t.Fatalf("want the plugin's error, not qrtx's kill: %v", err)
		}
	})
	t.Run("invalid backend name", func(t *testing.T) {
		for _, bad := range []string{"../evil", "a/b"} {
			r := t.TempDir()
			_ = os.WriteFile(filepath.Join(r, RepoConfigFile), []byte("tracker:\n  backend: "+bad+"\n"), 0o644)
			if _, err := OpenTracker(r); err == nil || !strings.Contains(err.Error(), bad) {
				t.Fatalf("%s: %v", bad, err)
			}
		}
	})
	t.Run("undeclared comment is never sent", func(t *testing.T) {
		setFakeMode(t, dir, "nocomment")
		defer setFakeMode(t, dir, "")
		tr, _ := OpenTracker(root)
		_ = os.Remove(filepath.Join(dir, "log"))
		err := tr.Comment("F-1", "hi", "c1")
		if adapterCode(err) != ErrCodeUnsupported {
			t.Fatalf("want unsupported: %v", err)
		}
		for _, op := range fakeLog(dir) {
			if op == OpComment {
				t.Fatal("comment reached the plugin")
			}
		}
	})
}

func TestTrackerExecLinks(t *testing.T) {
	root, dir := fakeRepo(t, "")
	seedFake(t, dir,
		Issue{ID: "F-1", Title: "a", Status: IssueStatus{Category: CategoryBacklog}},
		Issue{ID: "F-2", Title: "b", Status: IssueStatus{Category: CategoryBacklog}})
	tr := openT(t, root)
	two := 2
	got, err := tr.UpdateIssue("F-1", UpdateIssueInput{AddLinks: []IssueLink{{Type: "blocks", ID: "F-2"}}, Priority: &two})
	if err != nil || len(got.Links) != 1 || got.Priority == nil || *got.Priority != 2 {
		t.Fatalf("exec dropped add_links or priority: %+v %v", got, err)
	}
	if got, err = tr.UpdateIssue("F-1", UpdateIssueInput{RemoveLinks: []IssueLink{{Type: "blocks", ID: "F-2"}}}); err != nil || len(got.Links) != 0 {
		t.Fatalf("exec dropped remove_links: %+v %v", got, err)
	}
	zero := 0
	if got, err = tr.UpdateIssue("F-1", UpdateIssueInput{Priority: &zero}); err != nil || got.Priority == nil || *got.Priority != 0 {
		t.Fatalf("priority 0 must be sent: %+v %v", got, err)
	}
}
