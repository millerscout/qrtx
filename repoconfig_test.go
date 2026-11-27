package qrtx

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultVerifyPackages(t *testing.T) {
	root := t.TempDir()
	got, err := DefaultVerifyPackages(root)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty repo: %v %v", got, err)
	}
	for _, pkg := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(root, pkg), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, pkg, contractFileName), []byte("# "+pkg+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err = DefaultVerifyPackages(root)
	if err != nil || !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("contract packages: %v %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(root, RepoConfigFile), []byte("verify_packages: [b]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = DefaultVerifyPackages(root)
	if err != nil || !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("configured packages: %v %v", got, err)
	}
}

func TestStatusForRepoWithNothingToVerify(t *testing.T) {
	ClearStatusCache()
	st, err := StatusForRepo(t.TempDir(), true)
	if err != nil || st.Freshness != "unknown" {
		t.Fatalf("status %+v %v", st, err)
	}
}
