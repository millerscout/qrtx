package qrtx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractValidateCommand(t *testing.T) {
	cmd, skip, err := extractValidateCommand("V1.1 `go test ./internal/sdd/ -run TestFoo`")
	if err != nil || skip || cmd != "go test ./internal/sdd/ -run TestFoo" {
		t.Fatalf("got cmd=%q skip=%v err=%v", cmd, skip, err)
	}
	_, skip, err = extractValidateCommand("V1.2 manual: read diff")
	if err != nil || !skip {
		t.Fatalf("manual skip=%v err=%v", skip, err)
	}
	_, _, err = extractValidateCommand("V1.1 (TBD — e.g. go test)")
	if err == nil {
		t.Fatal("expected template error")
	}
}

func TestAllowedValidateCommand(t *testing.T) {
	if !allowedValidateCommand("go test ./internal/sdd/ -count=1", false, nil) {
		t.Fatal("go test should be allowed")
	}
	if allowedValidateCommand("rm -rf /", false, nil) {
		t.Fatal("rm should be rejected")
	}
	if !allowedValidateCommand("rm -rf /", true, nil) {
		t.Fatal("escape hatch should allow")
	}
	for _, c := range []string{"qrtx change verify --change x", "./qrtx change verify --change x", "sdd change verify --change x", "SDD_REPO=. sdd change verify --change x"} {
		if !allowedValidateCommand(c, false, nil) {
			t.Fatalf("%q should be allowed", c)
		}
	}
	os.Unsetenv(EnvRunValidate)
}

func TestAllowedValidateCommandRepoPrefixes(t *testing.T) {
	extra := []string{"scripts/check.sh"}
	if allowedValidateCommand("scripts/check.sh --quick", false, nil) {
		t.Fatal("unlisted script should be rejected")
	}
	if !allowedValidateCommand("scripts/check.sh --quick", false, extra) {
		t.Fatal("validate_allow prefix should be allowed")
	}
	if !allowedValidateCommand("FOO=1 scripts/check.sh", false, extra) {
		t.Fatal("env prefix should be stripped before matching")
	}
}

func TestLoadRepoConfig(t *testing.T) {
	root := t.TempDir()
	cfg, err := LoadRepoConfig(root)
	if err != nil || cfg.QuickCheck != "" || len(cfg.ValidateAllow) != 0 {
		t.Fatalf("missing file: %+v %v", cfg, err)
	}
	body := "validate_allow:\n  - scripts/check.sh\nquick_check: bash scripts/check.sh --quick\n"
	if err := os.WriteFile(root+"/"+RepoConfigFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadRepoConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.QuickCheck != "bash scripts/check.sh --quick" || len(cfg.ValidateAllow) != 1 {
		t.Fatalf("cfg %+v", cfg)
	}
	if err := os.WriteFile(root+"/"+RepoConfigFile, []byte("validate_allow: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRepoConfig(root); err == nil {
		t.Fatal("bad yaml should error")
	}
}

func TestRunValidateTasksSkipsManual(t *testing.T) {
	root := t.TempDir()
	slug := "run-val-manual"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	path := changeFilePath(root, slug)
	raw, _ := os.ReadFile(path)
	text := string(raw)
	text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "User goal.", 1)
	text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", "- Done.", 1)
	text = RemovePlaceholderSpecSections(text) // legacy manifest: no spec sections
	text = strings.Replace(text, "- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)", "- [ ] V1.1 manual: operator confirms", 1)
	text = strings.Replace(text, "- [ ] V1.2 (optional) golden row in scripts/fixtures/ if behavior is user-visible", "", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := RunValidateTasks(root, slug, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "skipped=1") {
		t.Fatalf("out=%q errOut=%q", out.String(), errOut.String())
	}
}

func TestRunValidateTasksRejectsDisallowed(t *testing.T) {
	root := t.TempDir()
	slug := "run-val-bad"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	path := changeFilePath(root, slug)
	raw, _ := os.ReadFile(path)
	text := string(raw)
	text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "User goal.", 1)
	text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", "- Done.", 1)
	text = RemovePlaceholderSpecSections(text) // legacy manifest: no spec sections
	text = strings.Replace(text, "- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)", "- [ ] V1.1 `curl http://evil`", 1)
	text = strings.Replace(text, "- [ ] V1.2 (optional) golden row in scripts/fixtures/ if behavior is user-visible", "", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	err := RunValidateTasks(root, slug, os.Stdout, os.Stderr)
	if err == nil || !strings.Contains(err.Error(), "allowlisted") {
		t.Fatalf("err=%v", err)
	}
}

func TestFinishProposeFailsOnScaffold(t *testing.T) {
	root := t.TempDir()
	slug := "finish-bad"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	err := FinishPropose(root, slug, &out, &errOut)
	if err == nil {
		t.Fatal("expected verify failure")
	}
}

func TestFinishProposeOK(t *testing.T) {
	root := t.TempDir()
	slug := "finish-ok"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	path := changeFilePath(root, slug)
	raw, _ := os.ReadFile(path)
	text := string(raw)
	text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "Fix the comment in internal/sdd/finish.go.", 1)
	text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", "- Verify passes.", 1)
	text = RemovePlaceholderSpecSections(text) // legacy manifest: no spec sections
	text = strings.Replace(text, "- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)", "- [x] V1.1 go test ./internal/sdd/", 1)
	text = strings.Replace(text, "- [ ] V1.2 (optional) golden row in scripts/fixtures/ if behavior is user-visible", "- [x] V1.2 manual: ok", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(root, "sdd"), 0o755)
	var out, errOut bytes.Buffer
	if err := FinishPropose(root, slug, &out, &errOut); err != nil {
		t.Fatalf("FinishPropose: %v stderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), `"score"`) {
		t.Fatalf("out=%s", out.String())
	}
}

func TestFinishProposePhases(t *testing.T) {
	prevR, prevE := queryRelatedHits, queryEmbeddingHits
	t.Cleanup(func() {
		queryRelatedHits = prevR
		queryEmbeddingHits = prevE
	})
	queryRelatedHits = func(string, string, int) []CodeHit { return nil }
	queryEmbeddingHits = func(string, string, int) []CodeHit { return nil }

	root := t.TempDir()
	if err := NewChange(root, "phase-context"); err != nil {
		t.Fatal(err)
	}
	writeProposeFixture(t, root, "phase-context", "Related queries return nothing", "- One check.", "### Phase One\n- [ ] 1.1 a\n- [ ] 1.2 b\n- [ ] 1.3 c\n- [ ] 1.4 d\n")
	var out, errOut bytes.Buffer
	err := FinishPropose(root, "phase-context", &out, &errOut)
	if err != nil {
		t.Fatalf("four tasks: %v stderr=%s out=%s", err, errOut.String(), out.String())
	}
	if !strings.Contains(out.String(), `"score"`) {
		t.Fatalf("score JSON missing: %s", out.String())
	}

	if err := NewChange(root, "phase-ready"); err != nil {
		t.Fatal(err)
	}
	writeProposeFixture(t, root, "phase-ready", "Related queries return nothing", "- One check.", "### Phase One\n- [x] 1.1 Done\n")
	out.Reset()
	errOut.Reset()
	if err := FinishPropose(root, "phase-ready", &out, &errOut); err != nil {
		t.Fatalf("ready: %v stderr=%s out=%s", err, errOut.String(), out.String())
	}
	if !strings.Contains(out.String(), `"score"`) {
		t.Fatalf("ready score missing: %s", out.String())
	}

	if err := NewChange(root, "phase-simple"); err != nil {
		t.Fatal(err)
	}
	writeProposeFixture(t, root, "phase-simple", "Fix the comment in internal/sdd/finish.go.", "- One check.", "- [x] 1.1 Done\n")
	out.Reset()
	errOut.Reset()
	if err := FinishPropose(root, "phase-simple", &out, &errOut); err != nil {
		t.Fatalf("simple: %v stderr=%s out=%s", err, errOut.String(), out.String())
	}
}

func writeProposeFixture(t *testing.T, root, slug, request, acceptance, tasks string) {
	t.Helper()
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", request, 1)
	text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", acceptance, 1)
	text = RemovePlaceholderSpecSections(text) // legacy manifest: no spec sections
	text = strings.Replace(text, "- [ ] 1.1 First implementation task", strings.TrimRight(tasks, "\n"), 1)
	text = strings.Replace(text, "- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)", "- [x] V1.1 manual: ok", 1)
	text = strings.Replace(text, "- [ ] V1.2 (optional) golden row in scripts/fixtures/ if behavior is user-visible", "", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
