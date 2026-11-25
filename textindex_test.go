package sdd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTextIndexSymbols(t *testing.T) {
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
	write("game/board.go", "package game\n\n// Board holds cells.\ntype Board struct{}\n\nfunc (b *Board) Place() {}\n")
	write("game/board_test.go", "package game\n\nfunc Place() {}\n")
	write("web/ui.ts", "export function renderBoard() {}\n")
	write("node_modules/x/index.js", "function renderBoard() {}\n")
	write("notes.md", "func Board\n")
	ix := TextIndex{Root: root}

	hits := ix.Symbols("", "Board", 5)
	if len(hits) != 1 || hits[0].Path != "game/board.go" || hits[0].Line != 4 {
		t.Fatalf("Board hits %+v", hits)
	}
	hits = ix.Symbols("", "Place", 5)
	if len(hits) != 1 || hits[0].Path != "game/board.go" || hits[0].Line != 6 {
		t.Fatalf("Place hits %+v (method def, test file skipped)", hits)
	}
	hits = ix.Symbols("", "renderBoard", 5)
	if len(hits) != 1 || hits[0].Path != "web/ui.ts" {
		t.Fatalf("renderBoard hits %+v (node_modules skipped)", hits)
	}
	if got := ix.Symbols("", "Bo.rd", 5); got != nil {
		t.Fatalf("non-identifier should not search: %+v", got)
	}
	if got := ix.Symbols("", "Board", 0); got != nil {
		t.Fatalf("limit 0: %+v", got)
	}
}

// Propose code lookups use the installed TextIndex for exact names.
func TestTextIndexFeedsProposeCodeHits(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "game"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "game", "board.go"), []byte("package game\n\ntype BoardState [9]byte\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev, prevSym := codeIndex, querySymbolHits
	SetCodeIndex(TextIndex{Root: root})
	querySymbolHits = indexSymbolHits // TestMain stubs it
	t.Cleanup(func() { codeIndex, querySymbolHits = prev, prevSym })
	hits := collectCodeHits("context", "demo", "report a winner on BoardState")
	if len(hits) != 1 || hits[0].Path != "game/board.go" || hits[0].Source != "symbol" {
		t.Fatalf("hits %+v", hits)
	}
}
