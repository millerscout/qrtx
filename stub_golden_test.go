package sdd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sddStubGoldenRow struct {
	ID         string `json:"id"`
	WantSubstr string `json:"want_substr"`
}

func TestSddCLIStubGoldenJSONL(t *testing.T) {
	rows := loadSddStubGolden(t)
	var buf bytes.Buffer
	if err := verifyASTStub(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, row := range rows {
		if row.WantSubstr != "" && !strings.Contains(out, row.WantSubstr) {
			t.Fatalf("%s: output missing %q: %s", row.ID, row.WantSubstr, out)
		}
	}
}

func loadSddStubGolden(t *testing.T) []sddStubGoldenRow {
	path := filepath.Join("testdata", "fixtures", "sdd_cli_stub_golden.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []sddStubGoldenRow
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row sddStubGoldenRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("parse: %v", err)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	return rows
}
