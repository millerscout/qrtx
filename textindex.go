package qrtx

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TextIndex is a CodeIndex that scans source files under Root for
// definitions. It needs no database. Related and Semantic return nothing.
type TextIndex struct {
	Root string
}

const textIndexMaxFileBytes = 1 << 20

// Symbols returns lines under Root that define name (func, def, class, type, ...).
func (ix TextIndex) Symbols(_ string, name string, limit int) []CodeHit {
	name = strings.TrimSpace(name)
	if ix.Root == "" || name == "" || limit <= 0 || !identRE.MatchString(name) {
		return nil
	}
	defRE := regexp.MustCompile(`\b(?:func|def|fn|function|class|struct|enum|trait|interface|type|protocol)\s+(?:\([^)]*\)\s*)?` + regexp.QuoteMeta(name) + `\b`)
	var hits []CodeHit
	_ = filepath.WalkDir(ix.Root, func(path string, d fs.DirEntry, err error) error {
		if len(hits) >= limit {
			return fs.SkipAll
		}
		if err != nil {
			return nil
		}
		if d.IsDir() {
			n := d.Name()
			if path != ix.Root && (strings.HasPrefix(n, ".") || n == "vendor" || n == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !touchExts[filepath.Ext(path)] || strings.Contains(d.Name(), "_test.") {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > textIndexMaxFileBytes {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), textIndexMaxFileBytes)
		for line := 1; sc.Scan(); line++ {
			if !defRE.MatchString(sc.Text()) {
				continue
			}
			rel, _ := filepath.Rel(ix.Root, path)
			hits = append(hits, CodeHit{Path: filepath.ToSlash(rel), Line: line})
			break
		}
		return nil
	})
	return hits
}

// Related returns nothing; TextIndex has no graph.
func (TextIndex) Related(string, string, int) []CodeHit { return nil }

// Semantic returns nothing; TextIndex has no embeddings.
func (TextIndex) Semantic(string, string, int) []CodeHit { return nil }

var identRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
