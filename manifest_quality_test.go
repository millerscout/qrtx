package sdd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScoreChangeManifestGoodFixture(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join("testdata", "fixtures", "sdd_manifest_quality", "intent-good")
	if err := copyTree(src, root); err != nil {
		t.Fatal(err)
	}
	sc, err := ScoreChangeManifest(root, "intent-good")
	if err != nil {
		t.Fatal(err)
	}
	if !sc.VerifyOK {
		t.Fatalf("verify: %s", sc.VerifyErr)
	}
	if sc.Score < 70 {
		t.Fatalf("score=%d want >= 70", sc.Score)
	}
}

func TestScoreChangeManifestBadFixture(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join("testdata", "fixtures", "sdd_manifest_quality", "intent-bad-missing-acceptance")
	if err := copyTree(src, root); err != nil {
		t.Fatal(err)
	}
	sc, err := ScoreChangeManifest(root, "intent-bad-missing-acceptance")
	if err != nil {
		t.Fatal(err)
	}
	if sc.VerifyOK {
		t.Fatal("expected verify fail")
	}
	if sc.Score > 70 {
		t.Fatalf("score=%d want <= 70", sc.Score)
	}
}

func TestFailureRestatementScore(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "drops-copy", restatementManifest)
	sc, err := ScoreChangeManifest(root, "drops-copy")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Score > 70 {
		t.Fatalf("restatement score=%d", sc.Score)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "read.md"), []byte("read\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, root, "drops-decided", decidedManifest)
	sc, err = ScoreChangeManifest(root, "drops-decided")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Score < 95 {
		t.Fatalf("decided score=%d verify=%s", sc.Score, sc.VerifyErr)
	}
	ground := filepath.Join("testdata", "fixtures", "sdd_manifest_quality", "intent-grounded")
	copyRoot := t.TempDir()
	if err := copyTree(ground, copyRoot); err != nil {
		t.Fatal(err)
	}
	sc, err = ScoreChangeManifest(copyRoot, "intent-grounded")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Score < 95 {
		t.Fatalf("grounded score=%d", sc.Score)
	}
}

func writeManifest(t *testing.T, root, slug, body string) {
	t.Helper()
	dir := filepath.Join(root, "sdd", "changes", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "change.sdd.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const restatementManifest = "```change\nname: drops-copy\nstatus: planning\npackages: []\n```\n\n## User request\n\nthe search drops results\n\n## Acceptance\n\n- the search drops results\n\n## Observed\n\n- The last row was a discard.\n\n## Tasks\n\n- [ ] 1.1 needs user: what did the failing check print?\n\n## Validate\n\n- [ ] V1.1 manual: confirm the search drops results\n"

const decidedManifest = "```change\nname: drops-decided\nstatus: planning\npackages: []\n```\n\n## User request\n\nthe search drops results\n\n## Acceptance\n\n- A regression test fails on the reported failure and passes after the fix.\n\n## Observed\n\n- `docs/read.md` is in this tree.\n\n## Tasks\n\n- [ ] 1.1 Add a regression test that fails on the reported failure and passes after the fix.\n\n## Validate\n\n- [ ] V1.1 go test ./internal/sdd/ -count=1\n"

func TestProductEchoScore(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "read.md"), []byte("read\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, root, "product-echo", productEchoManifest)
	echo, err := ScoreChangeManifest(root, "product-echo")
	if err != nil {
		t.Fatal(err)
	}
	if echo.Score > 70 {
		t.Fatalf("echo score=%d", echo.Score)
	}
	writeManifest(t, root, "one-outcome", productOneOutcomeManifest)
	one, err := ScoreChangeManifest(root, "one-outcome")
	if err != nil {
		t.Fatal(err)
	}
	if one.Score != echo.Score+15 {
		t.Fatalf("one outcome score=%d echo=%d", one.Score, echo.Score)
	}
	decided := filepath.Join("testdata", "fixtures", "sdd_manifest_quality", "failure-decided")
	copyRoot := t.TempDir()
	if err := copyTree(decided, copyRoot); err != nil {
		t.Fatal(err)
	}
	sc, err := ScoreChangeManifest(copyRoot, "failure-decided")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Score < 95 {
		t.Fatalf("failure-decided score=%d", sc.Score)
	}
}

const productEchoManifest = "```change\nname: product-echo\nstatus: planning\npackages: []\n```\n\n## User request\n\nwe need a tic tac toe game\n\na game in html\n\n## Acceptance\n\n- we need a tic tac toe game in html\n\n## Observed\n\n- `docs/read.md` is in this tree.\n\n## Tasks\n\n- [ ] 1.1 Draw the board in html\n\n## Validate\n\n- [ ] V1.1 manual: confirm we need a tic tac toe game in html\n"

const productOneOutcomeManifest = "```change\nname: one-outcome\nstatus: planning\npackages: []\n```\n\n## User request\n\nwe need a tic tac toe game\n\na game in html\n\n## Acceptance\n\n- we need a tic tac toe game in html and an empty board\n\n## Observed\n\n- `docs/read.md` is in this tree.\n\n## Tasks\n\n- [ ] 1.1 Draw the board in html\n\n## Validate\n\n- [ ] V1.1 manual: confirm we need a tic tac toe game in html\n"

func TestLintActiveChanges(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join("testdata", "fixtures", "sdd_manifest_quality", "intent-good")
	if err := copyTree(src, root); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := LintActiveChanges(root, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "intent-good") {
		t.Fatalf("output=%q", buf.String())
	}
}
