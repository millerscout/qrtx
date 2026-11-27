package qrtx

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sddChangeGoldenRow struct {
	ID               string `json:"id"`
	ExpectActiveSlug string `json:"expect_active_slug,omitempty"`
	MemorySlug       string `json:"memory_slug,omitempty"`
}

func TestSddChangeCLIGoldenJSONL(t *testing.T) {
	path := filepath.Join("testdata", "fixtures", "sdd_change_cli_golden.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []sddChangeGoldenRow
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row sddChangeGoldenRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("parse: %v", err)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	root := t.TempDir()
	for _, row := range rows {
		t.Run(row.ID, func(t *testing.T) {
			if row.ID == "verify-reject-scaffold" {
				slug := "golden-verify-reject"
				if err := NewChange(root, slug); err != nil {
					t.Fatal(err)
				}
				if err := ValidateChangeManifest(root, slug); err == nil {
					t.Fatal("expected scaffold verify to fail")
				}
				return
			}
			if row.ID == "finish-propose-fail" {
				slug := "golden-finish-fail"
				if err := NewChange(root, slug); err != nil {
					t.Fatal(err)
				}
				if err := FinishPropose(root, slug, io.Discard, io.Discard); err == nil {
					t.Fatal("expected finish-propose to fail on scaffold")
				}
				return
			}
			if row.ID == "run-validate-manual-skip" {
				slug := "golden-run-val-skip"
				if err := NewChange(root, slug); err != nil {
					t.Fatal(err)
				}
				path := changeFilePath(root, slug)
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				text := string(raw)
				text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "User goal.", 1)
				text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", "- Done.", 1)
				text = RemovePlaceholderSpecSections(text) // legacy manifest: no spec sections
				text = strings.Replace(text, "- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)", "- [ ] V1.1 manual: operator confirms", 1)
				text = strings.Replace(text, "- [ ] V1.2 (optional) golden row in scripts/fixtures/ if behavior is user-visible", "", 1)
				if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := RunValidateTasks(root, slug, io.Discard, io.Discard); err != nil {
					t.Fatal(err)
				}
				return
			}
			if row.ID == "verify-accept-minimal" {
				slug := "golden-verify-ok"
				if err := NewChange(root, slug); err != nil {
					t.Fatal(err)
				}
				path := changeFilePath(root, slug)
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				text := string(raw)
				text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "Golden verify passes.", 1)
				text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", "- CLI verify exits 0.", 1)
				text = RemovePlaceholderSpecSections(text) // legacy manifest: no spec sections
				text = strings.Replace(text, "- [ ] 1.1 First implementation task", "- [x] 1.1 Done", 1)
				text = strings.Replace(text, "- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)", "- [x] V1.1 go test ./internal/sdd/ -run TestSddChangeCLIGoldenJSONL", 1)
				text = strings.Replace(text, "- [ ] V1.2 (optional) golden row in scripts/fixtures/ if behavior is user-visible", "- [x] V1.2 manual: golden row present", 1)
				if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := ValidateChangeManifest(root, slug); err != nil {
					t.Fatal(err)
				}
				return
			}
			if row.ID == "stale-pin" {
				if err := NewChange(root, "was-bound"); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(filepath.Join(root, "sdd", "changes", "was-bound")); err != nil {
					t.Fatal(err)
				}
				memSlug := row.MemorySlug
				if memSlug == "" {
					memSlug = "was-bound"
				}
				m := ChangesMap(root, memSlug)
				want := row.ExpectActiveSlug
				if got := m["active_slug"]; got != want {
					t.Fatalf("active_slug=%v want %q", got, want)
				}
				if want == "" && m["stale_pin"] != memSlug {
					t.Fatalf("stale_pin=%v want %q", m["stale_pin"], memSlug)
				}
				return
			}
			slug := "golden-" + row.ID
			if err := NewChange(root, slug); err != nil {
				t.Fatal(err)
			}
			sum, err := SummarizeChange(root, slug)
			if err != nil || sum.TasksTotal < 1 {
				t.Fatalf("sum=%+v err=%v", sum, err)
			}
			m := ChangesMap(root, slug)
			if m["active_slug"] != slug {
				t.Fatalf("active_slug=%v", m["active_slug"])
			}
			_ = os.RemoveAll(filepath.Join(root, "sdd", "changes", slug))
		})
	}
}
