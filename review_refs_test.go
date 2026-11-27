package qrtx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hintsOfKind(p ReviewPacket, kind string) []ReviewHint {
	var out []ReviewHint
	for _, h := range p.Hints {
		if h.Kind == kind {
			out = append(out, h)
		}
	}
	return out
}

func hintMentions(hs []ReviewHint, subs ...string) bool {
	for _, h := range hs {
		ok := true
		for _, s := range subs {
			if !strings.Contains(h.Text, s) {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// The rbac-route-gates defects, reduced to a TS + SQL fixture.
func TestRefHintsRouteGatesFixture(t *testing.T) {
	p, err := BuildReviewPacket("testdata/refs_rbac", "route-gates")
	if err != nil {
		t.Fatal(err)
	}
	if !hintMentions(hintsOfKind(p, "used-before-created"), "`records.read`", "task 3.1", "task 1.1") {
		t.Errorf("want used-before-created for records.read (1.1 before 3.1); hints: %+v", p.Hints)
	}
	if !hintMentions(hintsOfKind(p, "existing-siblings"), "`records.read`", "`records.view.any`", "`records.view.own`") {
		t.Errorf("want existing-siblings listing records.view.*; hints: %+v", p.Hints)
	}
	if !hintMentions(hintsOfKind(p, "unknown-ref"), "`custom_field.manage.*`", "`custom_field.manage.deal`") {
		t.Errorf("want unknown-ref for the wildcard code; hints: %+v", p.Hints)
	}
	if !hintMentions(hintsOfKind(p, "uncovered-sites"), "`user_has_permission`", "src/app/api/v1/merges/[id]/apply/route.ts", "searches only src/app/api/admin") {
		t.Errorf("want uncovered-sites naming the merges route outside the grep scope; hints: %+v", p.Hints)
	}
	for _, h := range p.Hints {
		for _, quiet := range []string{"`task.skip`", "`company.add`", "`records.manage`", "tests/rbac/helpers.ts", "`withAuthAndPermission`"} {
			if strings.Contains(h.Text, quiet) {
				t.Errorf("hint should not mention %s: %s", quiet, h.Text)
			}
		}
	}
	// Each new hint must be answerable: it names a claim or task id.
	for _, h := range p.Hints {
		if len(h.Claims) == 0 {
			t.Errorf("hint without ids: %+v", h)
		}
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTreeSitterDefsAcrossLanguages(t *testing.T) {
	if !treeSitterEnabled {
		t.Skip("needs tree-sitter (cgo)")
	}
	srcs := []sourceFile{
		{path: "src/store/cache.ts", lines: strings.Split("export class SessionCache {\n  lock(key: string): void {}\n}\nexport function getCache(): SessionCache { return new SessionCache() }\nexport const makeKey = (a: string) => a\n", "\n")},
		{path: "app/store.py", lines: strings.Split("class Store:\n    def save(self, k):\n        pass\n\ndef open_store():\n    return Store()\n", "\n")},
		{path: "src/lib.rs", lines: strings.Split("pub struct Pool {}\nimpl Pool {\n    pub fn acquire(&self) {}\n}\npub fn new_pool() -> Pool { Pool {} }\n", "\n")},
	}
	defs := map[string][]codeDef{}
	treeSitterDefs(srcs, defs, map[string]bool{"SessionCache": true, "lock": true, "getCache": true, "makeKey": true, "Store": true, "save": true, "open_store": true, "Pool": true, "acquire": true, "new_pool": true})
	want := []struct {
		name, recv, path string
		isType           bool
	}{
		{"SessionCache", "", "src/store/cache.ts", true},
		{"lock", "SessionCache", "src/store/cache.ts", false},
		{"getCache", "", "src/store/cache.ts", false},
		{"makeKey", "", "src/store/cache.ts", false},
		{"Store", "", "app/store.py", true},
		{"save", "Store", "app/store.py", false},
		{"open_store", "", "app/store.py", false},
		{"Pool", "", "src/lib.rs", true},
		{"acquire", "Pool", "src/lib.rs", false},
		{"new_pool", "", "src/lib.rs", false},
	}
	for _, w := range want {
		found := false
		for _, d := range defs[w.name] {
			if d.path == w.path && d.recv == w.recv && d.isType == w.isType {
				found = true
			}
		}
		if !found {
			t.Errorf("missing def %+v; have %+v", w, defs[w.name])
		}
	}
}

// uncited-callers, which used to see only Go, fires on a TypeScript class.
func TestUncitedCallersTypeScript(t *testing.T) {
	if !treeSitterEnabled {
		t.Skip("needs tree-sitter (cgo)")
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"tsconfig.json":      `{"compilerOptions": {"baseUrl": ".", "paths": {"@/*": ["./src/*"]}}}`,
		"src/store/cache.ts": "export class SessionCache {\n  lock(key: string): void {}\n}\n",
		"src/api/a.ts":       "import { SessionCache } from '@/store/cache'\nconst c = new SessionCache()\n",
		"src/jobs/b.ts":      "import { SessionCache } from '../store/cache'\nexport const d = new SessionCache()\n",
		"src/other/c.ts":     "class SessionCache {}\nconst e = new SessionCache()\n",
		"sdd/changes/cache-lock/change.sdd.md": "```change\nname: cache-lock\nstatus: planning\npackages: []\n```\n\n## Why\n\nx\n\n## Acceptance\n\n- A1 no double write.\n\n" +
			"## Decisions\n\n- D1 `SessionCache` keeps a per-instance lock in `src/store/cache.ts`, so writes are serialized.\n",
	})
	p, err := BuildReviewPacket(root, "cache-lock")
	if err != nil {
		t.Fatal(err)
	}
	hs := hintsOfKind(p, "uncited-callers")
	if !hintMentions(hs, "new SessionCache", "src/api/a.ts", "src/jobs/b.ts") {
		t.Fatalf("want uncited-callers for the TS class; hints %+v touches %+v", p.Hints, p.Touches)
	}
	if hintMentions(hs, "src/other/c.ts") {
		t.Errorf("a file that does not import the module must not count: %+v", hs)
	}
}
