package qrtx

import (
	"os"
	"path/filepath"
	"testing"
)

// The graph follows imports of whatever module the repo declares.
func TestBuildGraphUsesRepoModulePath(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/demo\n\ngo 1.22\n")
	write("a/contract.sdd.md", "# a\n")
	write("a/a.go", "package a\n\nimport _ \"example.com/demo/b\"\n")
	write("b/contract.sdd.md", "# b\n")
	write("b/b.go", "package b\n")
	g, err := BuildGraph(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Edges) != 1 || g.Edges[0].From != "a" || g.Edges[0].To != "b" {
		t.Fatalf("edges %+v", g.Edges)
	}
}
