package sdd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildGraphIncludesDogfoodEdge(t *testing.T) {
	root, err := RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	g, err := BuildGraph(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Packages) < 2 {
		t.Fatalf("packages=%v", g.Packages)
	}
}

func TestWriteGraph(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "pkgok")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("testdata/pkgok/contract.sdd.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, contractFileName), src, 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := WriteGraph(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
