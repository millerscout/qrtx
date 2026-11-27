package qrtx

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// exportSymbol is an exported API member from a Go package directory.
type exportSymbol struct {
	Name      string
	Kind      string
	Signature string
}

// loadPackageExports parses non-test Go files in dir and returns exported symbols.
func loadPackageExports(dir string) ([]exportSymbol, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".go") || strings.HasSuffix(ent.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, ent.Name())
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no Go files in %s", dir)
	}
	pkgName := files[0].Name.Name
	var out []exportSymbol
	seen := map[string]bool{}
	for _, f := range files {
		if f.Name.Name != pkgName {
			continue
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				kind := "func"
				sig := formatFuncSig(d)
				if d.Recv != nil {
					kind = "method"
				}
				addExport(&out, seen, d.Name.Name, kind, sig)
			case *ast.GenDecl:
				switch d.Tok {
				case token.TYPE:
					for _, spec := range d.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok || !ts.Name.IsExported() {
							continue
						}
						addExport(&out, seen, ts.Name.Name, "type", "type "+ts.Name.Name)
					}
				case token.CONST, token.VAR:
					for _, spec := range d.Specs {
						vs, ok := spec.(*ast.ValueSpec)
						if !ok {
							continue
						}
						for _, name := range vs.Names {
							if !name.IsExported() {
								continue
							}
							kind := "var"
							if d.Tok == token.CONST {
								kind = "const"
							}
							addExport(&out, seen, name.Name, kind, kind+" "+name.Name)
						}
					}
				}
			}
		}
	}
	return out, nil
}

func addExport(out *[]exportSymbol, seen map[string]bool, name, kind, sig string) {
	if seen[name] {
		return
	}
	seen[name] = true
	*out = append(*out, exportSymbol{Name: name, Kind: kind, Signature: normalizeSig(sig)})
}

func formatFuncSig(d *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func ")
	if d.Recv != nil && len(d.Recv.List) > 0 {
		b.WriteString(formatFieldList(d.Recv))
		b.WriteString(" ")
	}
	b.WriteString(d.Name.Name)
	b.WriteString(formatFieldList(d.Type.Params))
	if d.Type.Results != nil && len(d.Type.Results.List) > 0 {
		if len(d.Type.Results.List) == 1 && len(d.Type.Results.List[0].Names) == 0 {
			b.WriteString(" ")
			b.WriteString(typeExprString(d.Type.Results.List[0].Type))
		} else {
			b.WriteString(" ")
			b.WriteString(formatFieldList(d.Type.Results))
		}
	}
	return normalizeSig(b.String())
}

func formatFieldList(fl *ast.FieldList) string {
	if fl == nil {
		return "()"
	}
	var parts []string
	for _, f := range fl.List {
		typ := typeExprString(f.Type)
		if len(f.Names) == 0 {
			parts = append(parts, typ)
			continue
		}
		for _, n := range f.Names {
			parts = append(parts, n.Name+" "+typ)
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func typeExprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeExprString(t.X)
	case *ast.SelectorExpr:
		return typeExprString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeExprString(t.Elt)
		}
		return fmt.Sprintf("[%s]%s", typeExprString(t.Len), typeExprString(t.Elt))
	case *ast.MapType:
		return "map[" + typeExprString(t.Key) + "]" + typeExprString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.ChanType:
		return "chan " + typeExprString(t.Value)
	case *ast.FuncType:
		return "func" + formatFieldList(t.Params)
	default:
		return "any"
	}
}

func normalizeSig(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}
