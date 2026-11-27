package qrtx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type manifestQualityGoldenRow struct {
	ID               string `json:"id"`
	Fixture          string `json:"fixture"`
	Slug             string `json:"slug"`
	ExpectPassVerify bool   `json:"expect_pass_verify"`
	ExpectMinScore   int    `json:"expect_min_score"`
	ExpectMaxScore   int    `json:"expect_max_score"`
	MaxWallMs        int    `json:"max_wall_ms"`
}

func TestSddChangeManifestQualityGoldenJSONL(t *testing.T) {
	path := filepath.Join("testdata", "fixtures", "sdd_change_manifest_quality_golden.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fixturesRoot := filepath.Join("testdata", "fixtures", "sdd_manifest_quality")
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row manifestQualityGoldenRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("parse: %v", err)
		}
		t.Run(row.ID, func(t *testing.T) {
			root := t.TempDir()
			src := filepath.Join(fixturesRoot, row.Fixture)
			if err := copyTree(src, root); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			sc, err := ScoreChangeManifest(root, row.Slug)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
			if row.MaxWallMs > 0 && elapsed > time.Duration(row.MaxWallMs)*time.Millisecond {
				t.Fatalf("wall %s exceeds %dms", elapsed, row.MaxWallMs)
			}
			if sc.VerifyOK != row.ExpectPassVerify {
				t.Fatalf("verify_ok=%v want %v err=%s", sc.VerifyOK, row.ExpectPassVerify, sc.VerifyErr)
			}
			if sc.Score < row.ExpectMinScore {
				t.Fatalf("score=%d want >= %d", sc.Score, row.ExpectMinScore)
			}
			if row.ExpectMaxScore > 0 && sc.Score > row.ExpectMaxScore {
				t.Fatalf("score=%d want <= %d", sc.Score, row.ExpectMaxScore)
			}
		})
	}
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
