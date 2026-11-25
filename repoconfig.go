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
	// QuickCheck is a shell command run by `change archive --quick-check`.
	QuickCheck string `yaml:"quick_check"`
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
