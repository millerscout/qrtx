package qrtx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PlanningMap summarizes contract packages and graph edges for UI context.
func PlanningMap(repoRoot string, refresh bool) (map[string]any, error) {
	pkgs, err := FindContractPackages(repoRoot)
	if err != nil {
		return nil, err
	}
	sort.Strings(pkgs)
	if len(pkgs) == 0 {
		return nil, nil
	}
	if refresh {
		if _, err := WriteGraph(repoRoot); err != nil {
			return nil, err
		}
	}
	g, err := loadGraph(repoRoot)
	if err != nil {
		g, err = BuildGraph(repoRoot)
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"graph_path":    graphFileName,
		"packages":      pkgs,
		"package_count": len(pkgs),
		"edge_count":    len(g.Edges),
		"focus_package": "",
	}, nil
}

func loadGraph(repoRoot string) (DependencyGraph, error) {
	path := filepath.Join(repoRoot, graphFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return DependencyGraph{}, err
	}
	var g DependencyGraph
	if err := json.Unmarshal(raw, &g); err != nil {
		return DependencyGraph{}, err
	}
	return g, nil
}

// MergePlanningIntoStatus adds planning to an existing sdd status map.
func MergePlanningIntoStatus(m map[string]any, repoRoot string, refresh bool, focusPackage string) error {
	if m == nil {
		return nil
	}
	plan, err := PlanningMap(repoRoot, refresh)
	if err != nil {
		return err
	}
	if plan == nil {
		return nil
	}
	if focus := strings.TrimSpace(focusPackage); focus != "" {
		plan["focus_package"] = focus
	}
	m["planning"] = plan
	return nil
}
