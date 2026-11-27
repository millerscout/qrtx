package qrtx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegressionValidateCommand(t *testing.T) {
	named := RegressionValidateCommand("the search drops results in internal/sdd/roadmap.go", "")
	if named != "go test ./internal/sdd/ -count=1" {
		t.Fatalf("named %q", named)
	}
	if strings.Contains(named, "manual: confirm") {
		t.Fatalf("manual command %q", named)
	}
	plain := RegressionValidateCommand("the search drops results", "")
	if plain != "go test ./... -count=1" {
		t.Fatalf("plain %q", plain)
	}
	if strings.Contains(plain, "manual: confirm") {
		t.Fatalf("manual command %q", plain)
	}
	fromObserved := RegressionValidateCommand("the search drops results", "- internal/ide/sdd_tool.go prints the line")
	if fromObserved != "go test ./internal/ide/ -count=1" {
		t.Fatalf("observed %q", fromObserved)
	}
	if RegressionAcceptance() != "- A regression test fails on the reported failure and passes after the fix." {
		t.Fatal(RegressionAcceptance())
	}
	if !strings.Contains(RegressionTask(), "regression test") || strings.HasPrefix(RegressionTask(), "needs user:") {
		t.Fatal(RegressionTask())
	}
	if !strings.Contains(RegressionWhy(), "set aside") {
		t.Fatal(RegressionWhy())
	}
}

func TestMarkQuestionOnlyPlanning(t *testing.T) {
	only := "### Phase ship\n{: id=\"phase-01\" status=\"review\"}\n\n- [ ] 1.1 needs user: what outcome should \"tic tac toe\" deliver?\n"
	got := MarkQuestionOnlyPlanning(only)
	if strings.Contains(got, `status="review"`) || !strings.Contains(got, `status="planning"`) {
		t.Fatalf("question only\n%s", got)
	}
	later := "### Phase ship\n{: id=\"phase-01\" status=\"review\"}\n\n- [ ] 1.1 needs user: which check belongs\n- [ ] 1.2 Keep the header\n"
	got = MarkQuestionOnlyPlanning(later)
	if !strings.Contains(got, `status="review"`) || strings.Contains(got, `status="planning"`) {
		t.Fatalf("later checkbox\n%s", got)
	}
}

func TestSiblingRequestOldestThenLex(t *testing.T) {
	root := t.TempDir()
	for _, slug := range []string{"beta-plan", "alpha-plan", "newer-plan"} {
		if err := NewChange(root, slug); err != nil {
			t.Fatal(err)
		}
		if err := setUserRequest(root, slug, "check the buffer"); err != nil {
			t.Fatal(err)
		}
	}
	same := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(changeFilePath(root, "alpha-plan"), same, same); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(changeFilePath(root, "beta-plan"), same, same); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(changeFilePath(root, "newer-plan"), same.Add(time.Hour), same.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := SiblingRequestSlug(root, "newer-plan", "check the buffer")
	if err != nil {
		t.Fatal(err)
	}
	if got != "alpha-plan" {
		t.Fatalf("lex %q", got)
	}
	older := same.Add(-time.Hour)
	if err := os.Chtimes(changeFilePath(root, "beta-plan"), older, older); err != nil {
		t.Fatal(err)
	}
	got, err = SiblingRequestSlug(root, "newer-plan", "check the buffer")
	if err != nil {
		t.Fatal(err)
	}
	if got != "beta-plan" {
		t.Fatalf("oldest %q", got)
	}
	rep, err := ProposeContext(root, "newer-plan", ProposeContextOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Overlaps) != 1 || rep.Overlaps[0].OtherSlug != "beta-plan" {
		t.Fatalf("overlaps=%v", rep.Overlaps)
	}
	if len(rep.Warnings) != 1 || rep.Warnings[0] != `request overlap with "beta-plan"` {
		t.Fatalf("warnings=%v", rep.Warnings)
	}
	info, err := os.Stat(filepath.Join(root, "sdd", "changes", "beta-plan", "change.sdd.md"))
	if err != nil || info.IsDir() {
		t.Fatal(err)
	}
}

func setUserRequest(root, slug, request string) error {
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := replaceSection(string(raw), "User request", request)
	return os.WriteFile(path, []byte(text), 0o644)
}
