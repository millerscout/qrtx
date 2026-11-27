//go:build cgo

package qrtx

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// treeSitterEnabled reports whether non-Go definitions are indexed.
const treeSitterEnabled = true

type tsLang struct {
	lang  *sitter.Language
	query string
}

// Definition queries. @func, @type and @method name the captured identifier.
const jsDefQuery = `
(function_declaration name: (identifier) @func)
(generator_function_declaration name: (identifier) @func)
(class_declaration name: (_) @type)
(method_definition name: (property_identifier) @method)
(variable_declarator name: (identifier) @func value: [(arrow_function) (function_expression)])
`

const tsDefQuery = jsDefQuery + `
(abstract_class_declaration name: (type_identifier) @type)
(interface_declaration name: (type_identifier) @type)
(function_signature name: (identifier) @func)
`

var (
	tsLangsOnce sync.Once
	tsLangs     map[string]tsLang
)

func treeSitterLangs() map[string]tsLang {
	tsLangsOnce.Do(func() {
		tsLangs = map[string]tsLang{
			".ts":  {typescript.GetLanguage(), tsDefQuery},
			".tsx": {tsx.GetLanguage(), tsDefQuery},
			".js":  {javascript.GetLanguage(), jsDefQuery},
			".jsx": {javascript.GetLanguage(), jsDefQuery},
			".mjs": {javascript.GetLanguage(), jsDefQuery},
			".cjs": {javascript.GetLanguage(), jsDefQuery},
			".py": {python.GetLanguage(), `
(function_definition name: (identifier) @func)
(class_definition name: (identifier) @type)
`},
			".rs": {rust.GetLanguage(), `
(function_item name: (identifier) @func)
(struct_item name: (type_identifier) @type)
(enum_item name: (type_identifier) @type)
(trait_item name: (type_identifier) @type)
`},
			".swift": {swift.GetLanguage(), `
(function_declaration name: (simple_identifier) @func)
(class_declaration name: (type_identifier) @type)
(protocol_declaration name: (type_identifier) @type)
`},
		}
	})
	return tsLangs
}

var (
	tsQueryMu    sync.Mutex
	tsQueryCache = map[string]*sitter.Query{}
)

func treeSitterQuery(ext string, l tsLang) *sitter.Query {
	tsQueryMu.Lock()
	defer tsQueryMu.Unlock()
	if q, ok := tsQueryCache[ext]; ok {
		return q
	}
	q, err := sitter.NewQuery([]byte(l.query), l.lang)
	if err != nil {
		q = nil // a grammar without one of the node types: no defs for it
	}
	tsQueryCache[ext] = q
	return q
}

// treeSitterDefs indexes non-test definitions in non-Go source files.
func treeSitterDefs(srcs []sourceFile, out map[string][]codeDef, want map[string]bool) {
	langs := treeSitterLangs()
	parser := sitter.NewParser()
	defer parser.Close()
	for _, s := range srcs {
		ext := filepath.Ext(s.path)
		l, ok := langs[ext]
		if s.test || !ok {
			continue
		}
		q := treeSitterQuery(ext, l)
		if q == nil {
			continue
		}
		src := []byte(strings.Join(s.lines, "\n"))
		if !mentionsAny(src, want) {
			continue
		}
		parser.SetLanguage(l.lang)
		tree, err := parser.ParseCtx(context.Background(), nil, src)
		if err != nil || tree == nil {
			continue
		}
		qc := sitter.NewQueryCursor()
		qc.Exec(q, tree.RootNode())
		for {
			m, ok := qc.NextMatch()
			if !ok {
				break
			}
			for _, c := range m.Captures {
				name := c.Node.Content(src)
				d := codeDef{name: name, path: s.path, lang: langFamily(s.path), line: int(c.Node.StartPoint().Row) + 1}
				switch q.CaptureNameForId(c.Index) {
				case "type":
					d.isType = true
				case "method":
					d.recv = enclosingTypeName(c.Node, src)
				case "func":
					// A Python function inside a class, or a Rust fn in an impl, is a method.
					d.recv = enclosingTypeName(c.Node, src)
				}
				out[name] = append(out[name], d)
			}
		}
		qc.Close()
		tree.Close()
	}
}

// enclosingTypeName is the class, impl or struct a definition sits in, if any.
func enclosingTypeName(n *sitter.Node, src []byte) string {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch p.Type() {
		case "function_declaration", "function_definition", "function_item", "arrow_function", "function_expression":
			if p.ChildByFieldName("name") != nil && p.ChildByFieldName("name").Content(src) != n.Content(src) {
				return "" // nested in another function: a local helper
			}
		case "class_declaration", "class_definition", "abstract_class_declaration", "class", "struct_item":
			if name := p.ChildByFieldName("name"); name != nil {
				return name.Content(src)
			}
			return ""
		case "impl_item":
			if typ := p.ChildByFieldName("type"); typ != nil {
				return typ.Content(src)
			}
			return ""
		}
	}
	return ""
}
