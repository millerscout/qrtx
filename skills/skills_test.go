package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	res, err := Install(dir, false)
	if err != nil || len(res) != len(Names) || res[0].Action != "created" {
		t.Fatalf("first install: %+v %v", res, err)
	}
	res, err = Install(dir, false)
	if err != nil || res[0].Action != "unchanged" {
		t.Fatalf("second install: %+v %v", res, err)
	}
	edited := filepath.Join(dir, "sdd-apply", "SKILL.md")
	if err := os.WriteFile(edited, []byte("local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(dir, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("edited file should need --force: %v", err)
	}
	if got, _ := os.ReadFile(edited); string(got) != "local edit\n" {
		t.Fatal("edited file was overwritten without --force")
	}
	res, err = Install(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Name == "sdd-apply" && r.Action != "updated" {
			t.Fatalf("force: %+v", r)
		}
	}
}

func TestInstallRefusesSymlinkedSkillDir(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("host overlay\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "sdd-propose")); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(dir, true); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("want symlink refusal, got %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(target, "SKILL.md")); string(got) != "host overlay\n" {
		t.Fatal("wrote through the symlink")
	}
}

var hostWordRE = regexp.MustCompile(`(?i)\b(aurora|aurora-llm|laya|zed|playbook)\b|` + "`terminal`")

func TestSkillsHaveFrontmatterAndNoHostNames(t *testing.T) {
	for _, name := range Names {
		raw, err := Read(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if !strings.HasPrefix(text, "---\nname: "+name+"\n") {
			t.Fatalf("%s: frontmatter must start with name: %s", name, name)
		}
		if m := hostWordRE.FindString(text); m != "" {
			t.Fatalf("%s mentions %q", name, m)
		}
	}
}
