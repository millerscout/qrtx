package sdd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxGraphNeighborLines = 20
const maxIssueRules = 5
const maxKnownFixes = 3
const maxFixSummaryRunes = 120

// ProposeContextOpts configures propose preflight.
type ProposeContextOpts struct {
	RefreshGraph bool
	Project      string
}

// PackageOverlap is a shared package with another active change.
type PackageOverlap struct {
	OtherSlug string   `json:"other_slug"`
	Shared    []string `json:"shared_packages"`
}

// GraphNeighbor is one contract import edge touching the draft packages.
type GraphNeighbor struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// IssuesSummary is bounded open-issue counts for meta planning.
type IssuesSummary struct {
	Total  int            `json:"total_open"`
	ByRule map[string]int `json:"by_rule"`
}

// KnownFix is a recent known pass chain for context.
type KnownFix struct {
	Method  string `json:"method"`
	Summary string `json:"change_summary"`
	Test    string `json:"test_name,omitempty"`
}

// ProposeContextReport is the stable JSON/human preflight payload.
type ProposeContextReport struct {
	Slug           string           `json:"slug"`
	Score          string           `json:"score"`
	Domain         string           `json:"domain"`
	CodeHits       []CodeHit        `json:"code_hits,omitempty"`
	Packages       []string         `json:"packages"`
	Overlaps       []PackageOverlap `json:"overlaps"`
	GraphNeighbors []GraphNeighbor  `json:"graph_neighbors"`
	Issues         *IssuesSummary   `json:"issues,omitempty"`
	KnownFixes     []KnownFix       `json:"fixledger,omitempty"`
	Warnings       []string         `json:"warnings"`
	// ExistingBehaviour is what archived changes promised for the touched packages.
	ExistingBehaviour []ExistingBehaviour `json:"existing_behaviour,omitempty"`
}

// ProposeContext loads domain context for an SDD change slug (reporting only).
func ProposeContext(repoRoot, slug string, opts ProposeContextOpts) (ProposeContextReport, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ProposeContextReport{}, fmt.Errorf("propose-context: empty slug")
	}
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		var err error
		repoRoot, err = RepoRoot()
		if err != nil {
			return ProposeContextReport{}, err
		}
	}
	project := strings.TrimSpace(opts.Project)
	if project == "" {
		project = DefaultProject
	}
	if project == "" {
		project = filepath.Base(repoRoot)
	}

	out := ProposeContextReport{Slug: slug}

	man, err := ParseChangeFile(repoRoot, slug)
	if err != nil {
		if os.IsNotExist(err) {
			return out, fmt.Errorf("propose-context: change %q not found under sdd/changes/", slug)
		}
		return out, err
	}
	out.Packages = normalizePackageList(man.Packages)
	raw, err := os.ReadFile(changeFilePath(repoRoot, slug))
	if err != nil {
		return out, err
	}
	text := string(raw)
	req := sectionMarkdown(text, "User request")
	bullets := groundedAcceptanceBullets(req, sectionMarkdown(text, "Acceptance"))

	allPkgs, err := activeChangePackages(repoRoot)
	if err != nil {
		return out, err
	}
	out.Overlaps, out.Warnings = computeOverlaps(slug, out.Packages, allPkgs)
	if sib, err := SiblingRequestSlug(repoRoot, slug, req); err != nil {
		return out, err
	} else if sib != "" {
		found := false
		for _, o := range out.Overlaps {
			if o.OtherSlug == sib {
				found = true
				break
			}
		}
		if !found {
			out.Overlaps = append(out.Overlaps, PackageOverlap{OtherSlug: sib})
		}
		out.Warnings = append(out.Warnings, requestOverlapWarning(sib))
		sort.Slice(out.Overlaps, func(i, j int) bool { return out.Overlaps[i].OtherSlug < out.Overlaps[j].OtherSlug })
		sort.Strings(out.Warnings)
	}

	if opts.RefreshGraph {
		if _, err := WriteGraph(repoRoot); err != nil {
			return out, err
		}
	}
	g, err := loadGraph(repoRoot)
	if err != nil {
		g, err = BuildGraph(repoRoot)
		if err != nil {
			return out, err
		}
	}
	out.GraphNeighbors = graphNeighbors(g, out.Packages, maxGraphNeighborLines)

	if sum := issuesSummary(project); sum != nil {
		out.Issues = sum
	}
	if hints := knownFixes(project, maxKnownFixes); len(hints) > 0 {
		out.KnownFixes = hints
	}
	out.Score = proposeBand(req, bullets, len(out.Overlaps) > 0)
	out.CodeHits = collectCodeHits(out.Score, project, req)
	var hitPaths []string
	for _, h := range out.CodeHits {
		hitPaths = append(hitPaths, h.Path)
	}
	out.Domain = proposeDomain(req, hitPaths, sectionMarkdown(text, "Observed"))
	dirs := behaviourPackages(repoRoot, out.Packages, text)
	for _, p := range hitPaths {
		dirs = append(dirs, behaviourPackages(repoRoot, []string{pathDir(p)}, "")...)
	}
	out.ExistingBehaviour = existingBehaviour(repoRoot, uniqueSorted(dirs))
	return out, nil
}

func activeChangePackages(repoRoot string) (map[string][]string, error) {
	slugs, err := ListChanges(repoRoot)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(slugs))
	for _, s := range slugs {
		man, err := ParseChangeFile(repoRoot, s)
		if err != nil {
			continue
		}
		out[s] = normalizePackageList(man.Packages)
	}
	return out, nil
}

func normalizePackageList(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(filepath.ToSlash(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func computeOverlaps(slug string, draft []string, all map[string][]string) ([]PackageOverlap, []string) {
	if len(draft) == 0 {
		return nil, nil
	}
	draftSet := map[string]bool{}
	for _, p := range draft {
		draftSet[p] = true
	}
	var overlaps []PackageOverlap
	var warnings []string
	for other, pkgs := range all {
		if other == slug {
			continue
		}
		var shared []string
		for _, p := range pkgs {
			if draftSet[p] {
				shared = append(shared, p)
			}
		}
		if len(shared) == 0 {
			continue
		}
		sort.Strings(shared)
		overlaps = append(overlaps, PackageOverlap{OtherSlug: other, Shared: shared})
		warnings = append(warnings, fmt.Sprintf("package overlap with %q: %s", other, strings.Join(shared, ", ")))
	}
	sort.Slice(overlaps, func(i, j int) bool { return overlaps[i].OtherSlug < overlaps[j].OtherSlug })
	sort.Strings(warnings)
	return overlaps, warnings
}

func graphNeighbors(g DependencyGraph, draft []string, limit int) []GraphNeighbor {
	if limit <= 0 || len(draft) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, p := range draft {
		want[p] = true
	}
	var out []GraphNeighbor
	for _, e := range g.Edges {
		if !want[e.From] && !want[e.To] {
			continue
		}
		out = append(out, GraphNeighbor{From: e.From, To: e.To})
		if len(out) >= limit {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

func issuesSummary(project string) *IssuesSummary {
	if contextProvider == nil {
		return nil
	}
	byRule, total, ok := contextProvider.OpenIssues(project)
	if !ok || total == 0 {
		return nil
	}
	type pair struct {
		rule string
		n    int
	}
	var pairs []pair
	for r, n := range byRule {
		pairs = append(pairs, pair{r, n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n > pairs[j].n
		}
		return pairs[i].rule < pairs[j].rule
	})
	if len(pairs) > maxIssueRules {
		pairs = pairs[:maxIssueRules]
	}
	trimmed := map[string]int{}
	for _, p := range pairs {
		trimmed[p.rule] = p.n
	}
	return &IssuesSummary{Total: total, ByRule: trimmed}
}

func knownFixes(project string, limit int) []KnownFix {
	if contextProvider == nil {
		return nil
	}
	rows := contextProvider.KnownFixes(project, limit)
	if len(rows) == 0 {
		return nil
	}
	out := make([]KnownFix, 0, len(rows))
	for _, h := range rows {
		h.Method = strings.TrimSpace(h.Method)
		h.Summary = truncateRunes(strings.TrimSpace(h.Summary), maxFixSummaryRunes)
		h.Test = strings.TrimSpace(h.Test)
		if h.Method == "" && h.Summary == "" {
			continue
		}
		out = append(out, h)
	}
	return out
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// WriteProposeContextHuman prints a propose preflight report.
func WriteProposeContextHuman(w io.Writer, rep ProposeContextReport) error {
	_, _ = fmt.Fprintf(w, "slug: %s\n", rep.Slug)
	_, _ = fmt.Fprintf(w, "score: %s\n", rep.Score)
	_, _ = fmt.Fprintf(w, "domain: %s\n", rep.Domain)
	if len(rep.CodeHits) == 0 {
		_, _ = fmt.Fprintln(w, "code_hits: (none)")
	} else {
		_, _ = fmt.Fprintln(w, "code_hits:")
		for _, h := range rep.CodeHits {
			_, _ = fmt.Fprintf(w, "  %s:%d %s\n", h.Path, h.Line, h.Source)
		}
	}
	if len(rep.Packages) == 0 {
		_, _ = fmt.Fprintln(w, "packages: (none in manifest)")
	} else {
		_, _ = fmt.Fprintf(w, "packages: %s\n", strings.Join(rep.Packages, ", "))
	}
	_, _ = fmt.Fprintln(w, "\noverlap:")
	if len(rep.Overlaps) == 0 {
		_, _ = fmt.Fprintln(w, "  (none)")
	} else {
		for _, o := range rep.Overlaps {
			_, _ = fmt.Fprintf(w, "  %s shares %s\n", o.OtherSlug, strings.Join(o.Shared, ", "))
		}
	}
	_, _ = fmt.Fprintln(w, "\ngraph_neighbors:")
	if len(rep.GraphNeighbors) == 0 {
		_, _ = fmt.Fprintln(w, "  (none)")
	} else {
		for _, e := range rep.GraphNeighbors {
			_, _ = fmt.Fprintf(w, "  %s -> %s\n", e.From, e.To)
		}
	}
	_, _ = fmt.Fprintln(w, "\nissues:")
	if rep.Issues == nil {
		_, _ = fmt.Fprintln(w, "  (unavailable or none)")
	} else {
		_, _ = fmt.Fprintf(w, "  total_open: %d\n", rep.Issues.Total)
		keys := sortedKeys(rep.Issues.ByRule)
		for _, k := range keys {
			_, _ = fmt.Fprintf(w, "  %s: %d\n", k, rep.Issues.ByRule[k])
		}
	}
	_, _ = fmt.Fprintln(w, "\nfixledger:")
	if len(rep.KnownFixes) == 0 {
		_, _ = fmt.Fprintln(w, "  (unavailable or none)")
	} else {
		for _, h := range rep.KnownFixes {
			line := h.Summary
			if h.Method != "" {
				line = h.Method + ": " + line
			}
			if h.Test != "" {
				line += " (" + h.Test + ")"
			}
			_, _ = fmt.Fprintf(w, "  %s\n", line)
		}
	}
	if len(rep.ExistingBehaviour) > 0 {
		_, _ = fmt.Fprintln(w, "\nexisting_behaviour:")
		for _, e := range rep.ExistingBehaviour {
			_, _ = fmt.Fprintf(w, "  %s:\n", e.Package)
			for _, line := range strings.Split(e.Text, "\n") {
				_, _ = fmt.Fprintf(w, "    %s\n", line)
			}
		}
	}
	return nil
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// WriteProposeContextJSON encodes the report for agents.
func WriteProposeContextJSON(w io.Writer, rep ProposeContextReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
