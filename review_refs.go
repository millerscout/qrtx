package sdd

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The rbac-route-gates plan (TypeScript + SQL, 2026-11-23) passed verify with
// no hints, yet it used a permission code in phase 1 that phase 4 creates,
// gated routes on a wildcard (`custom_field.manage.*`) that no exact check can
// match, added `records.read` next to the existing `records.view.any/own`, and
// replaced a direct RPC only under src/app/api/admin while the real callers were
// elsewhere. These checks work on identifiers and string literals, so they do
// not depend on the language.

var (
	// A dotted lowercase identifier: permission codes, event names, config keys.
	codeTokenRE = regexp.MustCompile("`([a-z][a-z0-9_]*(?:\\.(?:[a-z0-9_]+|\\*))+)`")
	// Dotted identifiers anywhere in source (code or strings), for existence.
	srcDottedRE = regexp.MustCompile(`[a-z_][a-z0-9_]*(?:\.[a-z0-9_]+)+`)
	// Quoted literals in source.
	srcLiteralRE = regexp.MustCompile("['\"`]([A-Za-z_][A-Za-z0-9_.:-]{2,})['\"`]")
	createWordRE = regexp.MustCompile(`(?i)\b(?:migrations?|insert\w*|adds?|added|creates?|created|introduc\w*|defines?|seeds?|new)\b`)
	// Lowercase after the first letter, so HTTP methods (DELETE) do not count.
	replaceWordRE = regexp.MustCompile(`\b(?:[Rr]eplac(?:e|es|ed|ing)|[Rr]emov(?:e|es|ed|ing)|[Mm]ov(?:e|es|ed|ing)\s+(?:\w+\s+){0,3}(?:off|to|onto)|[Mm]igrat(?:e|es|ed|ing)|[Dd]rop(?:s|ped|ping)?|[Dd]elet(?:e|es|ed|ing)|[Ss]top\s+call\w*|[Ss]wap(?:s|ped|ping)?|[Dd]eprecat(?:e|es|ed|ing))\b`)
	// A token used as the one value something is gated on or mapped to.
	valueUseRE = regexp.MustCompile(`(?i)(?:→|->|=)\s*$|\b(?:gates?|gated|checks?|requires?|authori[sz]e[sd]?)\s+(?:\w+\s+){0,2}(?:on|with)?\s*$`)
	// Words after the verb that start the destination ("move X to Y", "replaced by Y").
	destWordRE    = regexp.MustCompile(`(?i)\b(?:to|onto|into|with|by|for|from|when|if|unless|because|but|never|then|while|after|before)\b|→|->`)
	pronounRE     = regexp.MustCompile(`(?i)\b(?:each|them|it|these|those|all)\b`)
	passiveVerbRE = regexp.MustCompile(`^(?:replaced|removed|migrated|dropped|deleted|swapped|deprecated)$`)
	taskIDRE      = regexp.MustCompile(`^\s*-\s*\[[ xX]\]\s*([0-9]+(?:\.[0-9]+)+|V[0-9.]+)\b`)
	phaseFileRE   = regexp.MustCompile(`(?m)^## FILE: (.+)$`)
	grepScopeRE   = regexp.MustCompile(`\b(?:grep|rg)\b[^\n]*?\s((?:\./)?[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.\[\]-]+)+/?)`)
	backtickSpan  = regexp.MustCompile("`([^`\\n]+)`")
	quotedInSpan  = regexp.MustCompile(`['"]([A-Za-z_][A-Za-z0-9_.]{4,})['"]`)
	plainSymbolRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*$`)
)

// fileExtSegment is a last segment that makes a dotted token a file name.
var fileExtSegment = map[string]bool{"ts": true, "tsx": true, "js": true, "jsx": true, "mjs": true, "go": true, "py": true, "sql": true, "md": true, "json": true, "yaml": true, "yml": true, "sh": true, "toml": true, "txt": true, "html": true, "css": true, "rs": true, "swift": true, "lock": true, "env": true, "log": true, "csv": true, "mod": true, "sum": true}

const (
	refsMaxList         = 8
	refsMaxLiteralSites = 10
)

type refIndex struct {
	dotted   map[string]bool     // dotted identifiers anywhere in non-test source
	literals map[string][]string // quoted literal → files (non-test)
	srcs     []sourceFile
}

func buildRefIndex(srcs []sourceFile) refIndex {
	ix := refIndex{dotted: map[string]bool{}, literals: map[string][]string{}, srcs: srcs}
	for _, s := range srcs {
		if s.test {
			continue
		}
		for _, l := range s.lines {
			for _, m := range srcDottedRE.FindAllString(l, -1) {
				ix.dotted[m] = true
			}
			for _, m := range srcLiteralRE.FindAllStringSubmatch(l, -1) {
				ix.literals[m[1]] = appendUnique(ix.literals[m[1]], s.path)
			}
		}
	}
	return ix
}

// family is the first segment of a dotted token, with its dot.
func family(tok string) string {
	if i := strings.Index(tok, "."); i > 0 {
		return tok[:i+1]
	}
	return ""
}

// siblings lists existing quoted literals of tok's family, other than tok.
func (ix refIndex) siblings(tok string) []string {
	fam := family(tok)
	if fam == "" {
		return nil
	}
	var out []string
	for lit := range ix.literals {
		if lit != tok && strings.HasPrefix(lit, fam) && !strings.ContainsAny(lit, ":-") && !fileExtSegment[lit[strings.LastIndex(lit, ".")+1:]] {
			out = append(out, lit)
		}
	}
	sort.Strings(out)
	return out
}

// globMatches lists existing literals a wildcard token stands for.
func (ix refIndex) globMatches(tok string) []string {
	re := regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(tok), `\*`, `[a-z0-9_]+`) + "$")
	var out []string
	for lit := range ix.literals {
		if re.MatchString(lit) {
			out = append(out, lit)
		}
	}
	sort.Strings(out)
	return out
}

type planPhase struct {
	name  string
	lines []string
}

// splitPhaseFiles splits concatenated phase text ("## FILE: <path>" headers).
func splitPhaseFiles(phases string) []planPhase {
	var out []planPhase
	idx := phaseFileRE.FindAllStringSubmatchIndex(phases, -1)
	for i, m := range idx {
		end := len(phases)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		name := phases[m[2]:m[3]]
		if strings.HasSuffix(name, "/change.sdd.md") {
			continue
		}
		out = append(out, planPhase{name: name, lines: strings.Split(phases[m[1]:end], "\n")})
	}
	return out
}

// lineID is the task id a phase line starts with ("1.2"), if any.
func lineID(line string) string {
	if m := taskIDRE.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

type tokenUse struct {
	claims  []string // claim ids mentioning the token
	tasks   []string // task ids mentioning it, in plan order
	first   int      // first phase using it (-1: none)
	create  int      // first phase creating it (-1: none)
	firstT  string   // task id of the first use
	createT string   // task id of the creation
	decl    bool     // a Decision declares it new
}

func (u tokenUse) ids() []string {
	if len(u.claims) > 0 {
		return u.claims
	}
	return u.tasks
}

func tokenUses(tok string, claims []ReviewClaim, phases []planPhase) tokenUse {
	u := tokenUse{first: -1, create: -1}
	bt := "`" + tok + "`"
	for _, c := range claims {
		txt := stripRejected(c.Text)
		if strings.Contains(txt, bt) {
			u.claims = appendUnique(u.claims, c.ID)
			if c.Section == "Decisions" && createWordRE.MatchString(txt) {
				u.decl = true
			}
		}
	}
	for i, p := range phases {
		for _, l := range p.lines {
			l = stripRejected(l)
			if !strings.Contains(l, bt) {
				continue
			}
			id := lineID(l)
			if id != "" {
				u.tasks = appendUnique(u.tasks, id)
			}
			if u.first < 0 {
				u.first, u.firstT = i, id
			}
			// Creating means the task adds the token, not just names it next to a migration.
			if u.create < 0 && createWordRE.MatchString(l) && strings.Index(l, bt) > 0 && createWordRE.MatchString(l[:strings.Index(l, bt)]) {
				u.create, u.createT = i, id
			}
		}
	}
	return u
}

func phaseLabel(p planPhase, id string) string {
	name := p.name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if id != "" {
		return "task " + id + " (" + name + ")"
	}
	return name
}

// refHints runs the four identifier checks over the plan and its phases.
func refHints(srcs []sourceFile, text, phases string, claims []ReviewClaim) []ReviewHint {
	ph := splitPhaseFiles(phases)
	all := text + "\n" + phases
	var toks []string
	seen := map[string]bool{}
	for _, m := range codeTokenRE.FindAllStringSubmatch(all, -1) {
		t := m[1]
		last := t[strings.LastIndex(t, ".")+1:]
		if seen[t] || fileExtSegment[last] || allDigits(last) {
			continue
		}
		seen[t] = true
		toks = append(toks, t)
	}
	if len(toks) == 0 && !replaceWordRE.MatchString(all) {
		return nil
	}
	ix := buildRefIndex(srcs)
	var hints []ReviewHint
	famDone := map[string]bool{}
	for _, t := range toks {
		u := tokenUses(t, claims, ph)
		ids := u.ids()
		if len(ids) == 0 {
			continue
		}
		if strings.Contains(t, "*") {
			if ms := ix.globMatches(t); len(ms) >= 2 && usedAsValue(t, all) {
				hints = append(hints, ReviewHint{Kind: "unknown-ref", Claims: ids, Text: fmt.Sprintf(
					"`%s` is a pattern, but it is used as one value (%s). It stands for %d existing values (%s): which one does each use need, and does the check compare exact values?",
					t, listIDs(ids), len(ms), listCapped(ms))})
			}
			continue
		}
		if ix.dotted[t] || len(ix.literals[t]) > 0 {
			continue // exists
		}
		sibs := ix.siblings(t)
		created := u.create >= 0 || u.decl
		if !created {
			if len(sibs) >= 3 {
				hints = append(hints, ReviewHint{Kind: "unknown-ref", Claims: ids, Text: fmt.Sprintf(
					"`%s` (%s) is not in the code and no task creates it. Existing values of its family: %s. Is it one of these, or does a task need to add it?",
					t, listIDs(ids), listCapped(sibs))})
			}
			continue
		}
		if u.create >= 0 && u.first >= 0 && u.first < u.create {
			hints = append(hints, ReviewHint{Kind: "used-before-created", Claims: ids, Text: fmt.Sprintf(
				"`%s` is new, created by %s, but %s already uses it. What does the code do between those phases, and can the earlier phase's checks pass?",
				t, phaseLabel(ph[u.create], u.createT), phaseLabel(ph[u.first], u.firstT))})
		}
		if fam := family(t); len(sibs) > 0 && !famDone[fam] {
			famDone[fam] = true
			var news []string
			for _, o := range toks {
				if family(o) == fam && !ix.dotted[o] && len(ix.literals[o]) == 0 && !strings.Contains(o, "*") && len(tokenUses(o, claims, ph).ids()) > 0 {
					news = append(news, "`"+o+"`")
				}
			}
			hints = append(hints, ReviewHint{Kind: "existing-siblings", Claims: ids, Text: fmt.Sprintf(
				"%s %s new, and the code already has %s values: %s. Why not reuse them, and how do the new and old ones relate (who holds which)?",
				strings.Join(news, ", "), isAre(len(news)), fam+"*", listCapped(sibs))})
		}
	}
	hints = append(hints, uncoveredSiteHints(ix, all, claims, ph)...)
	return hints
}

// uncoveredSiteHints: a task replaces or removes a symbol or literal; list the
// sites of it the plan never names, and sites outside the task's own check.
func uncoveredSiteHints(ix refIndex, all string, claims []ReviewClaim, ph []planPhase) []ReviewHint {
	type target struct {
		ids    []string
		scopes []string
	}
	targets := map[string]*target{}
	var order []string
	consider := func(line, id string) {
		line = stripRejected(line)
		if !replaceWordRE.MatchString(line) {
			return
		}
		var scopes []string
		for _, m := range grepScopeRE.FindAllStringSubmatch(line, -1) {
			scopes = append(scopes, strings.TrimSuffix(strings.TrimPrefix(m[1], "./"), "/"))
		}
		for _, span := range replacedSpans(line) {
			var cands []string
			if plainSymbolRE.MatchString(span) {
				cands = append(cands, span)
			}
			for _, q := range quotedInSpan.FindAllStringSubmatch(span, -1) {
				cands = append(cands, q[1])
			}
			for _, c := range cands {
				if !distinctiveSymbol(c) {
					continue
				}
				t := targets[c]
				if t == nil {
					t = &target{}
					targets[c] = t
					order = append(order, c)
				}
				if id != "" {
					t.ids = appendUnique(t.ids, id)
				}
				for _, s := range scopes {
					t.scopes = appendUnique(t.scopes, s)
				}
			}
		}
	}
	for _, c := range claims {
		if c.Section == "Decisions" || c.Section == "Requirements" {
			consider(c.Text, c.ID)
		}
	}
	for _, p := range ph {
		for _, l := range p.lines {
			consider(l, lineID(l))
		}
	}
	var hints []ReviewHint
	for _, sym := range order {
		t := targets[sym]
		if len(t.ids) == 0 {
			continue
		}
		// Only symbols the repo defines or quotes: a library call is not "every site".
		defLangs := definedInRepo(ix.srcs, sym)
		literal := len(defLangs) == 0
		if literal && len(ix.literals[sym]) == 0 {
			continue
		}
		word := regexp.MustCompile(`(?:^|[^\w])` + regexp.QuoteMeta(sym) + `(?:$|[^\w])`)
		var sites, uncovered, outside []string
		for _, s := range ix.srcs {
			if s.test || langFamily(s.path) == "sql" || langFamily(s.path) == "" || (!literal && !defLangs[langFamily(s.path)]) {
				continue
			}
			for _, l := range s.lines {
				if word.MatchString(l) && !defLineRE.MatchString(l) && !goFuncLineRE.MatchString(l) {
					sites = appendUnique(sites, s.path)
					if !pathNamed(all, s.path) {
						uncovered = appendUnique(uncovered, s.path)
					}
					if len(t.scopes) > 0 && !underAny(s.path, t.scopes) {
						outside = appendUnique(outside, s.path)
					}
					break
				}
			}
		}
		// A literal quoted all over the repo (updated_at) is a common word, not one call to move.
		if len(uncovered) == 0 && len(outside) == 0 || literal && len(sites) > refsMaxLiteralSites {
			continue
		}
		msg := fmt.Sprintf("%s replace or remove `%s`, which %d files use", listIDs(t.ids), sym, len(sites))
		if len(uncovered) > 0 {
			msg += fmt.Sprintf("; the plan never names %s", listPaths(uncovered))
		}
		if len(outside) > 0 {
			msg += fmt.Sprintf("; the task's check searches only %s, so it passes while %s still use it", strings.Join(t.scopes, ", "), listPaths(outside))
		}
		msg += ". Does every site get the change?"
		hints = append(hints, ReviewHint{Kind: "uncovered-sites", Claims: t.ids, Text: msg})
	}
	return hints
}

// usedAsValue reports whether some use of tok reads as the single value of a
// mapping or gate ("POST → `x`", "gate on `x`"), not one item of a list.
func usedAsValue(tok, text string) bool {
	bt := "`" + tok + "`"
	for off := 0; ; {
		i := strings.Index(text[off:], bt)
		if i < 0 {
			return false
		}
		i += off
		from := i - 40
		if from < 0 {
			from = 0
		}
		if valueUseRE.MatchString(text[from:i]) {
			return true
		}
		off = i + len(bt)
	}
}

// stripRejected drops a "Rejected: …" tail: alternatives the plan does not take.
func stripRejected(s string) string {
	if i := strings.Index(s, "Rejected:"); i >= 0 {
		return s[:i]
	}
	return s
}

// replacedSpans returns the backticked spans a replace/remove verb acts on:
// spans before the verb or between it and the destination ("to", "with", "by"),
// in the same sentence. "Move each to X" refers back to the previous sentence.
// Sentences about deferred work are skipped.
func replacedSpans(line string) []string {
	// Split on sentence ends outside backticks.
	var sents []string
	start, inTick := 0, false
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '`':
			inTick = !inTick
		case !inTick && (line[i] == '.' || line[i] == ';') && i+1 < len(line) && (line[i+1] == ' ' || line[i+1] == '\t'):
			sents = append(sents, line[start:i])
			start = i + 1
		}
	}
	sents = append(sents, line[start:])
	spans := func(s string) []string {
		var out []string
		for _, m := range backtickSpan.FindAllStringSubmatch(s, -1) {
			out = append(out, m[1])
		}
		return out
	}
	var out []string
	for i, sent := range sents {
		loc := replaceWordRE.FindStringIndex(sent)
		if loc == nil || strings.Contains(strings.ToLower(sent), "defer") {
			continue
		}
		verb := strings.ToLower(sent[loc[0]:loc[1]])
		var got []string
		switch {
		case strings.HasPrefix(verb, "mov"):
			// "move X off/to Y": X sits inside the verb match.
			got = spans(sent[loc[0]:loc[1]])
		case passiveVerbRE.MatchString(verb):
			// "X calls are replaced by Y": X is the clause just before the verb.
			before := sent[:loc[0]]
			if j := strings.LastIndexAny(before, ",:("); j >= 0 {
				before = before[j+1:]
			}
			got = spans(before)
		default:
			// "replace X with Y", "remove X": X follows the verb, up to the destination
			// or the end of the clause.
			after := sent[loc[1]:]
			if d := destWordRE.FindStringIndex(after); d != nil {
				after = after[:d[0]]
			}
			if j := strings.IndexAny(after, ",:"); j >= 0 {
				after = after[:j]
			}
			got = spans(after)
		}
		if len(got) == 0 && i > 0 && pronounRE.MatchString(sent[loc[0]:min(len(sent), loc[1]+12)]) {
			got = spans(sents[i-1])
		}
		out = append(out, got...)
	}
	return out
}

// distinctiveSymbol keeps names unlikely to be ordinary words.
func distinctiveSymbol(s string) bool {
	if len(s) < 6 {
		return false
	}
	return strings.ContainsAny(s, "_.") || strings.ToLower(s) != s
}

// definedInRepo returns the language families that define sym. A Go-style
// "pkg.Name" must be defined in a directory named pkg, so os.Remove or
// json.Marshal (library calls) do not count.
func definedInRepo(srcs []sourceFile, sym string) map[string]bool {
	name, qual := sym, ""
	if i := strings.LastIndex(sym, "."); i >= 0 {
		name, qual = sym[i+1:], sym[:i]
	}
	pkgQual := qual != "" && strings.ToLower(qual[:1]) == qual[:1]
	re := regexp.MustCompile(`(?:function\*?|def|fn|func|class|interface|type|const|let|var)\s+(?:\([^)]*\)\s*)?` + regexp.QuoteMeta(name) + `\b`)
	out := map[string]bool{}
	for _, s := range srcs {
		if s.test || (pkgQual && filepath.Base(pathDir(s.path)) != qual && moduleBase(s.path) != qual) {
			continue
		}
		for _, l := range s.lines {
			if strings.Contains(l, name) && re.MatchString(l) {
				out[langFamily(s.path)] = true
				break
			}
		}
	}
	return out
}

func moduleBase(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// pathNamed reports whether the plan names a file: its path, or the short forms
// plans use (no src/ prefix, no extension, a Next.js route dir).
func pathNamed(text, path string) bool {
	cands := []string{path, strings.TrimPrefix(path, "src/")}
	noExt := strings.TrimSuffix(path, "."+path[strings.LastIndex(path, ".")+1:])
	cands = append(cands, noExt)
	if strings.HasSuffix(noExt, "/route") || strings.HasSuffix(noExt, "/page") {
		dir := pathDir(noExt)
		for _, pre := range []string{"src/app/api/", "app/api/", "src/app/", "app/", "pages/api/"} {
			if strings.HasPrefix(dir, pre) {
				cands = append(cands, strings.TrimPrefix(dir, pre))
				break
			}
		}
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		re := regexp.MustCompile("(?:^|[^A-Za-z0-9_/\\[\\]-])" + regexp.QuoteMeta(c) + "(?:$|[^A-Za-z0-9_\\[-])")
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

func underAny(path string, dirs []string) bool {
	for _, d := range dirs {
		if path == d || strings.HasPrefix(path, d+"/") {
			return true
		}
	}
	return false
}

func listCapped(xs []string) string {
	q := make([]string, 0, len(xs))
	for i, x := range xs {
		if i == refsMaxList {
			q = append(q, fmt.Sprintf("and %d more", len(xs)-refsMaxList))
			break
		}
		q = append(q, "`"+x+"`")
	}
	return strings.Join(q, ", ")
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}
