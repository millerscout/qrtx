package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type verifyASTGoldenRow struct {
	ID         string `json:"id"`
	Package    string `json:"package"`
	WantDrift  bool   `json:"want_drift"`
	WantSymbol string `json:"want_symbol,omitempty"`
}

func TestSddVerifyASTGoldenJSONL(t *testing.T) {
	rows := loadVerifyASTGolden(t)
	root := filepath.Join("testdata")
	for _, row := range rows {
		issues, err := VerifyPackage(root, row.Package)
		if err != nil {
			t.Fatalf("%s: %v", row.ID, err)
		}
		if row.WantDrift {
			if len(issues) == 0 {
				t.Fatalf("%s: expected drift", row.ID)
			}
			if row.WantSymbol != "" {
				found := false
				for _, iss := range issues {
					if iss.Symbol == row.WantSymbol {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("%s: missing symbol %s in %v", row.ID, row.WantSymbol, issues)
				}
			}
			continue
		}
		if len(issues) != 0 {
			t.Fatalf("%s: unexpected drift %v", row.ID, issues)
		}
	}
}

func loadVerifyASTGolden(t *testing.T) []verifyASTGoldenRow {
	path := filepath.Join("testdata", "fixtures", "sdd_verify_ast_golden.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []verifyASTGoldenRow
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row verifyASTGoldenRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("parse: %v", err)
		}
		rows = append(rows, row)
	}
	if len(rows) < 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	return rows
}
