package sdd

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
		t.Fatal("plugin did not get SDD_TRACKER_DEADLINE")
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
