package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/millerscout/sdd"
	"github.com/millerscout/sdd/skills"
)

func run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := Run(args, &out, &errOut)
	return out.String(), errOut.String(), err
}

func TestChangeLifecycleCommands(t *testing.T) {
	root := t.TempDir()
	t.Setenv(sdd.EnvRepo, root)

	if out, _, err := run(t, "change", "new", "add-winner"); err != nil || !strings.Contains(out, "created sdd/changes/add-winner/") {
		t.Fatalf("new: %q %v", out, err)
	}
	if out, _, err := run(t, "change", "list"); err != nil || strings.TrimSpace(out) != "add-winner" {
		t.Fatalf("list: %q %v", out, err)
	}
	out, _, err := run(t, "change", "status", "--change", "add-winner")
	if err != nil {
		t.Fatal(err)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(out), &status); err != nil || status["status"] != "planning" {
		t.Fatalf("status: %q %v", out, err)
	}
	if _, _, err := run(t, "change", "verify", "--change", "add-winner"); err == nil || !strings.Contains(err.Error(), "TBD") {
		t.Fatalf("verify should reject the template: %v", err)
	}
	if _, _, err := run(t, "change", "archive", "add-winner"); err == nil {
		t.Fatal("archive should refuse a planning change")
	}
	raw, err := os.ReadFile(filepath.Join(root, "sdd", "changes", "add-winner", "change.sdd.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "`"+sdd.CLIName+" change review-packet --change add-winner`") {
		t.Fatal("template should name the CLI")
	}
}

func TestQuickCheckNeedsConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv(sdd.EnvRepo, root)
	if err := sdd.RunQuickCheck(root, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "quick_check") {
		t.Fatalf("want missing-config error, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, sdd.RepoConfigFile), []byte("quick_check: echo quick-ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := sdd.RunQuickCheck(root, &out, &bytes.Buffer{}); err != nil || !strings.Contains(out.String(), "quick-ok") {
		t.Fatalf("quick check: %q %v", out.String(), err)
	}
}

func TestUnknownSubcommand(t *testing.T) {
	_, errOut, err := run(t, "nope")
	if err == nil || !strings.Contains(errOut, "Usage: "+sdd.CLIName) {
		t.Fatalf("usage: %q %v", errOut, err)
	}
}

// Every sdd command the bundled skills tell an agent to run exists.
func TestSkillCommandsExist(t *testing.T) {
	t.Setenv(sdd.EnvRepo, t.TempDir())
	cmdRE := regexp.MustCompile("`sdd ((?:change )?[a-z][a-z-]*)")
	seen := map[string]bool{}
	for _, name := range skills.Names {
		raw, err := skills.Read(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range cmdRE.FindAllStringSubmatch(string(raw), -1) {
			seen[m[1]] = true
		}
	}
	if len(seen) < 8 {
		t.Fatalf("found only %d commands: %v", len(seen), seen)
	}
	for cmd := range seen {
		_, errOut, err := run(t, strings.Fields(cmd)...)
		if err != nil && strings.Contains(err.Error(), "unknown") {
			t.Errorf("skills reference %q: %v %s", cmd, err, errOut)
		}
	}
}

func TestSkillsInstallCommand(t *testing.T) {
	root := t.TempDir()
	t.Setenv(sdd.EnvRepo, root)
	out, _, err := run(t, "skills", "install")
	if err != nil || strings.Count(out, "created ") != len(skills.Names) {
		t.Fatalf("install: %q %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "sdd-propose", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if out, _, err := run(t, "skills", "list"); err != nil || !strings.Contains(out, "sdd-archive") {
		t.Fatalf("list: %q %v", out, err)
	}
}

func TestConstitutionCommands(t *testing.T) {
	root := t.TempDir()
	t.Setenv(sdd.EnvRepo, root)
	if out, _, err := run(t, "constitution", "show"); err != nil || !strings.Contains(out, "no constitution") {
		t.Fatalf("show without one: %q %v", out, err)
	}
	if out, _, err := run(t, "constitution", "show", "--json"); err != nil || strings.TrimSpace(out) != "[]" {
		t.Fatalf("show --json without one: %q %v", out, err)
	}
	if out, _, err := run(t, "constitution", "init"); err != nil || !strings.Contains(out, sdd.DefaultConstitutionFile) {
		t.Fatalf("init: %q %v", out, err)
	}
	if _, _, err := run(t, "constitution", "init"); err == nil {
		t.Fatal("second init without --force should fail")
	}
	out, _, err := run(t, "constitution", "show")
	if err != nil || !strings.Contains(out, "Rule 1: Tests before code") {
		t.Fatalf("show: %q %v", out, err)
	}
	if _, _, err := run(t, "change", "propose-context", "--change", "nope"); err == nil {
		t.Fatal("propose-context on a missing slug should fail")
	}
	if _, _, err := run(t, "change", "new", "add-thing"); err != nil {
		t.Fatal(err)
	}
	out, _, err = run(t, "change", "propose-context", "--change", "add-thing", "--json")
	var rep sdd.ProposeContextReport
	if err != nil || json.Unmarshal([]byte(out), &rep) != nil || len(rep.Constitution) != 1 || rep.Constitution[0].Path != sdd.DefaultConstitutionFile {
		t.Fatalf("propose-context should carry the constitution: %q %v", out, err)
	}
}
