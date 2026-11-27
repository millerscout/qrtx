package qrtx

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DefaultVerifyPackages lists the packages verified when none are named:
// verify_packages from .sdd.yaml, or else every package with a contract.
func DefaultVerifyPackages(repoRoot string) ([]string, error) {
	cfg, err := LoadRepoConfig(repoRoot)
	if err != nil {
		return nil, err
	}
	if len(cfg.VerifyPackages) > 0 {
		return cfg.VerifyPackages, nil
	}
	return FindContractPackages(repoRoot)
}

// DriftIssue names one contract mismatch.
type DriftIssue struct {
	Package string
	Symbol  string
	Detail  string
}

// VerifyPackage compares contract.sdd.md to Go exports under repoRoot/relPkg.
func VerifyPackage(repoRoot, relPkg string) ([]DriftIssue, error) {
	relPkg = strings.Trim(relPkg, "/")
	dir := filepath.Join(repoRoot, relPkg)
	contractPath := filepath.Join(dir, contractFileName)
	contract, err := ParseContractFile(contractPath)
	if err != nil {
		return nil, err
	}
	if contract.Package != filepath.Base(relPkg) {
		return nil, fmt.Errorf("%s: contract package %q != %q", contractPath, contract.Package, filepath.Base(relPkg))
	}
	exports, err := loadPackageExports(dir)
	if err != nil {
		return nil, err
	}
	byName := map[string]exportSymbol{}
	for _, e := range exports {
		byName[e.Name] = e
	}
	var issues []DriftIssue
	for _, want := range contract.Exports {
		got, ok := byName[want.Name]
		if !ok {
			issues = append(issues, DriftIssue{
				Package: relPkg,
				Symbol:  want.Name,
				Detail:  "missing exported symbol",
			})
			continue
		}
		wantSig := normalizeSig(want.Signature)
		gotSig := normalizeSig(got.Signature)
		if wantSig != "" && wantSig != gotSig {
			issues = append(issues, DriftIssue{
				Package: relPkg,
				Symbol:  want.Name,
				Detail:  fmt.Sprintf("signature mismatch: contract %q got %q", wantSig, gotSig),
			})
		}
	}
	return issues, nil
}

// VerifyPackages runs drift check for each package path relative to repoRoot.
func VerifyPackages(repoRoot string, packages []string) ([]DriftIssue, error) {
	var all []DriftIssue
	for _, pkg := range packages {
		issues, err := VerifyPackage(repoRoot, pkg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pkg, err)
		}
		all = append(all, issues...)
	}
	return all, nil
}

// VerifyAST writes results to w and returns an error if drift was found.
func VerifyAST(w io.Writer, repoRoot string, packages []string) error {
	if len(packages) == 0 {
		var err error
		if packages, err = DefaultVerifyPackages(repoRoot); err != nil {
			return err
		}
	}
	issues, err := VerifyPackages(repoRoot, packages)
	if err != nil {
		return err
	}
	if len(issues) == 0 {
		_, _ = fmt.Fprintf(w, "qrtx verify-ast: OK (%d packages)\n", len(packages))
		return nil
	}
	for _, iss := range issues {
		_, _ = fmt.Fprintf(w, "drift %s %s: %s\n", iss.Package, iss.Symbol, iss.Detail)
	}
	return fmt.Errorf("%d drift issue(s)", len(issues))
}

// rootMarkers mark a repository root; the nearest directory holding any wins.
var rootMarkers = []string{RepoConfigFile, "go.mod", ".git"}

// RepoRoot is the project whose sdd/changes git should track.
// $QRTX_REPO (or another name in RepoEnv) overrides it. Otherwise the nearest
// directory with .sdd.yaml, go.mod or .git wins, and the working directory
// is used when there is none.
func RepoRoot() (string, error) {
	if r := firstEnv(RepoEnv); r != "" {
		return filepath.Abs(r)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		for _, marker := range rootMarkers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return wd, nil
}
