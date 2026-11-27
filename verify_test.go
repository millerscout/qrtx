package qrtx

import (
	"path/filepath"
	"testing"
)

func TestVerifyPackageOK(t *testing.T) {
	root := filepath.Join("testdata")
	issues, err := VerifyPackage(root, "pkgok")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues=%v", issues)
	}
}

func TestVerifyPackageDrift(t *testing.T) {
	root := filepath.Join("testdata")
	issues, err := VerifyPackage(root, "pkgdrift")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("expected drift")
	}
}

func TestLoadPackageExports(t *testing.T) {
	dir := filepath.Join("testdata", "pkgok")
	exports, err := loadPackageExports(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, e := range exports {
		names[e.Name] = e.Signature
	}
	if names["Greet"] != "func Greet(name string) string" {
		t.Fatalf("exports=%v", exports)
	}
}
