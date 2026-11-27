package sdd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RepoConfigFile is the optional per-repository settings file at the repo root.
const RepoConfigFile = ".sdd.yaml"

// RepoConfig is the content of .sdd.yaml.
type RepoConfig struct {
	// ValidateAllow lists extra command prefixes run-validate may execute.
	ValidateAllow []string `yaml:"validate_allow"`
	// VerifyPackages lists the packages verify-ast, sync-from-code and status
	// check when none are named. Empty means every package with a contract.
	VerifyPackages []string `yaml:"verify_packages"`
	// Constitution lists the project's rule files, repository-relative.
	// Empty means sdd/constitution.md when it exists.
	Constitution []string `yaml:"constitution"`
	// QuickCheck is a shell command run by `change archive --quick-check`.
	QuickCheck string `yaml:"quick_check"`
	// Project names this project in tracker refs; empty means DefaultProject,
	// then the repository directory name.
	Project string `yaml:"project"`
	// Tracker selects the issue tracker backend. Unstable until v1.
	Tracker TrackerConfig `yaml:"tracker"`
	// Sync tunes session leases and the sync daemon. Unstable until v1.
	Sync SyncConfig `yaml:"sync"`
}

// SyncConfig is the `sync:` block of .sdd.yaml; durations as Go strings.
type SyncConfig struct {
	Lease    string `yaml:"lease"`    // default 1h
	Tick     string `yaml:"tick"`     // default 5s
	Interval string `yaml:"interval"` // remote poll, default 2m
}

// TrackerConfig is the `tracker:` block of .sdd.yaml. LoadRepoConfig only
// decodes it; the tracker commands validate it when they resolve a backend.
type TrackerConfig struct {
	// Backend is "file" (default) or <name> for sdd-tracker-<name> on PATH.
	Backend string `yaml:"backend"`
	// Config is passed verbatim to the plugin. Never put secrets here.
	Config map[string]any `yaml:"config"`
	// StatusMap names the exact native status per category.
	StatusMap map[string]string `yaml:"status_map"`
	// Mirror is the granularity of issues sdd creates; only "change" exists.
	Mirror string `yaml:"mirror"`
	// Comments is summary (default), minimal or off.
	Comments string `yaml:"comments"`
	// Timeout is the per-call plugin timeout in seconds (default 30).
	Timeout int `yaml:"timeout"`
}

// LoadRepoConfig reads repoRoot/.sdd.yaml. A missing file is an empty config.
func LoadRepoConfig(repoRoot string) (RepoConfig, error) {
	var cfg RepoConfig
	raw, err := os.ReadFile(filepath.Join(repoRoot, RepoConfigFile))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", RepoConfigFile, err)
	}
	return cfg, nil
}

// RunQuickCheck runs the repo's quick_check command from repoRoot.
func RunQuickCheck(repoRoot string, stdout, stderr io.Writer) error {
	cfg, err := LoadRepoConfig(repoRoot)
	if err != nil {
		return err
	}
	cmd := strings.TrimSpace(cfg.QuickCheck)
	if cmd == "" {
		return fmt.Errorf("quick-check: set quick_check in %s", RepoConfigFile)
	}
	c := exec.Command("bash", "-lc", cmd)
	c.Dir = repoRoot
	c.Stdout = stdout
	c.Stderr = stderr
	c.Env = os.Environ()
	if err := c.Run(); err != nil {
		return fmt.Errorf("quick-check: %w", err)
	}
	return nil
}
