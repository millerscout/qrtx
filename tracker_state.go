package sdd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// trackerStateDir is where sdd keeps tracker state that is never committed:
// the outbox and lock files. Inside git it is <git common dir>/sdd, shared by
// every worktree; outside git it is <repoRoot>/.sdd. It is not created here.
func trackerStateDir(repoRoot string) (dir string, inGit bool) {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err == nil {
		common := strings.TrimSpace(string(out))
		if common != "" {
			if !filepath.IsAbs(common) {
				common = filepath.Join(repoRoot, common)
			}
			return filepath.Join(common, "sdd"), true
		}
	}
	return filepath.Join(repoRoot, ".sdd"), false
}

// ensureTrackerStateDir creates the state directory on first use. Outside
// git it also writes a .gitignore so the folder is never committed.
func ensureTrackerStateDir(repoRoot string) (string, error) {
	dir, inGit := trackerStateDir(repoRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if !inGit {
		ign := filepath.Join(dir, ".gitignore")
		if _, err := os.Stat(ign); os.IsNotExist(err) {
			if err := os.WriteFile(ign, []byte("*\n"), 0o644); err != nil {
				return "", err
			}
		}
	}
	return dir, nil
}

// rootKey names per-root files inside the shared state directory.
func rootKey(repoRoot string) string {
	abs, err := filepath.Abs(repoRoot)
	if err == nil {
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			abs = r
		}
		repoRoot = abs
	}
	sum := sha256.Sum256([]byte(repoRoot))
	return hex.EncodeToString(sum[:])[:12]
}

// withStateLock runs fn while holding the named lock in the state directory.
func withStateLock(repoRoot, name string, fn func() error) error {
	dir, err := ensureTrackerStateDir(repoRoot)
	if err != nil {
		return err
	}
	unlock, err := lockFile(filepath.Join(dir, name+".lock"))
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}
