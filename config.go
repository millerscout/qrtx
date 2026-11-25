package sdd

import (
	"os"
	"strings"
)

// Environment variables sdd reads.
const (
	// EnvRepo overrides the repository root (see RepoRoot).
	EnvRepo = "SDD_REPO"
	// EnvRunValidate set to "1" lets run-validate execute commands that are not allowlisted.
	EnvRunValidate = "SDD_RUN_VALIDATE"
)

// Host settings. A host that embeds sdd may change these once at startup.
var (
	// CLIName is the command shown in templates and error messages.
	CLIName = "sdd"
	// DefaultProject names the project for provider lookups when the caller
	// gives none. Empty means the repository directory name.
	DefaultProject string
	// RepoEnv lists variables that override the repository root; the first
	// non-empty one wins.
	RepoEnv = []string{EnvRepo}
	// RunValidateEnv lists variables that, set to "1", let run-validate execute
	// commands that are not allowlisted.
	RunValidateEnv = []string{EnvRunValidate}
)

func firstEnv(names []string) string {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}
