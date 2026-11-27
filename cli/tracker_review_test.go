package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/millerscout/qrtx"
)

// contextLines returns the `sdd:` lines of `tracker get` output.
func contextLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "sdd:") {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	return lines
}

func addIssue(t *testing.T, title string) string {
	t.Helper()
	out, _, err := run(t, "tracker", "add", title)
	if err != nil || !strings.HasPrefix(out, "added ") {
		t.Fatalf("add: %q %v", out, err)
	}
	return strings.TrimSpace(strings.TrimPrefix(out, "added "))
}

func TestTrackerGetContext(t *testing.T) {
	t.Run("started issue shows the change line only", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv(qrtx.EnvRepo, root)
		ref := addIssue(t, "Dark mode")
		if _, _, err := run(t, "tracker", "start", ref, "dark-mode"); err != nil {
			t.Fatal(err)
		}
		out, _, err := run(t, "tracker", "get", ref)
		if err != nil {
			t.Fatal(err)
		}
		lines := contextLines(out)
		if len(lines) != 1 || !strings.HasSuffix(lines[0], "dark-mode") {
			t.Fatalf("want one context line naming dark-mode, got %q\n%s", lines, out)
		}
		js, _, err := run(t, "tracker", "get", ref, "--json")
		if err != nil {
			t.Fatal(err)
		}
		var is qrtx.Issue
		if err := json.Unmarshal([]byte(js), &is); err != nil {
			t.Fatal(err)
		}
		if len(is.Refs) != 2 {
			t.Fatalf("--json must keep every ref: %+v", is.Refs)
		}
	})
	t.Run("added issue keeps its context line", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv(qrtx.EnvRepo, root)
		ref := addIssue(t, "Later")
		out, _, err := run(t, "tracker", "get", ref)
		if err != nil {
			t.Fatal(err)
		}
		if lines := contextLines(out); len(lines) != 1 || strings.HasSuffix(lines[0], " ") {
			t.Fatalf("want one trimmed context line, got %q", lines)
		}
	})
}
