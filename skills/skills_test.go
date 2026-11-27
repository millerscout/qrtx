package skills

import (
	"encoding/json"
	"fmt"
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

func TestInstallKeep(t *testing.T) {
	dir := t.TempDir()
	if _, err := InstallKeep(dir); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(dir, "sdd-propose", "SKILL.md")
	_ = os.WriteFile(edited, []byte("local\n"), 0o644)
	_ = os.RemoveAll(filepath.Join(dir, "sdd-archive"))
	res, err := InstallKeep(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range res {
		got[r.Name] = r.Action
	}
	if got["sdd-propose"] != "skipped" || got["sdd-apply"] != "unchanged" || got["sdd-archive"] != "created" {
		t.Fatalf("%v", got)
	}
	if b, _ := os.ReadFile(edited); string(b) != "local\n" {
		t.Fatal("local edit overwritten")
	}
}

func TestHooksInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.local.json")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	orig := `{"permissions":{"allow":["Bash(go test:*)"]},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo pre"}]}]}}`
	_ = os.WriteFile(path, []byte(orig), 0o644)
	changed, err := InstallHooks(path, "qrtx")
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	raw, _ := os.ReadFile(path)
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if s["permissions"] == nil || !strings.Contains(string(raw), "echo pre") {
		t.Fatalf("lost existing settings:\n%s", raw)
	}
	for _, want := range []string{`"SessionStart"`, `"UserPromptSubmit"`, `"PostToolUse"`, `"SessionEnd"`, "qrtx sync begin --hook", "qrtx sync heartbeat --hook --quiet", "qrtx sync inbox --hook --brief", "qrtx sync end --hook", `"async": true`, `"matcher": "*"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s:\n%s", want, raw)
		}
	}
	if strings.Count(string(raw), "qrtx sync heartbeat --hook --quiet") != 2 {
		t.Fatalf("heartbeat should appear for two events:\n%s", raw)
	}
	changed, err = InstallHooks(path, "qrtx")
	if err != nil || changed {
		t.Fatalf("re-run: %v %v", changed, err)
	}
	if again, _ := os.ReadFile(path); string(again) != string(raw) {
		t.Fatal("re-run rewrote the file")
	}
}

func TestHooksInstallKeepsSddHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.local.json")
	orig := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"sdd sync begin --hook"}]}],` +
		`"UserPromptSubmit":[{"hooks":[{"type":"command","command":"\"/x/sdd\" sync heartbeat --hook --quiet"}]}]}}`
	_ = os.WriteFile(path, []byte(orig), 0o644)
	if changed, err := InstallHooks(path, "qrtx"); err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	raw, _ := os.ReadFile(path)
	got := string(raw)
	for _, want := range []string{`"sdd sync begin --hook"`, `\"/x/sdd\" sync heartbeat --hook --quiet`, "qrtx sync begin --hook", "qrtx sync heartbeat --hook --quiet"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s:\n%s", want, got)
		}
	}
	if changed, err := InstallHooks(path, "qrtx"); err != nil || changed {
		t.Fatalf("re-run: %v %v", changed, err)
	}
}

func TestHasSyncHooks(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]bool{
		`{"hooks":{"SessionEnd":[{"hooks":[{"type":"command","command":"qrtx sync end --hook"}]}]}}`:                           true,
		`{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"\"/x/sdd\" sync heartbeat --hook --quiet"}]}]}}`: false,
		`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"sdd change list"}]}]}}`:                              false,
		`{"permissions":{}}`: false,
		``:                   false,
	}
	i := 0
	for body, want := range cases {
		i++
		path := filepath.Join(dir, fmt.Sprintf("s%d.json", i))
		_ = os.WriteFile(path, []byte(body), 0o644)
		if got, err := HasSyncHooks(path); err != nil || got != want {
			t.Fatalf("%s: got %v %v, want %v", body, got, err, want)
		}
	}
	if got, err := HasSyncHooks(filepath.Join(dir, "missing.json")); err != nil || got {
		t.Fatalf("missing file: %v %v", got, err)
	}
}
