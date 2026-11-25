package sdd

import (
	"fmt"
	"strings"
)

// MutatePackage syncs contract export signatures from Go AST and verifies the package.
func MutatePackage(repoRoot, relPkg string) error {
	relPkg = strings.Trim(strings.TrimSpace(relPkg), "/")
	if relPkg == "" {
		return fmt.Errorf("sdd mutate: missing package path")
	}
	if err := SyncPackageFromCode(repoRoot, relPkg); err != nil {
		return err
	}
	issues, err := VerifyPackage(repoRoot, relPkg)
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		return fmt.Errorf("sdd mutate: verify failed after sync for %s (%d issues)", relPkg, len(issues))
	}
	return nil
}
