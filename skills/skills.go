// Package skills embeds the agent skills for the sdd workflow
// (sdd-propose, sdd-apply, sdd-archive), one SKILL.md per directory.
package skills

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed sdd-*/SKILL.md
var files embed.FS

// Names lists the embedded skills in workflow order.
var Names = []string{"sdd-propose", "sdd-apply", "sdd-archive"}

// Read returns the SKILL.md for name.
func Read(name string) ([]byte, error) {
	return files.ReadFile(name + "/SKILL.md")
}

// Result says what Install did for one skill.
type Result struct {
	Name   string
	Path   string
	Action string // "created", "updated" or "unchanged"
}

// Install writes each skill to dir/<name>/SKILL.md. A differing file is
// replaced only when force is set. A skill directory that is a symlink is
// never written through.
func Install(dir string, force bool) ([]Result, error) {
	var out []Result
	for _, name := range Names {
		want, err := Read(name)
		if err != nil {
			return out, err
		}
		skillDir := filepath.Join(dir, name)
		if info, err := os.Lstat(skillDir); err == nil && info.Mode()&fs.ModeSymlink != 0 {
			return out, fmt.Errorf("%s is a symlink; remove it or pass another --dir", skillDir)
		}
		path := filepath.Join(skillDir, "SKILL.md")
		action := "created"
		have, err := os.ReadFile(path)
		switch {
		case err == nil && bytes.Equal(have, want):
			out = append(out, Result{name, path, "unchanged"})
			continue
		case err == nil && !force:
			return out, fmt.Errorf("%s differs from the bundled skill; pass --force to replace it", path)
		case err == nil:
			action = "updated"
		case !errors.Is(err, fs.ErrNotExist):
			return out, err
		}
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return out, err
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			return out, err
		}
		out = append(out, Result{name, path, action})
	}
	return out, nil
}

// InstallKeep installs every skill into dir like Install without force, but a
// skill whose local copy differs is reported "skipped" and the others still
// install.
func InstallKeep(dir string) ([]Result, error) {
	var out []Result
	for _, name := range Names {
		want, err := Read(name)
		if err != nil {
			return out, err
		}
		skillDir := filepath.Join(dir, name)
		if info, err := os.Lstat(skillDir); err == nil && info.Mode()&fs.ModeSymlink != 0 {
			out = append(out, Result{name, skillDir, "skipped"})
			continue
		}
		path := filepath.Join(skillDir, "SKILL.md")
		have, err := os.ReadFile(path)
		switch {
		case err == nil && bytes.Equal(have, want):
			out = append(out, Result{name, path, "unchanged"})
			continue
		case err == nil:
			out = append(out, Result{name, path, "skipped"})
			continue
		case !errors.Is(err, fs.ErrNotExist):
			return out, err
		}
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return out, err
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			return out, err
		}
		out = append(out, Result{name, path, "created"})
	}
	return out, nil
}
