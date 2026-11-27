package qrtx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// groundingFixture copies intent-grounded into root under slug and swaps its Observed bullet.
func groundingFixture(t *testing.T, root, slug, observed string) {
	t.Helper()
	src := filepath.Join("testdata", "fixtures", "sdd_manifest_quality", "intent-grounded", "sdd", "changes", "intent-grounded", "change.sdd.md")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(raw), "intent-grounded", slug)
	text = strings.Replace(text, "- `docs/read.md` is in this fixture tree.", observed, 1)
	dir := filepath.Join(root, "sdd", "changes", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "change.sdd.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestObservedGroundingEscapes(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "read.md"), []byte("read\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skipOnWindows(t, "symlinks")
	if err := os.Symlink(outside, filepath.Join(root, "docs", "link.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		observed string
		want     bool
	}{
		{"go-mod", "- `go.mod` is at the repo root.", false},
		{"parent", "- `../outside.go` sits beside the repo.", false},
		{"absolute", "- `" + outside + "` sits beside the repo.", false},
		{"symlink-out", "- `docs/link.go` points outside the repo.", false},
		{"in-repo", "- `docs/read.md` is in the repo.", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			slug := "escape-" + tc.name
			groundingFixture(t, root, slug, tc.observed)
			sc, err := ScoreChangeManifest(root, slug)
			if err != nil {
				t.Fatal(err)
			}
			if sc.ObservedGrounded != tc.want {
				t.Fatalf("ObservedGrounded=%v want %v (score %d)", sc.ObservedGrounded, tc.want, sc.Score)
			}
			if !tc.want && sc.Score > 94 {
				t.Fatalf("score=%d want <= 94 when Observed is not grounded", sc.Score)
			}
		})
	}
	t.Run("symlinked-root", func(t *testing.T) {
		link := filepath.Join(base, "repo-link")
		skipOnWindows(t, "symlinks")
		if err := os.Symlink(root, link); err != nil {
			t.Fatal(err)
		}
		groundingFixture(t, link, "via-link", "- `docs/read.md` is in the repo.")
		sc, err := ScoreChangeManifest(link, "via-link")
		if err != nil {
			t.Fatal(err)
		}
		if !sc.ObservedGrounded {
			t.Fatalf("file reached through a symlinked root should ground (score %d)", sc.Score)
		}
	})
}

func TestScoreChangeManifestTaskPaths(t *testing.T) {
	const tasks = "- [ ] 1.1 Change `docs/read.md` and `docs/new.md`. verify: `go test ./cli/` and `cli/cli_test.go`. covers S1\n"
	inline := t.TempDir()
	groundingFixture(t, inline, "inline", "- `docs/read.md` is in the repo.")
	setTasks(t, inline, "inline", tasks)
	roadmap := t.TempDir()
	groundingFixture(t, roadmap, "roadmap", "- `docs/read.md` is in the repo.")
	setTasks(t, roadmap, "roadmap", "### Phase One\n{: id=\"phase-01\" status=\"review\" file=\"phases/01-one.md\"}\n")
	phases := filepath.Join(roadmap, "sdd", "changes", "roadmap", "phases")
	if err := os.MkdirAll(phases, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phases, "01-one.md"), []byte("# Phase One\n\n"+tasks), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ root, slug string }{{inline, "inline"}, {roadmap, "roadmap"}} {
		before, err := ScoreChangeManifest(tc.root, tc.slug)
		if err != nil {
			t.Fatal(err)
		}
		if before.TasksPathsTotal != 2 || before.TasksPathsObserved != 1 {
			t.Fatalf("%s: task paths %d/%d want 1/2", tc.slug, before.TasksPathsObserved, before.TasksPathsTotal)
		}
	}
	// Score must not move with the counts: same plan, Observed matching no task path.
	base := t.TempDir()
	groundingFixture(t, base, "inline", "- `docs/read.md` is in the repo.")
	setTasks(t, base, "inline", "- [ ] 1.1 Change `docs/other.md`. verify: `go test ./cli/`. covers S1\n")
	a, _ := ScoreChangeManifest(inline, "inline")
	b, _ := ScoreChangeManifest(base, "inline")
	if a.Score != b.Score {
		t.Fatalf("score moved with task paths: %d vs %d", a.Score, b.Score)
	}
}

func setTasks(t *testing.T, root, slug, tasks string) {
	t.Helper()
	path := filepath.Join(root, "sdd", "changes", slug, "change.sdd.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "## Tasks")
	end := strings.Index(text, "## Validate")
	text = text[:start] + "## Tasks\n\n" + tasks + "\n" + text[end:]
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "docs", "read.md"), []byte("read\n"), 0o644)
}

// TestTestdataSurvivesModuleZip fails on anything under testdata/ that Go's module zip
// (golang.org/x/mod/zip) leaves out, so a consumer running `go test github.com/millerscout/qrtx`
// sees the same fixtures as this repository: nested modules, vendor/ and VCS folders, symlinks.
func TestTestdataSurvivesModuleZip(t *testing.T) {
	vcs := map[string]bool{".git": true, ".hg": true, ".bzr": true, ".svn": true}
	err := filepath.WalkDir("testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case d.IsDir() && (d.Name() == "vendor" || vcs[d.Name()]):
			t.Errorf("%s: the module zip leaves this folder out", path)
			return filepath.SkipDir
		case d.Type()&os.ModeSymlink != 0:
			t.Errorf("%s: the module zip leaves symlinks out", path)
		case !d.IsDir() && !d.Type().IsRegular():
			t.Errorf("%s: the module zip leaves non-regular files out", path)
		case !d.IsDir() && d.Name() == "go.mod":
			t.Errorf("%s: a go.mod makes its folder a separate module, left out of the module zip", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
