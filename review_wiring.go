package qrtx

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Two defects graders still found after the writers/callers packet
// (docs/SDD_VS_OPSX_BASELINE.md section 7.10):
//   - a new setting read only by one construction path (SettingsFromEnv) while
//     production builds the object another way (New(cfg…) in the server), so
//     the feature and its rollback switch never apply;
//   - a claim about "every call site" that does not hold for every call site.

var (
	envTokenRE   = regexp.MustCompile("`([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+)`")
	getenvLitRE  = regexp.MustCompile(`"([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+)"`)
	allCallersRE = regexp.MustCompile(`(?i)\b(?:every|all|each)\s+(?:\w+\s+){0,2}(?:call[\s-]?sites?|callers?|call\s+paths?|code\s+paths?|paths?)\b`)
	backtickRE   = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_]*(?:\\.[A-Za-z_][A-Za-z0-9_]*)?)(?:\\(\\))?`")
)

const (
	allCallersMaxSites = 10
	allCallersContext  = 3
)

// newSettings returns env var names the plan introduces (absent from non-test
// sources), with the claim ids naming each.
func newSettings(claims []ReviewClaim, srcs []sourceFile) map[string][]string {
	out := map[string][]string{}
	for _, c := range claims {
		if c.Section == "Acceptance" {
			continue
		}
		for _, m := range envTokenRE.FindAllStringSubmatch(c.Text, -1) {
			out[m[1]] = appendUnique(out[m[1]], c.ID)
		}
	}
	if len(out) == 0 {
		return out
	}
	for _, s := range srcs {
		if s.test {
			continue
		}
		for _, line := range s.lines {
			for name := range out {
				if strings.Contains(line, name) {
					delete(out, name)
				}
			}
		}
	}
	return out
}

// settingHomes finds the Go packages that read env vars sharing the new name's
// prefix (all but the last segment): a new APP_LLM_GATE_LEASE belongs with the
// packages already reading APP_LLM_GATE_MAX_WAITERS. Several homes mean the same
// family of settings is read in more than one place.
func settingHomes(name string, srcs []sourceFile) []string {
	// Shorten the prefix one segment at a time (APP_GATE_MAX_HOLD_SEC →
	// APP_GATE_MAX_HOLD_ → APP_GATE_MAX_ → APP_GATE_) until a family is found,
	// keeping at least three segments so a broad family such as APP_AGENT_
	// (many unrelated packages) never matches.
	prefix := name
	for {
		i := strings.LastIndex(strings.TrimSuffix(prefix, "_"), "_")
		if i <= 0 {
			return nil
		}
		prefix = prefix[:i+1]
		if strings.Count(prefix, "_") < 3 {
			return nil
		}
		if homes := envFamilyDirs(prefix, srcs); len(homes) > 0 {
			return homes
		}
	}
}

// envFamilyDirs lists package dirs whose non-test Go code names an env var with prefix.
func envFamilyDirs(prefix string, srcs []sourceFile) []string {
	seen := map[string]bool{}
	for _, s := range srcs {
		if s.test || !strings.HasSuffix(s.path, ".go") || seen[pathDir(s.path)] {
			continue
		}
		for _, line := range s.lines {
			if !strings.Contains(line, prefix) || !(strings.Contains(line, "Getenv") || strings.Contains(line, "LookupEnv")) {
				continue
			}
			for _, m := range getenvLitRE.FindAllStringSubmatch(line, -1) {
				if strings.HasPrefix(m[1], prefix) {
					seen[pathDir(s.path)] = true
				}
			}
		}
	}
	var out []string
	for dir := range seen {
		out = append(out, dir)
	}
	sort.Strings(out)
	return out
}

type buildSite struct {
	fn, path string
	line     int
	text     string
}

// buildSites lists production calls that construct or configure package dir:
// pkg.New…(, pkg.Init(, pkg.…FromEnv(, pkg.Global(.
func buildSites(dir string, srcs []sourceFile, defs map[string][]codeDef) []buildSite {
	pkg := regexp.QuoteMeta(filepath.Base(dir))
	re := regexp.MustCompile(`\b` + pkg + `\.(New\w*|Init|\w*FromEnv|Global)\(`)
	local := regexp.MustCompile(`(?:^|[^.\w])(New\w*|Init|\w*FromEnv|Global)\(`)
	var out []buildSite
	for _, s := range srcs {
		if s.test || !strings.HasSuffix(s.path, ".go") {
			continue
		}
		in := pathDir(s.path) == dir
		for i, line := range s.lines {
			if goFuncLineRE.MatchString(line) {
				continue
			}
			var m []string
			if in {
				m = local.FindStringSubmatch(line)
			} else {
				m = re.FindStringSubmatch(line)
			}
			if m == nil || !isBuildFunc(m[1], dir, defs) {
				continue
			}
			out = append(out, buildSite{fn: m[1], path: s.path, line: i + 1, text: truncateRunes(strings.TrimSpace(line), 120)})
		}
	}
	return out
}

// isBuildFunc keeps New, Init, Global and …FromEnv, and New<T> only when T is
// a struct type of the package (NewStore builds Store; NewSessionID does not).
func isBuildFunc(fn, dir string, defs map[string][]codeDef) bool {
	if fn == "New" || fn == "Init" || fn == "Global" || strings.HasSuffix(fn, "FromEnv") {
		return true
	}
	typ := strings.TrimPrefix(fn, "New")
	for _, d := range defs[typ] {
		if d.isType && pathDir(d.path) == dir {
			return true
		}
	}
	return false
}

// settingHints flags a new setting whose package is built by two or more paths
// the plan does not all cite.
func settingHints(text string, claims []ReviewClaim, srcs []sourceFile, defs map[string][]codeDef, isCited func(string) bool) ([]ReviewTouch, []ReviewHint) {
	news := newSettings(claims, srcs)
	names := make([]string, 0, len(news))
	for n := range news {
		names = append(names, n)
	}
	sort.Strings(names)
	var touches []ReviewTouch
	var hints []ReviewHint
	done := map[string]bool{}
	for _, n := range names {
		homes := settingHomes(n, srcs)
		key := strings.Join(homes, ",")
		if len(homes) == 0 || done[key] {
			continue
		}
		done[key] = true
		// Every new setting sharing these homes joins the same hint.
		var group, ids []string
		for _, m := range names {
			if strings.Join(settingHomes(m, srcs), ",") == key {
				group = append(group, "`"+m+"`")
				for _, id := range news[m] {
					ids = appendUnique(ids, id)
				}
			}
		}
		paths := map[string]bool{}
		var uncited []string
		for _, dir := range homes {
			paths["read in "+dir] = true
			for _, st := range buildSites(dir, srcs, defs) {
				paths[st.fn] = true
				t := ReviewTouch{Key: "build " + filepath.Base(dir), Kind: "build", Path: st.path, Line: st.line, Text: st.text, Cited: isCited(st.path)}
				touches = append(touches, t)
				if !t.Cited {
					uncited = appendUnique(uncited, fmt.Sprintf("%s:%d %s()", st.path, st.line, st.fn))
				}
			}
		}
		uncitedHomes := 0
		for _, dir := range homes {
			if !strings.Contains(text, dir) {
				uncitedHomes++
			}
		}
		builds := 0
		for w := range paths {
			if !strings.HasPrefix(w, "read in ") {
				builds++
			}
		}
		// The defect is two ways to build the same object; one constructor
		// with settings read in several places is not it.
		if builds < 2 || (len(uncited) == 0 && uncitedHomes == 0) {
			continue
		}
		var ways []string
		for w := range paths {
			if !strings.HasPrefix(w, "read in ") {
				ways = append(ways, w+"()")
			}
		}
		sort.Strings(ways)
		msg := fmt.Sprintf("%s %s new. Settings of this family are read in %s, and the object is built through %s", strings.Join(group, ", "), isAre(len(group)), strings.Join(homes, " and "), strings.Join(ways, ", "))
		if len(uncited) > 0 {
			msg += "; uncited build sites: " + listPaths(uncited)
		}
		msg += ". Which path does production use, and does the setting (and its off switch) reach every one?"
		hints = append(hints, ReviewHint{Kind: "setting-unwired", Claims: ids, Text: msg})
	}
	return capTouches(touches), hints
}

// allCallerHints checks claims about every caller against the real call sites.
func allCallerHints(text string, claims []ReviewClaim, defs map[string][]codeDef, srcs []sourceFile, citedDirs map[string]bool, aliases tsAliases) ([]ReviewCode, []ReviewHint) {
	var code []ReviewCode
	var hints []ReviewHint
	seen := map[string]bool{}
	for _, c := range claims {
		if c.Section == "Acceptance" || !allCallersRE.MatchString(c.Text) {
			continue
		}
		for _, m := range backtickRE.FindAllStringSubmatch(c.Text, -1) {
			tok := m[1]
			name := tok
			if i := strings.LastIndex(tok, "."); i >= 0 {
				name = tok[i+1:]
			}
			dirs := map[string]bool{}
			for _, d := range defs[name] {
				dirs[pathDir(d.path)] = true
			}
			// Prefer cited packages; else accept a name defined in exactly one package.
			scope := map[string]bool{}
			for d := range dirs {
				if citedDirs[d] {
					scope[d] = true
				}
			}
			if len(scope) == 0 && len(dirs) == 1 {
				scope = dirs
			}
			if len(scope) == 0 {
				continue
			}
			for _, u := range symbolUses(defs, []planSymbol{{token: tok, claims: []string{c.ID}}}, scope, aliases) {
				if strings.HasPrefix(u.key, "new ") && !strings.HasSuffix(tok, strings.TrimPrefix(u.key, "new ")) {
					continue
				}
				if seen[c.ID+u.key] {
					continue
				}
				seen[c.ID+u.key] = true
				sites := callSites(u, srcs)
				if len(sites) == 0 {
					continue
				}
				var files []string
				for i, s := range sites {
					files = appendUnique(files, s.Path)
					if i < allCallersMaxSites {
						code = append(code, s)
					}
				}
				msg := fmt.Sprintf("%s says something holds for every caller of `%s`; there are %d call sites in %d files (%s). Check each one: does the claim hold there?", c.ID, u.key, len(sites), len(files), listPaths(files))
				var wrong []string
				for _, pm := range reviewPathRE.FindAllStringSubmatch(c.Text, -1) {
					p := strings.TrimPrefix(pm[1], "./")
					if langFamily(p) != "" && langFamily(p) != "sql" && !containsPathSuffix(files, p) && !u.defs[p] {
						wrong = append(wrong, p)
					}
				}
				if len(wrong) > 0 {
					msg += fmt.Sprintf(" %s does not call it.", strings.Join(wrong, ", "))
				}
				hints = append(hints, ReviewHint{Kind: "all-callers", Claims: []string{c.ID}, Text: msg})
			}
		}
	}
	return code, hints
}

func containsPathSuffix(paths []string, p string) bool {
	for _, x := range paths {
		if x == p || strings.HasSuffix(x, "/"+p) {
			return true
		}
	}
	return false
}

// callSites returns each non-test call of u with a few lines of context.
func callSites(u symbolUse, srcs []sourceFile) []ReviewCode {
	var out []ReviewCode
	for _, s := range srcs {
		if s.test || !u.inScope(s, strings.Contains(u.key, ".")) {
			continue
		}
		for i, line := range s.lines {
			if u.isDefLine(line) || !u.match(s.path, line) {
				continue
			}
			from, to := i+1-allCallersContext, i+1+allCallersContext
			if from < 1 {
				from = 1
			}
			if to > len(s.lines) {
				to = len(s.lines)
			}
			out = append(out, ReviewCode{Path: s.path, From: from, To: to, Snippet: strings.Join(s.lines[from-1:to], "\n")})
		}
	}
	return out
}
