package sdd

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

const graphFileName = ".sdd-graph.json"

// DependencyGraph is the project contract dependency artifact.
type DependencyGraph struct {
	Packages []string    `json:"packages"`
	Edges    []GraphEdge `json:"edges"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// FindContractPackages lists repo-relative dirs containing contract.sdd.md.
// It skips dot-directories, vendor and node_modules.
func FindContractPackages(repoRoot string) ([]string, error) {
	var out []string
	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path == repoRoot {
				return nil
			}
			name := info.Name()
			// Skip hidden dirs (.git, .claude/worktrees, .theia, ...): they hold
			// tool state or stale copies of the tree, not project contracts.
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			switch name {
			case "vendor", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() != contractFileName {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

// BuildGraph collects import edges among contract packages.
func BuildGraph(repoRoot string) (DependencyGraph, error) {
	pkgs, err := FindContractPackages(repoRoot)
	if err != nil {
		return DependencyGraph{}, err
	}
	set := map[string]bool{}
	for _, p := range pkgs {
		set[p] = true
	}
	module := goModulePath(repoRoot)
	var edges []GraphEdge
	for _, pkg := range pkgs {
		imports, err := goImportsInPackage(filepath.Join(repoRoot, pkg))
		if err != nil {
			return DependencyGraph{}, fmt.Errorf("%s: %w", pkg, err)
		}
		for _, imp := range imports {
			target := moduleImportToPkg(repoRoot, module, imp)
			if target != "" && set[target] && target != pkg {
				edges = append(edges, GraphEdge{From: pkg, To: target})
			}
		}
	}
	return DependencyGraph{Packages: pkgs, Edges: edges}, nil
}

// goModulePath reads the module path from repoRoot/go.mod, or "" when absent.
func goModulePath(repoRoot string) string {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], "\"`")
		}
	}
	return ""
}

func moduleImportToPkg(repoRoot, module, imp string) string {
	imp = strings.TrimSpace(imp)
	if module == "" || !strings.HasPrefix(imp, module+"/") {
		return ""
	}
	rel := strings.TrimPrefix(imp, module+"/")
	dir := filepath.Join(repoRoot, rel)
	if _, err := os.Stat(filepath.Join(dir, contractFileName)); err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

func goImportsInPackage(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	seen := map[string]bool{}
	var out []string
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".go") || strings.HasSuffix(ent.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, ent.Name()), nil, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		for _, im := range f.Imports {
			p := strings.Trim(im.Path.Value, `"`)
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}

// WriteGraph writes .sdd-graph.json at repo root.
func WriteGraph(repoRoot string) (string, error) {
	g, err := BuildGraph(repoRoot)
	if err != nil {
		return "", err
	}
	path := filepath.Join(repoRoot, graphFileName)
	raw, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
