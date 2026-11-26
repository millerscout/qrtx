package sdd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetTrackerConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, RepoConfigFile)
	orig := "# team settings\nproject: shop # the product\nquick_check: go test ./...\n"
	_ = os.WriteFile(path, []byte(orig), 0o644)
	c := TrackerConfig{Backend: "github", Config: map[string]any{"repo": "o/r"}}
	changed, err := SetTrackerConfig(root, c)
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	got, _ := os.ReadFile(path)
	for _, want := range []string{"# team settings", "project: shop # the product", "quick_check: go test ./...", "tracker:", "backend: github", "repo: o/r"} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
	cfg, err := LoadRepoConfig(root)
	if err != nil || cfg.Project != "shop" || cfg.Tracker.Backend != "github" || cfg.Tracker.Config["repo"] != "o/r" {
		t.Fatalf("reload: %+v %v", cfg, err)
	}
	if changed, err := SetTrackerConfig(root, c); err != nil || changed {
		t.Fatalf("re-run: %v %v", changed, err)
	}
	if again, _ := os.ReadFile(path); !bytes.Equal(again, got) {
		t.Fatal("re-run rewrote .sdd.yaml")
	}
	if changed, _ := SetTrackerConfig(root, TrackerConfig{Backend: "github", Config: map[string]any{"repo": "o/other"}}); !changed {
		t.Fatal("new repo not written")
	}
	// No file yet.
	r2 := t.TempDir()
	if changed, err := SetTrackerConfig(r2, TrackerConfig{Backend: "jira", Config: map[string]any{"url": "https://x.atlassian.net", "project": "SHOP"}}); err != nil || !changed {
		t.Fatalf("new file: %v %v", changed, err)
	}
	if cfg, _ := LoadRepoConfig(r2); cfg.Tracker.Backend != "jira" {
		t.Fatalf("%+v", cfg)
	}
}
