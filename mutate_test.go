package qrtx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMutatePackageRoundTrip(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "internal", "pkgok")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"contract.sdd.md", "pkgok.go"} {
		raw, err := os.ReadFile(filepath.Join("testdata", "pkgok", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, name), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := MutatePackage(root, "internal/pkgok"); err != nil {
		t.Fatal(err)
	}
}
