package sdd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	querySymbolHits = func(string, string, int) []CodeHit { return nil }
	queryEmbeddingHits = func(string, string, int) []CodeHit { return nil }
	os.Exit(m.Run())
}

func TestGraphHitCount(t *testing.T) {
	prev := queryRelatedHits
	t.Cleanup(func() { queryRelatedHits = prev })
	called := false
	queryRelatedHits = func(project, query string, limit int) []CodeHit {
		called = true
		return []CodeHit{{Path: "internal/score/score.go", Line: 1, Source: "kuzu"}}
	}
	if n := GraphHitCount("demo", "a ferry board"); n != 0 || called {
		t.Fatalf("plain request hits %d called %v", n, called)
	}
	if n := GraphHitCount("demo", "fix bufferPoolForBytes"); n != 1 || !called {
		t.Fatalf("identifier hits %d called %v", n, called)
	}
}

func TestProposeScore(t *testing.T) {
	simple := proposeBand("Fix the comment in internal/codegraph/db.go", 1, false)
	if simple != "simple" {
		t.Fatalf("simple=%s", simple)
	}
	one := proposeBand("Related queries return nothing", 1, false)
	if one != "context" {
		t.Fatalf("context=%s", one)
	}
	two := proposeBand("Related queries return nothing", 2, false)
	if two != "goldfish" {
		t.Fatalf("goldfish=%s", two)
	}
}

func TestProposeHits(t *testing.T) {
	toks := queryTokens("write a python bufferPoolForBytes helper")
	if len(toks) != 1 || toks[0] != "bufferPoolForBytes" {
		t.Fatalf("tokens=%v", toks)
	}
	sentence := queryTokens("Score the request. Run a Goldfish check. Benchmark SDD.")
	if len(sentence) != 0 {
		t.Fatalf("sentence tokens=%v", sentence)
	}
	for _, w := range toks {
		if w == "write" || w == "python" {
			t.Fatalf("plain word kept: %s", w)
		}
	}

	var relatedCalls, embedCalls int
	prevR, prevE, prevS := queryRelatedHits, queryEmbeddingHits, querySymbolHits
	t.Cleanup(func() {
		queryRelatedHits = prevR
		queryEmbeddingHits = prevE
		querySymbolHits = prevS
	})
	queryRelatedHits = func(project, query string, limit int) []CodeHit {
		relatedCalls++
		if strings.Contains(query, "write") || strings.Contains(query, "python") {
			t.Fatalf("query included plain words: %s", query)
		}
		return []CodeHit{
			{Path: "a.go", Line: 1, Source: "kuzu"},
			{Path: "b.go", Line: 2, Source: "kuzu"},
			{Path: "c.go", Line: 3, Source: "kuzu"},
			{Path: "d.go", Line: 4, Source: "kuzu"},
		}
	}
	queryEmbeddingHits = func(project, query string, limit int) []CodeHit {
		embedCalls++
		return []CodeHit{{Path: "e.go", Line: 5, Source: "embedding"}}
	}

	hits := collectCodeHits("context", "demo", "write a python bufferPoolForBytes helper")
	if len(hits) != 3 {
		t.Fatalf("hits=%d", len(hits))
	}
	if hits[0].Source != "kuzu" || hits[2].Path != "c.go" {
		t.Fatalf("hits=%+v", hits)
	}
	if relatedCalls != 1 || embedCalls != 0 {
		t.Fatalf("related=%d embed=%d", relatedCalls, embedCalls)
	}

	relatedCalls, embedCalls = 0, 0
	if got := collectCodeHits("simple", "demo", "write a python bufferPoolForBytes helper"); got != nil {
		t.Fatalf("simple hits=%v", got)
	}
	if relatedCalls != 0 || embedCalls != 0 {
		t.Fatalf("simple called stores related=%d embed=%d", relatedCalls, embedCalls)
	}

	relatedCalls, embedCalls = 0, 0
	if got := collectCodeHits("context", "demo", "Score the request. Run a Goldfish check. Benchmark SDD."); got != nil {
		t.Fatalf("sentence hits=%v", got)
	}
	if relatedCalls != 0 || embedCalls != 0 {
		t.Fatalf("sentence called stores related=%d embed=%d", relatedCalls, embedCalls)
	}

	queryRelatedHits = func(string, string, int) []CodeHit {
		return []CodeHit{{Path: "a.go", Line: 1, Source: "kuzu"}}
	}
	queryEmbeddingHits = func(string, string, int) []CodeHit {
		embedCalls++
		return []CodeHit{{Path: "e.go", Line: 9, Source: "embedding"}}
	}
	mixed := collectCodeHits("goldfish", "demo", "bufferPoolForBytes")
	if len(mixed) != 2 || mixed[1].Source != "embedding" || mixed[1].Line != 9 {
		t.Fatalf("mixed=%+v", mixed)
	}

	var symbolCalls int
	querySymbolHits = func(string, string, int) []CodeHit {
		symbolCalls++
		return []CodeHit{{Path: "internal/score/score.go", Line: 187, Source: "symbol"}}
	}
	queryRelatedHits = func(string, string, int) []CodeHit {
		relatedCalls++
		return []CodeHit{{Path: "scripts/bench.py", Line: 1, Source: "kuzu"}}
	}
	embedCalls = 0
	relatedCalls = 0
	ranked := collectCodeHits("context", "demo", "call proposeDomain from the plan")
	if len(ranked) != 3 || ranked[0].Source != "symbol" || ranked[0].Path != "internal/score/score.go" {
		t.Fatalf("ranked=%+v", ranked)
	}
	if ranked[1].Source != "kuzu" || ranked[2].Source != "embedding" {
		t.Fatalf("ranked fill=%+v", ranked)
	}
	if symbolCalls != 1 {
		t.Fatalf("symbol calls=%d", symbolCalls)
	}
}

func TestProposeDomain(t *testing.T) {
	if got := proposeDomain("Fix the comment in internal/codegraph/db.go", nil, ""); got != "internal/codegraph" {
		t.Fatalf("request domain=%s", got)
	}
	if got := proposeDomain("Related queries return nothing", []string{"internal/sdd/propose_context.go"}, ""); got != "internal/sdd" {
		t.Fatalf("hit domain=%s", got)
	}
	if got := proposeDomain("write a python helper", nil, ""); got != "(none)" {
		t.Fatalf("empty domain=%s", got)
	}
	observed := "see `internal/sdd/propose_context.go` first"
	if got := proposeDomain("Related queries return nothing", nil, observed); got != "internal/sdd" {
		t.Fatalf("observed domain=%s", got)
	}
	if got := proposeDomain("Related queries return nothing", nil, ""); got != "(none)" {
		t.Fatalf("empty observed=%s", got)
	}
	if got := proposeDomain("Related queries return nothing", []string{"scripts/lab_code_rag.py"}, observed); got != "internal/sdd" {
		t.Fatalf("observed should win over a hit, got %s", got)
	}
	if got := proposeDomain("Fix the comment in internal/sdd/propose_phases.go", nil, "see internal/codegraph/db.go"); got != "internal/sdd" {
		t.Fatalf("request should win over observed, got %s", got)
	}
	if inDomain("internal/sdd", "internal/sdd/finish.go") != true {
		t.Fatal("finish.go should be in domain")
	}
	if inDomain("internal/sdd", "homepage/agent/app.js") {
		t.Fatal("homepage should be outside internal/sdd")
	}
}

func TestPhasesReady(t *testing.T) {
	needs := "### Phase One\n- [ ] 1.1 needs user: the command\n### Phase Two\n- [ ] 2.1 needs user: the place\n"
	if phasesReady(needs) {
		t.Fatal("needs user should not be ready")
	}
	ready := "### Phase One\n- [ ] 1.1 Edit `internal/sdd/propose_context.go`\n### Phase Two\n- [ ] 2.1 Edit `internal/sdd/propose_context.go`\n"
	corpus := "internal/sdd/propose_context.go"
	if !decisionGrounded(corpus, "- [ ] 1.1 Edit `internal/sdd/propose_context.go`") {
		t.Fatal("path should be grounded")
	}
	if !phasesReady(ready) {
		t.Fatal("replaced fixture should be ready")
	}
	mention := "### Phase One\n- [x] 1.1 A checked task mentions needs user: and runs `./internal/sdd/propose_phases.go`\n"
	if !phasesReady(mention) {
		t.Fatal("mentioning the prefix should stay ready")
	}
	phaseCorpus := "internal/sdd/propose_phases.go"
	if !decisionGrounded(phaseCorpus, "- [x] 1.1 A checked task mentions needs user: and runs `./internal/sdd/propose_phases.go`") {
		t.Fatal("dot-slash path should be grounded")
	}
	four := "### Phase One\n- [ ] 1.1 a\n- [ ] 1.2 b\n- [ ] 1.3 c\n- [ ] 1.4 d\n"
	if !phasesReady(four) {
		t.Fatal("four tasks should be ready")
	}
	if decisionGrounded(corpus, "- [ ] 1.1 calls `ZzMissingToken`") {
		t.Fatal("invented identifier should not be grounded")
	}
	if !decisionGrounded("", "- [x] 1.1 A path contains `/`") {
		t.Fatal("a lone slash should not be a path")
	}
	if !decisionGrounded("", "- [ ] 2.2 Add `proposeDomain(request string, hitPaths []string) string`") {
		t.Fatal("hitPaths in the signature should be grounded")
	}
	if !decisionGrounded("", "- [ ] 4.1 Add `goldfishPassed(goal, testGap, ready string, phasesOK bool) bool`") {
		t.Fatal("testGap in the signature should be grounded")
	}
	if !tasksGrounded("", "- [ ] 3.2 Add `openPhase`\n- [ ] 3.3 Print `openPhase`\n") {
		t.Fatal("openPhase from an earlier Add should be grounded")
	}
	if !decisionGrounded("", "- [ ] 4.1 Read `change.sdd.md`") {
		t.Fatal("manifest filename should be grounded")
	}
	if !decisionGrounded("internal/sdd", "- [ ] 2.2 `homepage/agent/app.js` is not in domain `internal/sdd`") {
		t.Fatal("not-in-domain path should be grounded")
	}
	if !decisionGrounded("", "- [ ] 5.2 false for `homepage/agent/app.js`") {
		t.Fatal("false-for path should be grounded")
	}
	if !decisionGrounded("", "- [ ] 5.1 Add `scripts/fixtures/sdd_plan_bench/scenarios.jsonl`") {
		t.Fatal("Add path should be grounded")
	}
	if !decisionGrounded("~/.cursor/skills/", "- [ ] 3.1 Mirror `~/.cursor/skills/sdd-propose/`") {
		t.Fatal("parent directory should ground the child path")
	}
	if decisionGrounded("internal/sdd/propose_phases.go", "- [ ] 1.1 Edit `internal/other/new.go`") {
		t.Fatal("internal/ alone should not ground a new path")
	}
	if decisionGrounded("~/.cursor/skills/", "- [ ] 3.1 Edit `~/.cursor/skills/sdd-propose/sub/file.md`") {
		t.Fatal("a deeper ancestor should not ground the path")
	}
	open := "### Phase One\n- [x] 1.1 done\n### Phase Two\n- [ ] 2.1 next\n"
	if got := openPhase(open); got != "### Phase Two" {
		t.Fatalf("open=%q", got)
	}
	if openPhase("- [ ] 1.1 no heading\n") != "" {
		t.Fatal("missing heading should be empty")
	}

	root := t.TempDir()
	slug := "open-phase-status"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(raw), "- [ ] 1.1 First implementation task", "### Phase One\n- [ ] 1.1 Still open\n### Phase Two\n- [ ] 2.1 Later", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := SummarizeChange(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	if sum.OpenPhase != "### Phase One" {
		t.Fatalf("open_phase=%q tasks_done=%d tasks_total=%d", sum.OpenPhase, sum.TasksDone, sum.TasksTotal)
	}
	if sum.TasksTotal < 1 {
		t.Fatal("tasks_total dropped")
	}
}

func TestGoldfishPassed(t *testing.T) {
	if goldfishPassed("", "### Phase One gap", "yes", true) {
		t.Fatal("empty goal")
	}
	if goldfishPassed("ship it", "### Phase One gap", "no", true) {
		t.Fatal("ready no")
	}
	if goldfishPassed("ship it", "### Phase One gap", "yes", false) {
		t.Fatal("phasesOK false")
	}
	if goldfishPassed("ship it", "caching", "yes", true) {
		t.Fatal("test gap without a phase heading")
	}
	if !goldfishPassed("ship it", "### Phase One lacks a test", "yes", true) {
		t.Fatal("expected pass")
	}
}

func TestProposeDiagnostic(t *testing.T) {
	root := t.TempDir()
	var out, errOut bytes.Buffer
	if err := NewChange(root, "drop-words"); err != nil {
		t.Fatal(err)
	}
	writeProposeFixture(t, root, "drop-words", "The page still drops tic tac toe", "- One check.", "### Phase One\n- [x] 1.1 Done\n")
	err := FinishPropose(root, "drop-words", &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "diagnostic:") {
		t.Fatalf("err=%v out=%s", err, out.String())
	}
	if strings.Contains(out.String(), `"score"`) {
		t.Fatalf("score JSON printed: %s", out.String())
	}

	path := changeFilePath(root, "drop-words")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(raw), "(TBD — cite a path that exists in the repo. Separate from assumptions.)", "`substituteCommandArguments` returned only the loader sentence", 1)
	if !strings.Contains(text, "substituteCommandArguments") {
		t.Fatal("observed placeholder missing")
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if err := FinishPropose(root, "drop-words", &out, &errOut); err != nil {
		t.Fatalf("ready: %v stderr=%s out=%s", err, errOut.String(), out.String())
	}
	if !strings.Contains(out.String(), `"score"`) {
		t.Fatalf("score missing: %s", out.String())
	}

	if err := NewChange(root, "tic-tac-toe-feature"); err != nil {
		t.Fatal(err)
	}
	writeProposeFixture(t, root, "tic-tac-toe-feature", "tic tac toe", "- One check.", "### Phase One\n- [x] 1.1 Done\n")
	out.Reset()
	errOut.Reset()
	if err := FinishPropose(root, "tic-tac-toe-feature", &out, &errOut); err != nil {
		t.Fatalf("feature: %v stderr=%s out=%s", err, errOut.String(), out.String())
	}
}
