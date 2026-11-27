package qrtx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Spec sections (Scenarios, Decisions, Risks) close the planning gap to OpenSpec
// measured in docs/SDD_VS_OPSX_BASELINE.md. They are checked only when the heading
// is present, so manifests written before the template gained them stay valid.

var (
	scenarioIDRE       = regexp.MustCompile(`^S(\d+)\b`)
	scenarioCiteRE     = regexp.MustCompile(`\bS(\d+)\b`)
	scenarioWhenRE     = regexp.MustCompile(`\bWHEN\b`)
	scenarioThenRE     = regexp.MustCompile(`\bTHEN\b`)
	decisionRejectRE   = regexp.MustCompile(`(?i)\brejected:`)
	rollbackLineRE     = regexp.MustCompile(`(?i)^(?:-\s+)?rollback:\s*(.*)$`)
	deployLineRE       = regexp.MustCompile(`(?i)^(?:-\s+)?deploy:\s*(.*)$`)
	negativeScenarioRE = regexp.MustCompile(`(?i)\b(error|fails?|failure|refus\w*|reject\w*|non-zero|exit [1-9]|unchanged|rollback|rolls? back|crash\w*|concurren\w*|invalid|missing|unavailable|timeout|times out|stale|empty|zero|denied|not a host|corrupt\w*|partial\w*|interrupt\w*)\b`)
	emptyScenarioRE    = regexp.MustCompile(`\bWHEN\s+(?:no|none|nothing|zero)\b`)
)

// scenario is one `- S<n> WHEN … THEN …` bullet.
type scenario struct {
	ID   string
	Text string
}

// SpecDepth counts spec content; it is reported next to the structural score and never gates.
type SpecDepth struct {
	Requirements       int  `json:"requirements"`
	Scenarios          int  `json:"scenarios"`
	NegativeScenarios  int  `json:"negative_scenarios"`
	UncoveredScenarios int  `json:"uncovered_scenarios"`
	Decisions          int  `json:"decisions"`
	Risks              int  `json:"risks"`
	Rollback           bool `json:"rollback"`
	Deploy             bool `json:"deploy"`
	HasSpecSections    bool `json:"has_spec_sections"`
	Reviewed           bool `json:"reviewed"`
	ReviewFindings     int  `json:"review_findings"`
	ReviewHints        int  `json:"review_hints"`
}

func hasSection(text, heading string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "## "+heading {
			return true
		}
	}
	return false
}

func sectionBullets(section string) []string {
	out := []string{}
	for _, line := range strings.Split(section, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- ") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(trim, "- ")))
		}
	}
	return out
}

// parseScenarios returns the S-id bullets of the Scenarios section and any malformed bullets.
func parseScenarios(text string) (scenarios []scenario, malformed []string) {
	for _, body := range sectionBullets(sectionMarkdown(text, "Scenarios")) {
		m := scenarioIDRE.FindStringSubmatch(body)
		if m == nil || !scenarioWhenRE.MatchString(body) || !scenarioThenRE.MatchString(body) {
			malformed = append(malformed, body)
			continue
		}
		scenarios = append(scenarios, scenario{ID: "S" + m[1], Text: body})
	}
	return scenarios, malformed
}

// scenarioCitations collects S-ids cited by Validate, Tasks and phase task files.
func scenarioCitations(repoRoot, slug, text string) map[string]bool {
	cited := map[string]bool{}
	add := func(s string) {
		for _, m := range scenarioCiteRE.FindAllStringSubmatch(s, -1) {
			cited["S"+m[1]] = true
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

func decisionBullets(text string) (withAlt int, missing []string, none bool) {
	bullets := sectionBullets(sectionMarkdown(text, "Decisions"))
	if len(bullets) == 1 && strings.HasPrefix(strings.ToLower(bullets[0]), "none") {
		return 0, nil, true
	}
	for _, b := range bullets {
		if decisionRejectRE.MatchString(b) {
			withAlt++
		} else {
			missing = append(missing, b)
		}
	}
	return withAlt, missing, false
}

// deployLine returns the text after `Deploy:` in Risks, or "".
func deployLine(text string) string {
	for _, line := range strings.Split(sectionMarkdown(text, "Risks"), "\n") {
		if m := deployLineRE.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func riskLines(text string) (risks int, rollback bool, rollbackFound bool) {
	for _, line := range strings.Split(sectionMarkdown(text, "Risks"), "\n") {
		trim := strings.TrimSpace(line)
		if m := rollbackLineRE.FindStringSubmatch(trim); m != nil {
			rollbackFound = true
			rest := strings.ToLower(strings.TrimSpace(m[1]))
			rollback = rest != "" && !strings.HasPrefix(rest, "n/a") && !strings.HasPrefix(rest, "none")
			continue
		}
		if deployLineRE.MatchString(trim) {
			continue
		}
		if strings.HasPrefix(trim, "- ") && !strings.HasPrefix(strings.ToLower(trim), "- none") {
			risks++
		}
	}
	return risks, rollback, rollbackFound
}

// validateSpecSections checks the spec sections that are present in a manifest.
func validateSpecSections(repoRoot, slug, text string) error {
	if err := validateRequirements(slug, text); err != nil {
		return err
	}
	if hasSection(text, "Scenarios") {
		section := sectionMarkdown(text, "Scenarios")
		if strings.Contains(section, "(TBD") {
			return fmt.Errorf("%s: Scenarios still contains TBD", slug)
		}
		scenarios, malformed := parseScenarios(text)
		if len(malformed) > 0 {
			return fmt.Errorf("%s: scenario must read `S<n> WHEN … THEN …`: %s", slug, truncateRunes(malformed[0], 100))
		}
		if len(scenarios) == 0 {
			return fmt.Errorf("%s: Scenarios has no `- S<n> WHEN … THEN …` bullet", slug)
		}
		seen := map[string]bool{}
		for _, sc := range scenarios {
			if seen[sc.ID] {
				return fmt.Errorf("%s: duplicate scenario id %s", slug, sc.ID)
			}
			seen[sc.ID] = true
		}
		cited := scenarioCitations(repoRoot, slug, text)
		var uncovered []string
		for _, sc := range scenarios {
			if !cited[sc.ID] {
				uncovered = append(uncovered, sc.ID)
			}
		}
		if len(uncovered) > 0 {
			sort.Strings(uncovered)
			return fmt.Errorf("%s: scenario %s is not cited by any Validate line or task (add `covers %s`)", slug, strings.Join(uncovered, ","), uncovered[0])
		}
	}
	if hasSection(text, "Decisions") {
		section := sectionMarkdown(text, "Decisions")
		if strings.Contains(section, "(TBD") {
			return fmt.Errorf("%s: Decisions still contains TBD", slug)
		}
		withAlt, missing, none := decisionBullets(text)
		if len(missing) > 0 {
			return fmt.Errorf("%s: decision needs `Rejected: <alternative> because <reason>`: %s", slug, truncateRunes(missing[0], 100))
		}
		if withAlt == 0 && !none {
			return fmt.Errorf("%s: Decisions has no bullet (write `- None: <reason>` when there is no choice)", slug)
		}
	}
	if hasSection(text, "Risks") {
		section := sectionMarkdown(text, "Risks")
		if strings.Contains(section, "(TBD") {
			return fmt.Errorf("%s: Risks still contains TBD", slug)
		}
		_, rollback, found := riskLines(text)
		if !found {
			return fmt.Errorf("%s: Risks needs a `Rollback:` line (`Rollback: n/a (<reason>)` when nothing persists)", slug)
		}
		// A change that needs a real rollback persists something, so the
		// order in which binaries, scripts and data move must be stated.
		if rollback && deployLine(text) == "" {
			return fmt.Errorf("%s: Risks needs a `Deploy:` line before `Rollback:` (the order binaries, scripts and data move in, and which old version keeps working meanwhile)", slug)
		}
	}
	return validateReviewSection(repoRoot, slug, text)
}

// computeSpecDepth reports spec-section counts for the score JSON.
func computeSpecDepth(repoRoot, slug, text string) SpecDepth {
	d := SpecDepth{HasSpecSections: hasSection(text, "Scenarios") || hasSection(text, "Decisions") || hasSection(text, "Risks")}
	scenarios, _ := parseScenarios(text)
	d.Scenarios = len(scenarios)
	reqs, _ := parseRequirements(text)
	d.Requirements = len(reqs)
	cited := scenarioCitations(repoRoot, slug, text)
	for _, sc := range scenarios {
		if negativeScenarioRE.MatchString(sc.Text) || emptyScenarioRE.MatchString(sc.Text) {
			d.NegativeScenarios++
		}
		if !cited[sc.ID] {
			d.UncoveredScenarios++
		}
	}
	d.Decisions, _, _ = decisionBullets(text)
	d.Risks, d.Rollback, _ = riskLines(text)
	dp := strings.ToLower(deployLine(text))
	d.Deploy = dp != "" && !strings.HasPrefix(dp, "n/a")
	if hasSection(text, "Review") && !strings.Contains(sectionMarkdown(text, "Review"), "(TBD") {
		d.Reviewed = true
		d.ReviewFindings = reviewFindings(text)
	}
	d.ReviewHints = len(allReviewHints(repoRoot, text, reviewClaims(text)))
	return d
}

// specSectionHeadings are validated only when present.
var specSectionHeadings = []string{"Requirements", "Scenarios", "Decisions", "Risks", "Review"}

// RemovePlaceholderSpecSections drops spec sections that still hold only the
// scaffold placeholder. Automated proposes (the IDE local-model path and
// WriteRoadmap) use it: they are not yet benchmarked on these sections, so an
// unfilled one is removed instead of failing finish-propose.
func RemovePlaceholderSpecSections(text string) string {
	for _, heading := range specSectionHeadings {
		text = removePlaceholderSection(text, heading)
	}
	return text
}

// removePlaceholderSection drops one section while it still holds only its TBD placeholder.
func removePlaceholderSection(text, heading string) string {
	if !hasSection(text, heading) || !strings.Contains(sectionMarkdown(text, heading), "(TBD") {
		return text
	}
	marker := "## " + heading
	start := -1
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.TrimSpace(line) == marker {
			start = offset
			break
		}
		offset += len(line)
	}
	if start < 0 {
		return text
	}
	rest := text[start+len(marker):]
	if next := nextKnownSection(rest); next >= 0 {
		return text[:start] + rest[next+1:]
	}
	return strings.TrimRight(text[:start], "\n") + "\n"
}
