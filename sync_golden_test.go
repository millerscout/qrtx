package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSddSyncFromCodeGoldenJSONL(t *testing.T) {
	path := "testdata/fixtures/sdd_sync_from_code_golden.jsonl"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			ID  string `json:"id"`
			Pkg string `json:"package"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		pkgDir := filepath.Join(root, filepath.FromSlash(row.Pkg))
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"contract.sdd.md", "pkgok.go"} {
			b, err := os.ReadFile(filepath.Join("testdata", "pkgok", name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(pkgDir, name), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := SyncPackageFromCode(root, row.Pkg); err != nil {
			t.Fatalf("%s: %v", row.ID, err)
		}
		issues, err := VerifyPackage(root, row.Pkg)
		if err != nil || len(issues) > 0 {
			t.Fatalf("%s: issues=%v err=%v", row.ID, issues, err)
		}
	}
}
