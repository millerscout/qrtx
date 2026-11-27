package qrtx

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The review packet feeds a cold second reader. The 2026-11-22 bench
// (docs/SDD_VS_OPSX_BASELINE.md section 7) showed that SDD plans contradict
// themselves or the cited code more often than opsx plans, and that a
// prompt-only consistency pass did not fix that. The packet numbers every
// claim, attaches the cited code, and adds mechanical hints for the patterns
// graders found most often. ValidateReviewSection requires each hint to be
// answered in `## Review`.

// ReviewClaim is one numbered statement of the plan.
type ReviewClaim struct {
	ID      string `json:"id"`
	Section string `json:"section"`
	Text    string `json:"text"`
}

// ReviewCode is a cited file excerpt.
type ReviewCode struct {
	Path    string `json:"path"`
	From    int    `json:"from"`
	To      int    `json:"to"`
	Snippet string `json:"snippet"`
}

// ReviewHint is a likely contradiction found mechanically; it may be a false positive.
type ReviewHint struct {
	Kind   string   `json:"kind"`
	Claims []string `json:"claims"`
	Text   string   `json:"text"`
}

// ReviewPacket is what the second reader sees.
type ReviewPacket struct {
	Slug   string        `json:"slug"`
	Claims []ReviewClaim `json:"claims"`
	Code   []ReviewCode  `json:"code"`
	Hints  []ReviewHint  `json:"hints"`
	// Touches is other code using the same on-disk data or plan symbols.
	Touches []ReviewTouch `json:"touches"`
	// Callers holds each call site of a symbol a claim says "every caller" of.
	Callers []ReviewCode `json:"callers,omitempty"`
	// Constitution is the project's rules, which the plan's C lines cite.
	Constitution []ConstitutionDoc `json:"constitution,omitempty"`
}

var (
	reviewPathRE     = regexp.MustCompile("([A-Za-z0-9_.][A-Za-z0-9_./-]*\\.(?:go|py|ts|tsx|vue|js|sh|json|yaml|yml|toml))(?::(\\d+)(?:-(\\d+))?)?")
	decisionIDRE     = regexp.MustCompile(`^D(\d+)\b`)
	restoreRE        = regexp.MustCompile(`(?i)\b(restor\w*|backup|back up|snapshot|revert\w*|copy back|rename back|\.bak)\b`)
	noLossRE         = regexp.MustCompile(`(?i)(\bno\s+(?:\w+\s+){0,3}(?:write|save|update|data|session write)s?\s+(?:is\s+|are\s+)?(?:lost|dropped)|\bnever\s+(?:lose|loses|drop|drops)\b|\bloses?\s+no\b|\bwithout\s+losing\b|\bnot\s+lost\b|\bno\s+(?:lost|dropped)\s+(?:write|save|update)s?)`)
	presenceRE       = regexp.MustCompile(`(?i)(\balways\b|\bpresent\b|""|\[\]|\bempty string\b|\bnever\s+(?:null|omitted|missing)\b|\bnull\b|\bfixed keys\b|\bexact(?:ly)?\b)`)
	negatedTermRE    = regexp.MustCompile(`(?i)\b(?:no|never|without|not|cannot|can't|zero)\s+(?:\w+\s+){0,2}(starv\w*|overlap\w*|double[- ]book\w*|data loss|loss|downtime|drop\w*|duplicat\w*|race\w*|block\w*)`)
	acceptedTermRE   = regexp.MustCompile(`(?i)\b(?:accept\w*|tolerat\w*|allow\w*|may|can still|still)\b[^.;]{0,60}?\b(starv\w*|overlap\w*|double[- ]book\w*|data loss|loss|downtime|drop\w*|duplicat\w*|race\w*|block\w*)`)
	reviewBulletIDRE = regexp.MustCompile(`^R(\d+)\b`)
)

const (
	reviewMaxFiles   = 8
	reviewMaxSnippet = 40
	reviewContext    = 12
)

// BuildReviewPacket numbers the claims of a manifest, attaches cited code and hints.
func BuildReviewPacket(repoRoot, slug string) (ReviewPacket, error) {
	raw, err := os.ReadFile(changeFilePath(repoRoot, slug))
	if err != nil {
		return ReviewPacket{}, err
	}
	return buildReviewPacket(repoRoot, slug, string(raw), changePhaseText(repoRoot, string(raw))), nil
}

// buildReviewPacket builds the packet from the manifest text and the text of
// its phase files (tasks live there, and some hints read them).
func buildReviewPacket(repoRoot, slug, text, phases string) ReviewPacket {
	p := ReviewPacket{Slug: slug}
	p.Claims = reviewClaims(text)
	p.Code = reviewCode(repoRoot, text)
	touches, touchHints, callerCode := reviewTouches(repoRoot, text, p.Claims)
	p.Touches = touches
	p.Callers = callerCode
	p.Hints = append(reviewHints(repoRoot, text, p.Claims), touchHints...)
	p.Hints = append(p.Hints, refHints(loadSources(repoRoot), text, phases, p.Claims)...)
	// A broken constitution config is reported by finish-propose; the packet
	// is still useful without it.
	p.Constitution, _ = LoadConstitution(repoRoot)
	return p
}

var phaseTasksLineRE = regexp.MustCompile(`(?m)^tasks:\s*(\S+\.md)\s*$`)

// changePhaseText concatenates the phase files the manifest links, in order.
func changePhaseText(repoRoot, text string) string {
	var b strings.Builder
	for _, m := range phaseTasksLineRE.FindAllStringSubmatch(text, -1) {
		raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(m[1])))
		if err != nil {
			continue
		}
		b.WriteString("## FILE: " + m[1] + "\n")
		b.Write(raw)
		b.WriteString("\n")
	}
	return b.String()
}

func reviewClaims(text string) []ReviewClaim {
	var out []ReviewClaim
	n := 0
	for _, b := range sectionBullets(sectionMarkdown(text, "Acceptance")) {
		n++
		out = append(out, ReviewClaim{ID: "A" + strconv.Itoa(n), Section: "Acceptance", Text: b})
	}
	reqs, _ := parseRequirements(text)
	for _, r := range reqs {
		out = append(out, ReviewClaim{ID: r.ID, Section: "Requirements", Text: r.Text})
	}
	scenarios, _ := parseScenarios(text)
	for _, sc := range scenarios {
		out = append(out, ReviewClaim{ID: sc.ID, Section: "Scenarios", Text: sc.Text})
	}
	n = 0
	for _, b := range sectionBullets(sectionMarkdown(text, "Decisions")) {
		n++
		id := "D" + strconv.Itoa(n)
		if m := decisionIDRE.FindStringSubmatch(b); m != nil {
			id = "D" + m[1]
		}
		out = append(out, ReviewClaim{ID: id, Section: "Decisions", Text: b})
	}
	for _, b := range sectionBullets(sectionMarkdown(text, "Constitution")) {
		if m := constitutionBulletIDRE.FindStringSubmatch(b); m != nil {
			out = append(out, ReviewClaim{ID: "C" + m[1], Section: "Constitution", Text: b})
		}
	}
	n = 0
	for _, b := range claimLines(sectionMarkdown(text, "Assumptions")) {
		n++
		out = append(out, ReviewClaim{ID: "M" + strconv.Itoa(n), Section: "Assumptions", Text: b})
	}
	n = 0
	for _, line := range strings.Split(sectionMarkdown(text, "Risks"), "\n") {
		trim := strings.TrimSpace(line)
		if m := rollbackLineRE.FindStringSubmatch(trim); m != nil {
			out = append(out, ReviewClaim{ID: "RB", Section: "Risks", Text: strings.TrimSpace(m[1])})
			continue
		}
		if m := deployLineRE.FindStringSubmatch(trim); m != nil {
			out = append(out, ReviewClaim{ID: "DP", Section: "Risks", Text: strings.TrimSpace(m[1])})
			continue
		}
		if strings.HasPrefix(trim, "- ") {
			n++
			out = append(out, ReviewClaim{ID: "K" + strconv.Itoa(n), Section: "Risks", Text: strings.TrimSpace(strings.TrimPrefix(trim, "- "))})
		}
	}
	return out
}

// claimLines returns bullets, or non-empty paragraphs when a section has no bullets.
func claimLines(section string) []string {
	if b := sectionBullets(section); len(b) > 0 {
		return b
	}
	var out []string
	for _, line := range strings.Split(section, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "{:") || strings.Contains(trim, "(TBD") {
			continue
		}
		out = append(out, trim)
	}
	return out
}

type citedPath struct {
	path     string
	from, to int
}

func citedPaths(repoRoot, text string) []citedPath {
	var out []citedPath
	seen := map[string]bool{}
	for _, heading := range []string{"Observed", "Assumptions", "Decisions", "Risks", "Scenarios", "Acceptance"} {
		for _, m := range reviewPathRE.FindAllStringSubmatch(sectionMarkdown(text, heading), -1) {
			path := strings.TrimPrefix(m[1], "./")
			if strings.Contains(path, "..") {
				continue
			}
			if st, err := os.Stat(filepath.Join(repoRoot, path)); err != nil || st.IsDir() {
				continue
			}
			from, _ := strconv.Atoi(m[2])
			to, _ := strconv.Atoi(m[3])
			key := path + ":" + m[2] + "-" + m[3]
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, citedPath{path: path, from: from, to: to})
		}
	}
	return out
}

func reviewCode(repoRoot, text string) []ReviewCode {
	var out []ReviewCode
	files := map[string]bool{}
	for _, c := range citedPaths(repoRoot, text) {
		if !files[c.path] && len(files) >= reviewMaxFiles {
			continue
		}
		files[c.path] = true
		b, err := os.ReadFile(filepath.Join(repoRoot, c.path))
		if err != nil {
			continue
		}
		lines := strings.Split(string(b), "\n")
		from, to := 1, len(lines)
		switch {
		case c.from > 0 && c.to >= c.from:
			from, to = c.from-reviewContext/2, c.to+reviewContext/2
		case c.from > 0:
			from, to = c.from-reviewContext, c.from+reviewContext
		default:
			// Whole-file cite: show the json-tagged structs of Go files, else the head.
			if strings.HasSuffix(c.path, ".go") {
				if code, ok := goTaggedStructs(repoRoot, c.path, lines); ok {
					out = append(out, code)
					continue
				}
			}
			to = reviewMaxSnippet
		}
		if from < 1 {
			from = 1
		}
		if to > len(lines) {
			to = len(lines)
		}
		if to-from+1 > reviewMaxSnippet {
			to = from + reviewMaxSnippet - 1
		}
		out = append(out, ReviewCode{Path: c.path, From: from, To: to, Snippet: strings.Join(lines[from-1:to], "\n")})
	}
	return out
}

func goTaggedStructs(repoRoot, path string, lines []string) (ReviewCode, bool) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(repoRoot, path), nil, parser.SkipObjectResolution)
	if err != nil {
		return ReviewCode{}, false
	}
	var parts []string
	first, last := 0, 0
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || !hasJSONTag(st) {
			return true
		}
		a, b := fset.Position(ts.Pos()).Line, fset.Position(st.End()).Line
		if first == 0 {
			first = a
		}
		last = b
		parts = append(parts, strings.Join(lines[a-1:b], "\n"))
		return true
	})
	if len(parts) == 0 {
		return ReviewCode{}, false
	}
	snippet := strings.Join(parts, "\n\n")
	if ls := strings.Split(snippet, "\n"); len(ls) > reviewMaxSnippet {
		snippet = strings.Join(ls[:reviewMaxSnippet], "\n")
	}
	return ReviewCode{Path: path, From: first, To: last, Snippet: snippet}, true
}

func hasJSONTag(st *ast.StructType) bool {
	for _, f := range st.Fields.List {
		if f.Tag != nil && strings.Contains(f.Tag.Value, "json:") {
			return true
		}
	}
	return false
}

type omitField struct {
	name, path string
	line       int
}

func omitemptyFields(repoRoot string, paths []citedPath) []omitField {
	var out []omitField
	done := map[string]bool{}
	for _, c := range paths {
		if done[c.path] || !strings.HasSuffix(c.path, ".go") {
			continue
		}
		done[c.path] = true
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(repoRoot, c.path), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			field, ok := n.(*ast.Field)
			if !ok || field.Tag == nil {
				return true
			}
			tag, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				return true
			}
			js := reflect.StructTag(tag).Get("json")
			parts := strings.Split(js, ",")
			if len(parts) < 2 || parts[0] == "" || parts[0] == "-" {
				return true
			}
			for _, opt := range parts[1:] {
				if opt == "omitempty" {
					out = append(out, omitField{name: parts[0], path: c.path, line: fset.Position(field.Pos()).Line})
				}
			}
			return true
		})
	}
	return out
}

func reviewHints(repoRoot, text string, claims []ReviewClaim) []ReviewHint {
	var hints []ReviewHint
	byID := map[string]ReviewClaim{}
	for _, c := range claims {
		byID[c.ID] = c
	}
	// 1. Rollback restores earlier state while a claim promises no write is lost.
	if rb, ok := byID["RB"]; ok && restoreRE.MatchString(rb.Text) {
		for _, c := range claims {
			if c.ID != "RB" && noLossRE.MatchString(c.Text) {
				hints = append(hints, ReviewHint{
					Kind:   "rollback-vs-no-loss",
					Claims: []string{"RB", c.ID},
					Text:   "RB restores earlier state, while " + c.ID + " promises no write is lost: what happens to writes made after the backup or snapshot?",
				})
			}
		}
	}
	// 2. A field tagged omitempty in cited code that a check expects present.
	for _, f := range omitemptyFields(repoRoot, citedPaths(repoRoot, text)) {
		quoted := regexp.MustCompile(`(?:^|[^A-Za-z0-9_])` + regexp.QuoteMeta(f.name) + `(?:[^A-Za-z0-9_]|$)`)
		var ids []string
		for _, c := range claims {
			if (c.Section == "Acceptance" || c.Section == "Scenarios") && quoted.MatchString(c.Text) && presenceRE.MatchString(c.Text) {
				ids = append(ids, c.ID)
			}
		}
		if len(ids) > 0 {
			hints = append(hints, ReviewHint{
				Kind:   "omitempty-vs-presence",
				Claims: ids,
				Text:   fmt.Sprintf("%s expect `%s` in the output, but %s:%d tags it omitempty, so a zero value is dropped when that struct is marshalled directly", strings.Join(ids, ","), f.name, f.path, f.line),
			})
		}
	}
	// 3. A risk handled only by a warning or a deferral, while a check promises no loss.
	var promises []string
	for _, c := range claims {
		if (c.Section == "Acceptance" || c.Section == "Requirements" || c.Section == "Scenarios") && noLossRE.MatchString(c.Text) {
			promises = append(promises, c.ID)
		}
	}
	if len(promises) > 0 {
		for _, c := range claims {
			if c.Section != "Risks" && c.Section != "Decisions" {
				continue
			}
			mitigation := c.Text
			if i := strings.Index(c.Text, "→"); i >= 0 {
				mitigation = c.Text[i:]
			} else if c.Section == "Risks" {
				continue
			}
			if !softMitigationRE.MatchString(mitigation) || hardMitigationRE.MatchString(mitigation) || !lossTermRE.MatchString(c.Text) {
				continue
			}
			hints = append(hints, ReviewHint{
				Kind:   "soft-mitigation-vs-promise",
				Claims: append([]string{c.ID}, promises...),
				Text:   fmt.Sprintf("%s handles a loss case only by warning, detecting or deferring it, while %s promise no loss: is the case really prevented, or must the promise be narrowed and the case named in Non-goals?", c.ID, listIDs(promises)),
			})
		}
	}
	// 4. One claim rules something out while another accepts it.
	negated := map[string][]string{}
	accepted := map[string][]string{}
	for _, c := range claims {
		for _, m := range negatedTermRE.FindAllStringSubmatch(c.Text, -1) {
			negated[termStem(m[1])] = appendUnique(negated[termStem(m[1])], c.ID)
		}
		if c.Section == "Decisions" || c.Section == "Risks" || c.Section == "Assumptions" {
			for _, m := range acceptedTermRE.FindAllStringSubmatch(c.Text, -1) {
				accepted[termStem(m[1])] = appendUnique(accepted[termStem(m[1])], c.ID)
			}
		}
	}
	terms := make([]string, 0, len(negated))
	for t := range negated {
		terms = append(terms, t)
	}
	sort.Strings(terms)
	for _, t := range terms {
		for _, a := range accepted[t] {
			for _, n := range negated[t] {
				if a == n {
					continue
				}
				hints = append(hints, ReviewHint{
					Kind:   "ruled-out-vs-accepted",
					Claims: []string{n, a},
					Text:   fmt.Sprintf("%s rules out %s, while %s accepts or allows it", n, t, a),
				})
			}
		}
	}
	return hints
}

var (
	softMitigationRE = regexp.MustCompile(`(?i)\b(warn\w*|log\w*|detect\w*|alert\w*|report\w*|flag\w*|deferred|later|follow[- ]?up|out of scope|documented)\b`)
	hardMitigationRE = regexp.MustCompile(`(?i)\b(lock\w*|retr(?:y|ies)|reject\w*|refus\w*|block\w*|reconcil\w*|re-?import\w*|merg\w*|replay\w*|serializ\w*|atomic\w*)\b`)
	lossTermRE       = regexp.MustCompile(`(?i)\b(lost|lose\w*|loss|overwrit\w*|clobber\w*|stale|skew|missed|dropped|silently)\b`)
	deferFixRE       = regexp.MustCompile(`(?i)\b(defer\w*|later|follow[- ]?up|out of scope|warn(?:s|ing)? only|only warn\w*|next change|separate change|not handled)\b`)
	claimIDRE        = regexp.MustCompile(`\b([AS]\d+)\b`)
	wordRE           = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.]{4,}`)
	commonWords      = map[string]bool{"after": true, "before": true, "while": true, "which": true, "their": true, "there": true, "would": true, "should": true, "could": true, "every": true, "still": true, "write": true, "writes": true, "change": true, "because": true, "conflict": true}
)

// deferredPromise rejects a review fix that defers a case an Acceptance bullet
// or a no-loss scenario still promises, unless Non-goals or Deferred names it.
// The bench found plans that "fixed" a conflict this way and kept the promise.
func deferredPromise(slug, text, bullet string) error {
	arrow := strings.Index(bullet, "→")
	if arrow < 0 {
		arrow = strings.Index(bullet, "->")
	}
	if arrow < 0 || !deferFixRE.MatchString(bullet[arrow:]) {
		return nil
	}
	conflict := bullet[:arrow]
	promised := ""
	claims := reviewClaims(text)
	for _, m := range claimIDRE.FindAllStringSubmatch(conflict, -1) {
		for _, c := range claims {
			if c.ID != m[1] {
				continue
			}
			if c.Section == "Acceptance" || ((c.Section == "Scenarios" || c.Section == "Requirements") && noLossRE.MatchString(c.Text)) {
				promised = c.ID
			}
		}
	}
	if promised == "" {
		return nil
	}
	scope := strings.ToLower(sectionMarkdown(text, "Non-goals") + "\n" + sectionMarkdown(text, "Deferred"))
	shared := 0
	for _, w := range wordRE.FindAllString(strings.ToLower(conflict), -1) {
		if !commonWords[w] && !claimIDRE.MatchString(strings.ToUpper(w)) && strings.Contains(scope, w) {
			shared++
		}
	}
	if shared >= 2 {
		return nil
	}
	return fmt.Errorf("%s: review fix defers a case %s still promises; handle it (Decision, Scenario or task), or change %s and name the case in Non-goals or Deferred: %s", slug, promised, promised, truncateRunes(bullet, 120))
}

// allReviewHints is every hint the packet shows, including uncited writers and callers.
func allReviewHints(repoRoot, text string, claims []ReviewClaim) []ReviewHint {
	_, touchHints, _ := reviewTouches(repoRoot, text, claims)
	hints := append(reviewHints(repoRoot, text, claims), touchHints...)
	return append(hints, refHints(loadSources(repoRoot), text, changePhaseText(repoRoot, text), claims)...)
}

func termStem(s string) string {
	s = strings.ToLower(s)
	for _, stem := range []string{"starv", "overlap", "double", "data loss", "loss", "downtime", "drop", "duplicat", "race", "block"} {
		if strings.HasPrefix(s, stem) {
			return stem
		}
	}
	return s
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// FormatReviewPacket renders the packet as the reviewer prompt.
func FormatReviewPacket(p ReviewPacket) string {
	var b strings.Builder
	b.WriteString("# Review packet: " + p.Slug + "\n\n")
	b.WriteString("You are a second reader with no stake in this plan. Find statements that cannot all be true together, and claims that the cited code contradicts. ")
	b.WriteString("Read the cited files yourself when an excerpt is not enough. Report only real conflicts: a reworded duplicate or a missing nice-to-have is not one.\n\n")
	b.WriteString("Output one line per finding, then stop:\n")
	b.WriteString("`- R<n> <claim ids or path:line> <the conflict> → <fix: new or changed D/A/S id, or needs user: question>`\n")
	b.WriteString("If there is no conflict, output `- None: <what you checked>`. Every hint below needs its own R line, even when you conclude it is not a conflict (then the fix is `not a conflict because …`).\n\n")
	b.WriteString("## Claims\n\n")
	for _, c := range p.Claims {
		b.WriteString("- " + c.ID + " (" + c.Section + ") " + c.Text + "\n")
	}
	if len(p.Constitution) > 0 {
		b.WriteString("\n## Constitution\n\n")
		b.WriteString("The project's rules. Check each C claim against its rule, and report a rule the plan breaks, or a rule the change touches that no C claim covers, as a finding.\n\n")
		for _, d := range p.Constitution {
			b.WriteString("- " + d.Path + "\n")
			for _, r := range d.Rules {
				b.WriteString("  - " + r + "\n")
			}
		}
	}
	if len(p.Hints) > 0 {
		b.WriteString("\n## Hints (mechanical, may be false positives)\n\n")
		for _, h := range p.Hints {
			b.WriteString("- [" + h.Kind + "] " + h.Text + "\n")
		}
	}
	if len(p.Touches) > 0 {
		b.WriteString("\n## Other code using the same data or symbols\n\n")
		b.WriteString("Each line is a file that reads or writes the same on-disk data, or calls a symbol the plan's design depends on. Uncited ones are code the plan never looked at: open them before deciding a hint is not a conflict.\n\n")
		for _, t := range p.Touches {
			mark := ""
			if !t.Cited {
				mark = " (uncited)"
			}
			fmt.Fprintf(&b, "- [%s] %s:%d%s `%s`\n", t.Key, t.Path, t.Line, mark, t.Text)
		}
	}
	if len(p.Callers) > 0 {
		b.WriteString("\n## Every call site of symbols a claim covers for all callers\n")
		for _, c := range p.Callers {
			fmt.Fprintf(&b, "\n### %s:%d-%d\n\n```\n%s\n```\n", c.Path, c.From, c.To, c.Snippet)
		}
	}
	if len(p.Code) > 0 {
		b.WriteString("\n## Cited code\n")
		for _, c := range p.Code {
			fmt.Fprintf(&b, "\n### %s:%d-%d\n\n```\n%s\n```\n", c.Path, c.From, c.To, c.Snippet)
		}
	}
	return b.String()
}

// reviewFindings counts R bullets in `## Review`.
func reviewFindings(text string) int {
	n := 0
	for _, b := range sectionBullets(sectionMarkdown(text, "Review")) {
		if reviewBulletIDRE.MatchString(b) {
			n++
		}
	}
	return n
}

// validateReviewSection checks `## Review` when present: R bullets carry a fix,
// and every mechanical hint is answered by a bullet naming one of its claims.
func validateReviewSection(repoRoot, slug, text string) error {
	if !hasSection(text, "Review") {
		return nil
	}
	section := sectionMarkdown(text, "Review")
	if strings.Contains(section, "(TBD") {
		return fmt.Errorf("%s: Review still contains TBD (run `%s change review-packet --change %s` and record the second reader's findings)", slug, CLIName, slug)
	}
	bullets := sectionBullets(section)
	if len(bullets) == 0 {
		return fmt.Errorf("%s: Review has no bullet (`- R1 … → …` or `- None: <what was checked>`)", slug)
	}
	var reviewed []string
	for _, b := range bullets {
		lower := strings.ToLower(b)
		if strings.HasPrefix(lower, "none") {
			reviewed = append(reviewed, b)
			continue
		}
		if !reviewBulletIDRE.MatchString(b) {
			return fmt.Errorf("%s: review finding must start with R<n>: %s", slug, truncateRunes(b, 100))
		}
		if !strings.Contains(b, "→") && !strings.Contains(b, "->") {
			return fmt.Errorf("%s: review finding needs `→ <fix>`: %s", slug, truncateRunes(b, 100))
		}
		if err := deferredPromise(slug, text, b); err != nil {
			return err
		}
		reviewed = append(reviewed, b)
	}
	hints := allReviewHints(repoRoot, text, reviewClaims(text))
	for _, h := range hints {
		answered := false
		for _, b := range reviewed {
			if !reviewBulletIDRE.MatchString(b) {
				continue
			}
			for _, id := range h.Claims {
				if regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + `\b`).MatchString(b) {
					answered = true
				}
			}
		}
		if !answered {
			return fmt.Errorf("%s: review hint not answered (add an R line naming %s): %s", slug, strings.Join(h.Claims, " or "), h.Text)
		}
	}
	return nil
}
