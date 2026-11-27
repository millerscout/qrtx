package qrtx

import (
	"os"
	"strings"
	"testing"
)

// overlapping writes change slug sharing package internal/sdd and linking issues.
func overlapping(t *testing.T, root, slug string, issues ...string) {
	t.Helper()
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(changeFilePath(root, slug))
	text := strings.Replace(string(raw), "packages: []", "packages:\n  - internal/sdd", 1)
	var err error
	if text, err = setManifestIssues(text, issues); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changeFilePath(root, slug), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProposeContextOrder(t *testing.T) {
	t.Run("names the change that goes first", func(t *testing.T) {
		root := t.TempDir()
		tr := openT(t, root)
		a, _ := tr.CreateIssue(CreateIssueInput{Title: "Rename"})
		b, _ := tr.CreateIssue(CreateIssueInput{Title: "Release"})
		c, _ := tr.CreateIssue(CreateIssueInput{Title: "Unrelated"})
		if _, err := SetLink(tr, "file:"+a.ID, "blocks", "file:"+b.ID, false); err != nil {
			t.Fatal(err)
		}
		overlapping(t, root, "x", "file:"+a.ID)
		overlapping(t, root, "y", "file:"+b.ID)
		overlapping(t, root, "z", "file:"+c.ID)

		want := "x (file:" + a.ID + " blocks file:" + b.ID + ")"
		rep, err := ProposeContext(root, "y", ProposeContextOpts{})
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, o := range rep.Overlaps {
			got[o.OtherSlug] = o.GoesFirst
		}
		if got["x"] != want || got["z"] != "" || len(rep.Overlaps) != 2 {
			t.Fatalf("goes_first: %+v", rep.Overlaps)
		}
		var human strings.Builder
		_ = WriteProposeContextHuman(&human, rep)
		if !strings.Contains(human.String(), "goes first: "+want) {
			t.Fatalf("human output lacks the order:\n%s", human.String())
		}

		// From x's side the order is the same: x goes first.
		rep, _ = ProposeContext(root, "x", ProposeContextOpts{})
		for _, o := range rep.Overlaps {
			if o.OtherSlug == "y" && o.GoesFirst != want {
				t.Fatalf("from x: %+v", o)
			}
		}
		if strings.Contains(strings.Join(rep.Warnings, "\n"), "order") {
			t.Fatalf("no order warning expected: %v", rep.Warnings)
		}
	})
	t.Run("a warning when the tracker is unreachable", func(t *testing.T) {
		root, dir := fakeRepo(t, "")
		overlapping(t, root, "x", "fake:F-1")
		overlapping(t, root, "y", "fake:F-2")
		setFakeMode(t, dir, "down")
		rep, err := ProposeContext(root, "y", ProposeContextOpts{})
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(rep.Warnings, "\n")
		if !strings.Contains(joined, "order") || !strings.Contains(joined, "tracker") {
			t.Fatalf("want an order warning naming the tracker: %v", rep.Warnings)
		}
		for _, o := range rep.Overlaps {
			if o.GoesFirst != "" {
				t.Fatalf("no order without the tracker: %+v", o)
			}
		}
	})
	t.Run("no linked issues, no tracker call", func(t *testing.T) {
		root, dir := fakeRepo(t, "")
		overlapping(t, root, "x")
		overlapping(t, root, "y", "fake:F-2")
		if _, err := ProposeContext(root, "y", ProposeContextOpts{}); err != nil {
			t.Fatal(err)
		}
		for _, op := range fakeLog(dir) {
			if op == OpGetIssue {
				t.Fatal("get_issue called though x links no issue")
			}
		}
	})
}
