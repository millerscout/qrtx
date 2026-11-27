package qrtx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoRootMarkers(t *testing.T) {
	t.Setenv(EnvRepo, "")
	for _, marker := range []string{RepoConfigFile, "go.mod", ".git"} {
		t.Run(marker, func(t *testing.T) {
			root, _ := filepath.EvalSymlinks(t.TempDir())
			if err := os.WriteFile(filepath.Join(root, marker), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			sub := filepath.Join(root, "src", "app")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			prev, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(sub); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(prev) })
			got, err := RepoRoot()
			if err != nil || got != root {
				t.Fatalf("RepoRoot from %s = %q, %v; want %q", sub, got, err, root)
			}
		})
	}
}
