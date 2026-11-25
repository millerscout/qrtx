package sdd

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Requirements group scenarios by concern, as opsx's `### Requirement` +
// `#### Scenario` does. The edge-case gap analysis (docs/SDD_VS_OPSX_BASELINE.md
// section 7.10) traced opsx's lifecycle coverage to one requirement per concern,
// each with its own scenarios. The section is optional and checked only when
// its heading is present.

var (
	requirementIDRE   = regexp.MustCompile(`^Q(\d+)\b`)
	requirementNormRE = regexp.MustCompile(`\b(SHALL|MUST)\b`)
	scenarioReqRE     = regexp.MustCompile(`^S\d+\s*\(\s*(Q\d+(?:\s*,\s*Q\d+)*)\s*\)`)
	reqIDListRE       = regexp.MustCompile(`Q\d+`)
)

// requirement is one `- Q<n> <subject> SHALL <behaviour>.` bullet.
type requirement struct {
	ID   string
	Text string
}

// parseRequirements returns the Q-id bullets of the Requirements section and any malformed bullets.
func parseRequirements(text string) (reqs []requirement, malformed []string) {
	for _, b := range sectionBullets(sectionMarkdown(text, "Requirements")) {
		m := requirementIDRE.FindStringSubmatch(b)
		if m == nil || !requirementNormRE.MatchString(b) {
			malformed = append(malformed, b)
			continue
		}
		reqs = append(reqs, requirement{ID: "Q" + m[1], Text: b})
	}
	return reqs, malformed
}

// scenarioRequirements maps each scenario id to the requirement ids it names in `S3 (Q2) WHEN …`.
func scenarioRequirements(text string) map[string][]string {
	out := map[string][]string{}
	scenarios, _ := parseScenarios(text)
	for _, sc := range scenarios {
		bullet := sc.ID + strings.TrimPrefix(sc.Text, sc.ID)
		if m := scenarioReqRE.FindStringSubmatch(bullet); m != nil {
			out[sc.ID] = reqIDListRE.FindAllString(m[1], -1)
			continue
		}
		out[sc.ID] = nil
	}
	return out
}

// validateRequirements checks the Requirements section when present: Q ids
// with SHALL/MUST, every scenario names an existing requirement, and every
// requirement has at least one scenario.
func validateRequirements(slug, text string) error {
	if !hasSection(text, "Requirements") {
		return nil
	}
	if strings.Contains(sectionMarkdown(text, "Requirements"), "(TBD") {
		return fmt.Errorf("%s: Requirements still contains TBD", slug)
	}
	reqs, malformed := parseRequirements(text)
	if len(malformed) > 0 {
		return fmt.Errorf("%s: requirement must read `Q<n> <subject> SHALL <behaviour>`: %s", slug, truncateRunes(malformed[0], 100))
	}
	if len(reqs) == 0 {
		return fmt.Errorf("%s: Requirements has no `- Q<n> … SHALL …` bullet", slug)
	}
	known := map[string]bool{}
	for _, r := range reqs {
		if known[r.ID] {
			return fmt.Errorf("%s: duplicate requirement id %s", slug, r.ID)
		}
		known[r.ID] = true
	}
	covered := map[string]bool{}
	byScenario := scenarioRequirements(text)
	ids := make([]string, 0, len(byScenario))
	for id := range byScenario {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, sid := range ids {
		qs := byScenario[sid]
		if len(qs) == 0 {
			return fmt.Errorf("%s: scenario %s names no requirement (write `%s (Q1) WHEN … THEN …`)", slug, sid, sid)
		}
		for _, q := range qs {
			if !known[q] {
				return fmt.Errorf("%s: scenario %s names unknown requirement %s", slug, sid, q)
			}
			covered[q] = true
		}
	}
	var bare []string
	for _, r := range reqs {
		if !covered[r.ID] {
			bare = append(bare, r.ID)
		}
	}
	if len(bare) > 0 {
		return fmt.Errorf("%s: requirement %s has no scenario (add `- S<n> (%s) WHEN … THEN …`)", slug, strings.Join(bare, ","), bare[0])
	}
	return nil
}
