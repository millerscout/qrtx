package qrtx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The review packet started Go-only (go/parser defs, package-qualified call
// regexes). A TypeScript plan (rbac-route-gates, 2026-11-23) got zero hints
// while a manual review found nine defects, so definitions for other languages
// come from tree-sitter and calls are matched per language family.

// langFamily groups extensions whose files can call each other's code.
func langFamily(path string) string {
	switch filepath.Ext(path) {
	case ".go":
		return "go"
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue":
		return "js"
	case ".py":
		return "py"
	case ".rs":
		return "rs"
	case ".swift":
		return "swift"
	case ".sql":
		return "sql"
	}
	return ""
}

// defLineRE marks lines that define rather than call (skipped as call sites).
var defLineRE = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:static\s+)?(?:function\*?|def|fn|func|class)\s`)

// moduleKey is the import-comparable form of a source path: no extension, no
// trailing /index or /__init__.
func moduleKey(path string) string {
	p := strings.TrimSuffix(path, filepath.Ext(path))
	p = strings.TrimSuffix(p, "/index")
	p = strings.TrimSuffix(p, "/__init__")
	return p
}

var (
	jsImportRE = regexp.MustCompile(`(?:\bfrom\s+|\bimport\s*\(\s*|\brequire\s*\(\s*|^\s*import\s+)['"]([^'"]+)['"]`)
	pyImportRE = regexp.MustCompile(`^\s*(?:from\s+([\w.]+)\s+import\b|import\s+([\w.]+))`)
	rsUseRE    = regexp.MustCompile(`^\s*(?:pub\s+)?use\s+([\w:]+)`)
)

// tsAliases maps tsconfig path aliases ("@/") to repo dirs ("src/").
type tsAliases map[string]string

var tsconfigCommentRE = regexp.MustCompile(`(?m)^\s*//.*$`)

func loadTSAliases(repoRoot string) tsAliases {
	out := tsAliases{}
	raw, err := os.ReadFile(filepath.Join(repoRoot, "tsconfig.json"))
	if err == nil {
		var cfg struct {
			CompilerOptions struct {
				BaseURL string              `json:"baseUrl"`
				Paths   map[string][]string `json:"paths"`
			} `json:"compilerOptions"`
		}
		if json.Unmarshal(tsconfigCommentRE.ReplaceAll(raw, nil), &cfg) == nil {
			base := strings.Trim(strings.TrimPrefix(cfg.CompilerOptions.BaseURL, "./"), "/")
			for k, v := range cfg.CompilerOptions.Paths {
				if len(v) == 0 || !strings.HasSuffix(k, "/*") {
					continue
				}
				target := strings.TrimSuffix(strings.TrimPrefix(v[0], "./"), "*")
				if base != "" && base != "." {
					target = base + "/" + target
				}
				out[strings.TrimSuffix(k, "*")] = target
			}
		}
	}
	if len(out) == 0 {
		out["@/"] = "src/"
		out["~/"] = "src/"
	}
	return out
}

// importsModule reports whether a non-Go file imports the module defined at defPath.
func importsModule(s sourceFile, defPath string, aliases tsAliases) bool {
	want := moduleKey(defPath)
	switch langFamily(s.path) {
	case "js":
		for _, l := range s.lines {
			for _, m := range jsImportRE.FindAllStringSubmatch(l, -1) {
				spec := m[1]
				var got string
				switch {
				case strings.HasPrefix(spec, "."):
					got = filepath.ToSlash(filepath.Clean(filepath.Join(pathDir(s.path), spec)))
				default:
					for a, dir := range aliases {
						if strings.HasPrefix(spec, a) {
							got = dir + strings.TrimPrefix(spec, a)
						}
					}
				}
				if got != "" && moduleKey(got+".x") == want {
					return true
				}
			}
		}
	case "py":
		for _, l := range s.lines {
			m := pyImportRE.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			mod := m[1]
			if mod == "" {
				mod = m[2]
			}
			if strings.HasPrefix(mod, ".") {
				// Relative import: resolve against the file's package dir.
				dots := len(mod) - len(strings.TrimLeft(mod, "."))
				dir := pathDir(s.path)
				for i := 1; i < dots; i++ {
					dir = pathDir(dir)
				}
				mod = dir + "/" + strings.ReplaceAll(strings.TrimLeft(mod, "."), ".", "/")
			} else {
				mod = strings.ReplaceAll(mod, ".", "/")
			}
			mod = strings.TrimSuffix(mod, "/")
			if mod == want || strings.HasSuffix(want, "/"+mod) {
				return true
			}
		}
	case "rs":
		name := filepath.Base(want)
		for _, l := range s.lines {
			if m := rsUseRE.FindStringSubmatch(l); m != nil && strings.Contains("::"+m[1]+"::", "::"+name+"::") {
				return true
			}
		}
	case "swift":
		return true // one module per target; no per-file imports
	}
	return false
}

var defKeywords = map[string]bool{"function": true, "def": true, "fn": true, "func": true, "class": true, "struct": true, "enum": true, "trait": true, "interface": true, "protocol": true, "const": true, "let": true, "var": true}

// varKeywords define a function only when assigned one (= () => or = function).
var varKeywords = map[string]bool{"const": true, "let": true, "var": true}

var methodModifiers = map[string]bool{"public": true, "private": true, "protected": true, "static": true, "async": true, "get": true, "set": true, "override": true, "readonly": true, "pub": true, "export": true, "default": true}

// mentionsAny reports whether src may define one of the wanted names: the
// name follows a definition keyword, or starts a line (after modifiers) and is
// followed by "(" or "<" on a line ending in "{" (a method). One pass; files that cannot define them are skipped.
func mentionsAny(src []byte, want map[string]bool) bool {
	isWord := func(c byte) bool {
		return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
	}
	prev, prevEnd := "", 0
	lineStart := true // only modifiers seen since the line began
	for i := 0; i < len(src); {
		c := src[i]
		if c == '\n' {
			lineStart, prev = true, ""
			i++
			continue
		}
		if !isWord(c) {
			if c != ' ' && c != '\t' && c != '*' {
				lineStart = false
			}
			i++
			continue
		}
		j := i
		for j < len(src) && isWord(src[j]) {
			j++
		}
		w := string(src[i:j])
		if want[w] {
			if defKeywords[prev] && onlySpace(src[prevEnd:i]) && (!varKeywords[prev] || lineDefinesFunc(src, j)) {
				return true
			}
			if lineStart {
				k := j
				for k < len(src) && (src[k] == ' ' || src[k] == '\t') {
					k++
				}
				if k < len(src) && (src[k] == '(' || src[k] == '<') && lineEndsWithBrace(src, k) {
					return true
				}
			}
		}
		if !methodModifiers[w] {
			lineStart = false
		}
		prev, prevEnd = w, j
		i = j
	}
	return false
}

// lineEndsWithBrace reports whether the line holding src[k] ends in "{".
func lineEndsWithBrace(src []byte, k int) bool {
	e := k
	for e < len(src) && src[e] != '\n' {
		e++
	}
	line := strings.TrimRight(string(src[k:e]), " \t\r")
	return strings.HasSuffix(line, "{")
}

// lineDefinesFunc reports whether the rest of the line assigns a function.
func lineDefinesFunc(src []byte, k int) bool {
	e := k
	for e < len(src) && src[e] != '\n' {
		e++
	}
	rest := string(src[k:e])
	return strings.Contains(rest, "=>") || strings.Contains(rest, "function")
}

func onlySpace(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '*' {
			return false
		}
	}
	return len(b) > 0
}
