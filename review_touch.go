package qrtx

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The N=8 bench (docs/SDD_VS_OPSX_BASELINE.md section 7.6) found that SDD
// plans fail consistency when their design depends on code the plan never
// read: a Python script writing the same memory files, or five call sites of
// a constructor that a per-instance lock assumes is a singleton. The packet
// therefore lists other code that touches the same data or symbols, and adds a
// hint when that code is not cited.

// ReviewTouch is code outside the plan's citations that uses the same data or symbol.
type ReviewTouch struct {
	Key   string `json:"key"`
	Kind  string `json:"kind"` // "data" or "caller"
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Text  string `json:"text"`
	Cited bool   `json:"cited"`
}

const (
	touchMaxPerKey   = 8
	touchMaxFileSize = 1 << 20
)

var (
	touchExts = map[string]bool{".go": true, ".py": true, ".sh": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".vue": true, ".rs": true, ".swift": true, ".sql": true}
	touchSkip = map[string]bool{".git": true, "node_modules": true, ".claude": true, "sdd": true, "openspec": true, "vendor": true, "dist": true, "target": true, ".venv": true, "testdata": true}
	// Claims that change how data is stored, which other writers must follow.
	formatClaimRE = regexp.MustCompile(`(?i)\b(format|schema|migrat\w*|layout|envelope|version\w*|encod\w*|lock\w*|flock|atomic\w*|rename|journal|on-disk)\b`)
	// Claims whose design depends on how many instances or callers exist.
	stateClaimRE  = regexp.MustCompile(`(?i)\b(lock\w*|mutex|flock|per[- ]instance|per[- ]repository|singleton|cache\w*|guard\w*|serializ\w*|in-process|sync\.once|semaphore)\b`)
	symbolTokenRE = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_]*(?:\\.[A-Za-z_][A-Za-z0-9_]*)?)(?:\\(\\))?`|\\b([A-Z][a-z0-9]+[A-Z][A-Za-z0-9]*(?:\\.[A-Z][A-Za-z0-9]*)?)\\b")
	// Test trees in non-Go projects (tests/rbac/helpers.ts).
	testDirRE = regexp.MustCompile(`(?:^|/)(?:tests?|__tests__|spec|e2e)/`)
	// Built bundles (index-2BMm26z8.js, app.min.js) are not source anyone edits.
	generatedBundleRE = regexp.MustCompile(`(?:[-.][A-Za-z0-9_-]{8,}|\.min|\.bundle|\.chunk)\.(?:js|mjs|cjs|css)$`)
	genericSegment    = map[string]bool{".": true, "..": true, ".config": true, ".local": true, "state": true, "share": true, "data": true, "tmp": true, "cache": true}
)

type sourceFile struct {
	path  string
	lines []string
	test  bool
}

// loadSources reads the repo's source files once (tests included, flagged).
func loadSources(repoRoot string) []sourceFile {
	var out []sourceFile
	_ = filepath.WalkDir(repoRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != repoRoot && (touchSkip[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !touchExts[filepath.Ext(p)] || generatedBundleRE.MatchString(d.Name()) {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > touchMaxFileSize {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, p)
		rel = filepath.ToSlash(rel)
		base := filepath.Base(rel)
		test := strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
			(langFamily(rel) != "go" && testDirRE.MatchString(rel))
		out = append(out, sourceFile{path: rel, lines: strings.Split(string(b), "\n"), test: test})
		return nil
	})
	return out
}

type storageKey struct {
	label string
	re    *regexp.Regexp
	from  string
}

// storageKeys finds where cited Go files keep data on disk: env var names read
// with os.Getenv and consecutive string segments passed to filepath.Join.
func storageKeys(repoRoot string, cited []citedPath) []storageKey {
	var out []storageKey
	seen := map[string]bool{}
	add := func(label string, re *regexp.Regexp, from string) {
		if seen[label] {
			return
		}
		seen[label] = true
		out = append(out, storageKey{label: label, re: re, from: from})
	}
	done := map[string]bool{}
	for _, c := range cited {
		if done[c.path] || !strings.HasSuffix(c.path, ".go") || strings.HasSuffix(c.path, "_test.go") {
			continue
		}
		done[c.path] = true
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(repoRoot, c.path), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch {
			case pkg.Name == "os" && (sel.Sel.Name == "Getenv" || sel.Sel.Name == "LookupEnv") && len(call.Args) == 1:
				if s, ok := stringLit(call.Args[0]); ok && len(s) >= 4 {
					add(s, regexp.MustCompile(`\b`+regexp.QuoteMeta(s)+`\b`), c.path)
				}
			case pkg.Name == "filepath" && sel.Sel.Name == "Join":
				var run []string
				flush := func() {
					segs := run
					run = nil
					for len(segs) > 0 && genericSegment[segs[0]] {
						segs = segs[1:]
					}
					if len(segs) < 2 {
						return
					}
					quoted := make([]string, len(segs))
					for i, s := range segs {
						quoted[i] = regexp.QuoteMeta(s)
					}
					// Matches "a/b", Python's "a" / "b" and Join("a", "b").
					add(strings.Join(segs, "/"), regexp.MustCompile(strings.Join(quoted, `["'\s]*[/,]\s*["']?`)), c.path)
				}
				for _, a := range call.Args {
					if s, ok := stringLit(a); ok && s != "" && !strings.ContainsAny(s, "%*") {
						run = append(run, strings.Trim(s, "/"))
						continue
					}
					flush()
				}
				flush()
			}
			return true
		})
	}
	return out
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

type codeDef struct {
	name, recv, path string
	isType           bool
	lang             string // langFamily of path
	line             int    // set for non-Go defs
}

// goDefs indexes non-test Go funcs, methods and struct types by name.
func goDefs(repoRoot string, srcs []sourceFile) map[string][]codeDef {
	out := map[string][]codeDef{}
	for _, s := range srcs {
		if s.test || !strings.HasSuffix(s.path, ".go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(repoRoot, s.path), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		for _, d := range f.Decls {
			switch decl := d.(type) {
			case *ast.FuncDecl:
				def := codeDef{name: decl.Name.Name, path: s.path, lang: "go"}
				if decl.Recv != nil && len(decl.Recv.List) == 1 {
					def.recv = recvName(decl.Recv.List[0].Type)
				}
				out[def.name] = append(out[def.name], def)
			case *ast.GenDecl:
				for _, sp := range decl.Specs {
					if ts, ok := sp.(*ast.TypeSpec); ok {
						if _, ok := ts.Type.(*ast.StructType); ok {
							out[ts.Name.Name] = append(out[ts.Name.Name], codeDef{name: ts.Name.Name, path: s.path, isType: true, lang: "go"})
						}
					}
				}
			}
		}
	}
	return out
}

// codeDefs indexes definitions in every supported language: Go through
// go/parser, the rest through tree-sitter.
// Only files containing one of the names the plan mentions are parsed.
func codeDefs(repoRoot string, srcs []sourceFile, text string) map[string][]codeDef {
	out := goDefs(repoRoot, srcs)
	treeSitterDefs(srcs, out, planNames(text))
	return out
}

var planNameRE = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

// planNames are the identifier-like words of backticked spans and CamelCase words.
func planNames(text string) map[string]bool {
	out := map[string]bool{}
	for _, m := range symbolTokenRE.FindAllString(text, -1) {
		for _, w := range planNameRE.FindAllString(m, -1) {
			out[w] = true
		}
	}
	for _, m := range regexp.MustCompile("`[^`\n]+`").FindAllString(text, -1) {
		for _, w := range planNameRE.FindAllString(m, -1) {
			out[w] = true
		}
	}
	return out
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return recvName(t.X)
	}
	return ""
}

type planSymbol struct {
	token  string
	claims []string
}

// planSymbols returns Go-looking names in stateful claims (locks, caches, claims).
func planSymbols(claims []ReviewClaim) []planSymbol {
	idx := map[string]int{}
	var out []planSymbol
	for _, c := range claims {
		// Only design claims: a scenario names a symbol to test it, not to rely on it.
		if (c.Section != "Decisions" && c.Section != "Assumptions" && c.Section != "Risks") || !stateClaimRE.MatchString(c.Text) {
			continue
		}
		for _, m := range symbolTokenRE.FindAllStringSubmatch(c.Text, -1) {
			tok := m[1]
			if tok == "" {
				tok = m[2]
			}
			if i, ok := idx[tok]; ok {
				out[i].claims = appendUnique(out[i].claims, c.ID)
				continue
			}
			idx[tok] = len(out)
			out = append(out, planSymbol{token: tok, claims: []string{c.ID}})
		}
	}
	return out
}

type symbolUse struct {
	key    string
	dir    string         // package directory of the definition
	local  *regexp.Regexp // use inside that package
	remote *regexp.Regexp // use from other packages (package-qualified)
	defs   map[string]bool
	claims []string
	// Non-Go definitions: the module is the defining file, and remote use
	// needs an import of it.
	lang    string
	module  string
	aliases tsAliases
}

func (u symbolUse) local_(path string) bool {
	if u.module != "" {
		return path == u.module
	}
	return pathDir(path) == u.dir
}

func (u symbolUse) match(path, line string) bool {
	if u.local_(path) {
		return u.local.MatchString(line)
	}
	return u.remote.MatchString(line)
}

// inScope reports whether file s can use u at all: same language family and,
// from another package or module, an import of it when the key needs one.
func (u symbolUse) inScope(s sourceFile, needImport bool) bool {
	if u.module == "" {
		if !strings.HasSuffix(s.path, ".go") {
			return false
		}
		return u.local_(s.path) || !needImport || importsDir(s.lines, u.dir)
	}
	if langFamily(s.path) != u.lang {
		return false
	}
	return u.local_(s.path) || importsModule(s, u.module, u.aliases)
}

// isDefLine reports a line that defines rather than calls.
func (u symbolUse) isDefLine(line string) bool {
	if u.module == "" {
		return goFuncLineRE.MatchString(line)
	}
	return defLineRE.MatchString(line)
}

// foreignUses builds call patterns for a definition outside Go.
func foreignUses(d codeDef, typ, name string, claims []string, aliases tsAliases) []symbolUse {
	q := regexp.QuoteMeta(name)
	base := symbolUse{dir: pathDir(d.path), defs: map[string]bool{d.path: true}, claims: claims, lang: d.lang, module: d.path, aliases: aliases}
	ctor := func(t string) *regexp.Regexp {
		tq := regexp.QuoteMeta(t)
		switch d.lang {
		case "js":
			return regexp.MustCompile(`\bnew\s+` + tq + `\s*[(<]`)
		case "rs":
			return regexp.MustCompile(`\b` + tq + `(?:::new\s*\(|\s*\{)`)
		}
		return regexp.MustCompile(`(?:^|[^.\w])` + tq + `\(`)
	}
	switch {
	case d.isType:
		u := base
		u.key, u.local, u.remote = "new "+name, ctor(name), ctor(name)
		return []symbolUse{u}
	case d.recv == "" && typ == "":
		u := base
		u.key = name + "()"
		u.local = regexp.MustCompile(`(?:^|[^.\w$])` + q + `\s*\(`)
		// Imported bare, or through a namespace import (mod.name).
		u.remote = regexp.MustCompile(`(?:^|[^\w$])` + q + `\s*\(`)
		return []symbolUse{u}
	case d.recv != "" && (typ == "" || d.recv == typ):
		u := base
		u.key = d.recv + "." + name + "()"
		u.local = regexp.MustCompile(`\.` + q + `\s*\(`)
		u.remote = u.local
		t := base
		t.key, t.local, t.remote = "new "+d.recv, ctor(d.recv), ctor(d.recv)
		return []symbolUse{u, t}
	}
	return nil
}

func pathDir(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return "."
}

// symbolUses resolves plan symbols defined in a cited package to call patterns:
// a type's constructor and literals, a function's calls, or a method's calls.
// Calls from other packages must be qualified with the package name, so a
// generic name such as New or Save does not match unrelated code.
func symbolUses(defs map[string][]codeDef, syms []planSymbol, citedDirs map[string]bool, aliases tsAliases) []symbolUse {
	var out []symbolUse
	seen := map[string]bool{}
	add := func(u symbolUse) {
		k := u.key + "@" + u.dir + "@" + u.module
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, u)
	}
	typeUse := func(typ string, d codeDef, claims []string) symbolUse {
		dir := pathDir(d.path)
		pkg := regexp.QuoteMeta(filepath.Base(dir))
		tq := regexp.QuoteMeta(typ)
		return symbolUse{
			key: "new " + typ, dir: dir, defs: map[string]bool{d.path: true}, claims: claims,
			local:  regexp.MustCompile(`(?:^|[^.\w])New` + tq + `\(|[&\s(=]` + tq + `\{`),
			remote: regexp.MustCompile(`\b` + pkg + `\.(?:New` + tq + `\(|` + tq + `\{)`),
		}
	}
	for _, s := range syms {
		typ, name := "", s.token
		if i := strings.LastIndex(s.token, "."); i >= 0 {
			typ, name = s.token[:i], s.token[i+1:]
		}
		var cands []codeDef
		receivers := map[string]bool{}
		for _, d := range defs[name] {
			if citedDirs[pathDir(d.path)] {
				cands = append(cands, d)
				if d.recv != "" {
					receivers[d.recv] = true
				}
			}
		}
		// A bare method name with several receivers is ambiguous: skip it.
		if typ == "" && len(receivers) > 1 {
			continue
		}
		for _, d := range cands {
			if d.lang != "go" {
				for _, u := range foreignUses(d, typ, name, s.claims, aliases) {
					add(u)
				}
				continue
			}
			dir := pathDir(d.path)
			q := regexp.QuoteMeta(name)
			pkg := regexp.QuoteMeta(filepath.Base(dir))
			switch {
			case d.isType:
				add(typeUse(name, d, s.claims))
			case d.recv == "" && typ == "":
				add(symbolUse{
					key: name + "()", dir: dir, defs: map[string]bool{d.path: true}, claims: s.claims,
					local:  regexp.MustCompile(`(?:^|[^.\w])` + q + `\(`),
					remote: regexp.MustCompile(`\b` + pkg + `\.` + q + `\(`),
				})
			case d.recv != "" && (typ == "" || d.recv == typ):
				// Method calls go through a value, so match .Name( in files of
				// the package or files that name the package.
				add(symbolUse{
					key: d.recv + "." + name + "()", dir: dir, defs: map[string]bool{d.path: true}, claims: s.claims,
					local:  regexp.MustCompile(`\.` + q + `\(`),
					remote: regexp.MustCompile(`\.` + q + `\(`),
				})
				// State kept per instance of the receiver depends on how many are made.
				for _, td := range defs[d.recv] {
					if td.isType && pathDir(td.path) == dir {
						add(typeUse(d.recv, td, s.claims))
					}
				}
			}
		}
	}
	return out
}

var goFuncLineRE = regexp.MustCompile(`^func\b`)

// reviewTouches lists code using the same storage keys or symbols as the plan.
func reviewTouches(repoRoot, text string, claims []ReviewClaim) ([]ReviewTouch, []ReviewHint, []ReviewCode) {
	cited := citedPaths(repoRoot, text)
	citedSet := map[string]bool{}
	for _, c := range cited {
		citedSet[c.path] = true
	}
	keys := storageKeys(repoRoot, cited)
	syms := planSymbols(claims)
	if len(keys) == 0 && len(syms) == 0 && !claimsNeedSources(claims) {
		return nil, nil, nil
	}
	srcs := loadSources(repoRoot)
	citedDirs := map[string]bool{}
	for _, c := range cited {
		citedDirs[pathDir(c.path)] = true
	}
	defs := codeDefs(repoRoot, srcs, text)
	aliases := loadTSAliases(repoRoot)
	uses := symbolUses(defs, syms, citedDirs, aliases)

	var touches []ReviewTouch
	var hints []ReviewHint
	isCited := func(p string) bool { return citedSet[p] || strings.Contains(text, p) }

	settingTouches, settingH := settingHints(text, claims, srcs, defs, isCited)
	touches = append(touches, settingTouches...)
	hints = append(hints, settingH...)
	callerCode, callerH := allCallerHints(text, claims, defs, srcs, citedDirs, aliases)
	hints = append(hints, callerH...)

	// Data keys the plan names (the store it changes) get one merged hint;
	// other keys read by the same cited files are listed without a hint.
	var named []string
	var namedUncited []string
	for _, k := range keys {
		var found []ReviewTouch
		var uncited []string
		for _, s := range srcs {
			if s.test || s.path == k.from {
				continue
			}
			for i, line := range s.lines {
				if k.re.MatchString(line) {
					t := ReviewTouch{Key: k.label, Kind: "data", Path: s.path, Line: i + 1, Text: truncateRunes(strings.TrimSpace(line), 120), Cited: isCited(s.path)}
					found = append(found, t)
					if !t.Cited {
						uncited = appendUnique(uncited, s.path)
					}
					break
				}
			}
		}
		touches = append(touches, capTouches(found)...)
		if len(uncited) > 0 && (strings.Contains(text, k.label) || k.re.MatchString(text)) {
			named = append(named, "`"+k.label+"`")
			for _, u := range uncited {
				namedUncited = appendUnique(namedUncited, u)
			}
		}
	}
	if len(named) > 0 {
		var ids []string
		for _, c := range claims {
			if (c.Section == "Decisions" || c.Section == "Assumptions" || c.ID == "RB") && formatClaimRE.MatchString(c.Text) {
				ids = append(ids, c.ID)
			}
		}
		if len(ids) > 0 {
			hints = append(hints, ReviewHint{
				Kind:   "uncited-writer",
				Claims: ids,
				Text:   fmt.Sprintf("%s, which the plan changes, %s also used by %s, never cited: do they read or write the same data, and do %s still hold when they run during or after the change?", strings.Join(named, " and "), isAre(len(named)), listPaths(namedUncited), listIDs(ids)),
			})
		}
	}

	for _, u := range uses {
		var found []ReviewTouch
		var uncited []string
		for _, s := range srcs {
			// A method called from another package needs that package imported.
			if s.test || !u.inScope(s, strings.HasSuffix(u.key, "()") && strings.Contains(u.key, ".")) {
				continue
			}
			for i, line := range s.lines {
				if u.isDefLine(line) || !u.match(s.path, line) {
					continue
				}
				t := ReviewTouch{Key: u.key, Kind: "caller", Path: s.path, Line: i + 1, Text: truncateRunes(strings.TrimSpace(line), 120), Cited: isCited(s.path)}
				found = append(found, t)
				if !t.Cited && !u.defs[s.path] {
					uncited = appendUnique(uncited, s.path)
				}
			}
		}
		touches = append(touches, capTouches(found)...)
		if len(uncited) < 2 {
			continue
		}
		hints = append(hints, ReviewHint{
			Kind:   "uncited-callers",
			Claims: u.claims,
			Text:   fmt.Sprintf("%s rely on `%s`, which %d uncited files also use (%s): does the design hold when each of them has its own instance or call?", listIDs(u.claims), u.key, len(uncited), listPaths(uncited)),
		})
	}
	return touches, hints, callerCode
}

// claimsNeedSources reports whether a claim names an env var or a claim about every caller.
func claimsNeedSources(claims []ReviewClaim) bool {
	for _, c := range claims {
		if envTokenRE.MatchString(c.Text) || allCallersRE.MatchString(c.Text) {
			return true
		}
	}
	return false
}

// capTouches keeps uncited touches first, then at most touchMaxPerKey.
func capTouches(ts []ReviewTouch) []ReviewTouch {
	sort.SliceStable(ts, func(i, j int) bool { return !ts[i].Cited && ts[j].Cited })
	if len(ts) > touchMaxPerKey {
		ts = ts[:touchMaxPerKey]
	}
	return ts
}

func listPaths(ps []string) string {
	sort.Strings(ps)
	if len(ps) > 5 {
		return strings.Join(ps[:5], ", ") + fmt.Sprintf(" and %d more", len(ps)-5)
	}
	return strings.Join(ps, ", ")
}

// importsDir reports whether a Go file imports the package in dir (by path suffix).
func importsDir(lines []string, dir string) bool {
	suffix := "/" + dir + `"`
	for _, l := range lines {
		if strings.Contains(l, suffix) {
			return true
		}
		if strings.HasPrefix(l, "func ") {
			return false
		}
	}
	return false
}

func listIDs(ids []string) string {
	if len(ids) > 4 {
		return strings.Join(ids[:4], ",") + ",…"
	}
	return strings.Join(ids, ",")
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
