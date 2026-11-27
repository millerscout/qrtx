package qrtx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// livingChange builds a complete change citing internal/store/store.go, which has a contract.
func livingChange(t *testing.T) (string, string) {
	t.Helper()
	reqs := "- Q1 Save SHALL keep every write when two sessions save at once."
	scen := "- S1 (Q1) WHEN two sessions save the same project THEN both writes are in the file\n- S2 (Q1) WHEN a save crashes midway THEN the file is the previous version"
	root, slug := reqManifest(t, reqs, scen)
	dir := filepath.Join(root, "internal", "store")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "store.go"), []byte("package store\n\nfunc Save() error { return nil }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	real, err := os.ReadFile(filepath.Join("testdata", "contracts", "living-base.contract.sdd.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, contractFileName), real, 0o644); err != nil {
		t.Fatal(err)
	}
	path := changeFilePath(root, slug)
	raw, _ := os.ReadFile(path)
	text := replaceSection(string(raw), "Observed", "- `internal/store/store.go` holds Save.")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, slug
}

func TestPromoteBehaviourIdempotent(t *testing.T) {
	root, slug := livingChange(t)
	written, err := promoteBehaviour(root, slug, "2026-11-22")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 || written[0] != "internal/store/contract.sdd.md" {
		t.Fatalf("written %v", written)
	}
	again, err := promoteBehaviour(root, slug, "2026-11-23")
	if err != nil || len(again) != 0 {
		t.Fatalf("second promote wrote %v, err %v", again, err)
	}
	path := filepath.Join(root, "internal", "store", contractFileName)
	b, _ := os.ReadFile(path)
	text := string(b)
	if strings.Count(text, "## Behaviour") != 1 || strings.Count(text, "### 2026-11-22 "+slug) != 1 {
		t.Fatalf("contract\n%s", text)
	}
	for _, want := range []string{slug + "/Q1 Save SHALL", slug + "/S2 (Q1) WHEN a save crashes"} {
		if !strings.Contains(text, want) {
			t.Fatalf("contract lacks %q\n%s", want, text)
		}
	}
	if _, err := ParseContractFile(path); err != nil {
		t.Fatalf("contract no longer parses: %v", err)
	}
}

func TestProposeContextShowsExistingBehaviour(t *testing.T) {
	root, slug := livingChange(t)
	if _, err := promoteBehaviour(root, slug, "2026-11-22"); err != nil {
		t.Fatal(err)
	}
	rep, err := ProposeContext(root, slug, ProposeContextOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.ExistingBehaviour) != 1 || rep.ExistingBehaviour[0].Package != "internal/store" || !strings.Contains(rep.ExistingBehaviour[0].Text, slug+"/S1") {
		t.Fatalf("existing behaviour %+v", rep.ExistingBehaviour)
	}
	var buf bytes.Buffer
	if err := WriteProposeContextHuman(&buf, rep); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "existing_behaviour:\n  internal/store:") {
		t.Fatalf("human output\n%s", buf.String())
	}
}

func TestPromoteBehaviourSkipsLegacyManifest(t *testing.T) {
	root := t.TempDir()
	if err := NewChange(root, "legacy"); err != nil {
		t.Fatal(err)
	}
	path := changeFilePath(root, "legacy")
	raw, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(RemovePlaceholderSpecSections(string(raw))), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := promoteBehaviour(root, "legacy", "2026-11-22")
	if err != nil || len(written) != 0 {
		t.Fatalf("written %v err %v", written, err)
	}
}
