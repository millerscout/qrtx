package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/millerscout/qrtx"
)

func TestTrackerStartWarns(t *testing.T) {
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	ref := addIssue(t, "Dark mode")
	if _, errOut, err := run(t, "tracker", "start", ref, "dark-mode"); err != nil || strings.Contains(errOut, "warning:") {
		t.Fatalf("first start: %q %v", errOut, err)
	}
	out, errOut, err := run(t, "tracker", "start", ref, "dark-mode-2")
	if err != nil {
		t.Fatalf("second start must still succeed: %v", err)
	}
	if !strings.Contains(errOut, "warning:") || !strings.Contains(errOut, "dark-mode") {
		t.Fatalf("want a warning naming dark-mode on stderr, got %q (stdout %q)", errOut, out)
	}
	for _, slug := range []string{"dark-mode", "dark-mode-2"} {
		raw, err := os.ReadFile(filepath.Join(root, "sdd", "changes", slug, "change.sdd.md"))
		if err != nil || !strings.Contains(string(raw), ref) {
			t.Fatalf("%s should list %s: %v", slug, ref, err)
		}
	}
}
