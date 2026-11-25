package sdd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// checkedScenarioCitations collects S-ids cited by checked (`- [x]`) lines of
// Validate, Tasks and phase task files. It is the apply-time counterpart of
// scenarioCitations, which counts any citation at propose time.
func checkedScenarioCitations(repoRoot, slug, text string) map[string]bool {
	cited := map[string]bool{}
	add := func(s string) {
		for _, line := range strings.Split(s, "\n") {
			m := sddCheckboxRE.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil || !strings.EqualFold(m[1], "x") {
				continue
			}
			for _, c := range scenarioCiteRE.FindAllStringSubmatch(m[2], -1) {
				cited["S"+c[1]] = true
			}
		}
	}
	add(sectionMarkdown(text, "Validate"))
	add(sectionMarkdown(text, "Tasks"))
	if repoRoot != "" && slug != "" {
		files, _ := filepath.Glob(filepath.Join(changeDir(repoRoot, slug), "phases", "*.md"))
		for _, f := range files {
			if b, err := os.ReadFile(f); err == nil {
				add(string(b))
			}
		}
	}
	return cited
}

// checkScenarioCoverage fails when a manifest with a Scenarios section has a
// scenario that no checked task or Validate line cites. Manifests without the
// section are unaffected.
func checkScenarioCoverage(repoRoot, slug, text string) error {
	if !hasSection(text, "Scenarios") {
		return nil
	}
	scenarios, _ := parseScenarios(text)
	cited := checkedScenarioCitations(repoRoot, slug, text)
	missing := []string{}
	for _, s := range scenarios {
		if !cited[s.ID] {
			missing = append(missing, s.ID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("scenarios not covered by a checked task: %s", strings.Join(missing, ","))
	}
	return nil
}
