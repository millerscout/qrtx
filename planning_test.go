package qrtx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanningMapDogfood(t *testing.T) {
	root, err := RepoRoot()
	if err != nil {
		t.Skip(err)
	}
	plan, err := PlanningMap(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil {
		t.Fatal("expected planning map")
	}
	if plan["package_count"].(int) < 1 {
		t.Fatalf("package_count=%v", plan["package_count"])
	}
	pkgs, ok := plan["packages"].([]string)
	if !ok || len(pkgs) == 0 {
		t.Fatalf("packages=%T %v", plan["packages"], plan["packages"])
	}
}

func TestPlanningMapEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	plan, err := PlanningMap(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan != nil {
		t.Fatalf("want nil plan, got %v", plan)
	}
	_ = os.MkdirAll(filepath.Join(dir, "internal", "x"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "internal", "x", contractFileName), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err = PlanningMap(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || plan["package_count"].(int) != 1 {
		t.Fatalf("plan=%v", plan)
	}
}
