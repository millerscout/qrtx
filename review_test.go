package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const reviewGoFile = `package hybrid

type SessionState struct {
	RestoreTo string ` + "`json:\"restore_to,omitempty\"`" + `
	Sessions  []string ` + "`json:\"sessions\"`" + `
}
`

// reviewManifest writes a filled manifest plus cli/hybrid/sessions.go into a temp repo.
func reviewManifest(t *testing.T, acceptance, scenarios, decisions, risks, review string) (string, string) {
	t.Helper()
	root := t.TempDir()
	slug := "review-check"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cli", "hybrid"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cli", "hybrid", "sessions.go"), []byte(reviewGoFile), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(changeFilePath(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "Add --json to sessions list.", 1)
	text = replaceSection(text, "Acceptance", acceptance)
	text = replaceSection(text, "Scenarios", scenarios)
	text = replaceSection(text, "Decisions", decisions)
	text = replaceSection(text, "Risks", risks)
	text = replaceSection(text, "Observed", "- `SessionState` in `cli/hybrid/sessions.go` holds the registry.")
	text = replaceSection(text, "Assumptions", "- JSON reuses the SessionState tags.")
	text = replaceSection(text, "Tasks", "- [ ] 1.1 Add the list command; verify: go test")
	text = replaceSection(text, "Validate", "- [ ] V1.1 `go test ./cli/hybrid` covers S1,S2")
	text = replaceSection(text, "Review", review)
	text = removePlaceholderSection(text, "Requirements") // optional; fixtures leave it out
	if err := os.WriteFile(changeFilePath(root, slug), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, slug
}

const (
	revAcceptance = "- `--json` always prints `restore_to`, as `\"\"` when unset."
	revScenarios  = "- S1 WHEN no session is live THEN stdout is `{\"restore_to\":\"\",\"sessions\":[]}`\n- S2 WHEN a flag is unknown THEN exit 2"
	revDecisions  = "- D1 Marshal SessionState directly. Rejected: a view struct because it duplicates fields."
	revRisks      = "- Contract burden → golden test.\n\nRollback: n/a (read-only command)"
)

func hintKinds(p ReviewPacket) []string {
	var out []string
	for _, h := range p.Hints {
		out = append(out, h.Kind)
	}
	return out
}

func TestReviewPacketOmitemptyHint(t *testing.T) {
	root, slug := reviewManifest(t, revAcceptance, revScenarios, revDecisions, revRisks, "- None: x")
	p, err := BuildReviewPacket(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, c := range p.Claims {
		ids[c.ID] = true
	}
	for _, want := range []string{"A1", "S1", "S2", "D1", "M1", "K1", "RB"} {
		if !ids[want] {
			t.Fatalf("missing claim %s in %+v", want, p.Claims)
		}
	}
	if len(p.Code) != 1 || p.Code[0].Path != "cli/hybrid/sessions.go" || !strings.Contains(p.Code[0].Snippet, "omitempty") {
		t.Fatalf("code %+v", p.Code)
	}
	found := false
	for _, h := range p.Hints {
		if h.Kind == "omitempty-vs-presence" && strings.Contains(h.Text, "restore_to") {
			found = true
		}
	}
	if !found {
		t.Fatalf("hints %v", hintKinds(p))
	}
	out := FormatReviewPacket(p)
	for _, want := range []string{"## Claims", "## Hints", "## Cited code", "R<n>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("packet lacks %q\n%s", want, out)
		}
	}
}

func TestReviewPacketRollbackAndRuledOutHints(t *testing.T) {
	acceptance := "- No session write is lost while the migration runs.\n- No waiter starves behind later arrivals."
	decisions := "- D1 User requests always go first. Rejected: FIFO because chat latency matters; background work may starve under load."
	risks := "- Long migration → progress log.\n\nRollback: restore the pre-migration backup of every project file."
	root, slug := reviewManifest(t, acceptance, revScenarios, decisions, risks, "- None: x")
	p, err := BuildReviewPacket(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	kinds := strings.Join(hintKinds(p), ",")
	if !strings.Contains(kinds, "rollback-vs-no-loss") || !strings.Contains(kinds, "ruled-out-vs-accepted") {
		t.Fatalf("hints %s", kinds)
	}
}

func TestReviewPacketNoHintsWhenConsistent(t *testing.T) {
	acceptance := "- `--json` prints the session list."
	root, slug := reviewManifest(t, acceptance, "- S1 WHEN a flag is unknown THEN exit 2\n- S2 WHEN no session is live THEN an empty list prints", "- None: no design choice.", revRisks, "- None: checked A1 against S1,S2")
	p, err := BuildReviewPacket(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Hints) != 0 {
		t.Fatalf("unexpected hints %+v", p.Hints)
	}
	if err := ValidateChangeManifest(root, slug); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReviewSection(t *testing.T) {
	cases := []struct {
		name, review, want string
	}{
		{"tbd", "- (TBD — x)", "Review still contains TBD"},
		{"unanswered hint", "- None: looked at everything", "review hint not answered"},
		{"no fix arrow", "- R1 A1 conflicts with the omitempty tag", "needs `→ <fix>`"},
		{"bad id", "- Finding: A1 vs code → D2", "must start with R<n>"},
		{"answered", "- R1 A1 vs cli/hybrid/sessions.go restore_to omitempty → D2 dedicated view struct", ""},
		{"answered not a conflict", "- R1 A1 → not a conflict because the CLI marshals a view struct", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, slug := reviewManifest(t, revAcceptance, revScenarios, revDecisions, revRisks, tc.review)
			err := ValidateChangeManifest(root, slug)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestSpecDepthReview(t *testing.T) {
	root, slug := reviewManifest(t, revAcceptance, revScenarios, revDecisions, revRisks, "- R1 A1 vs restore_to omitempty → D2 view struct\n- R2 D1 vs A1 → D1 changed")
	sc, err := ScoreChangeManifest(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	if !sc.Depth.Reviewed || sc.Depth.ReviewFindings != 2 || sc.Depth.ReviewHints != 1 {
		t.Fatalf("depth %+v", sc.Depth)
	}
}

const touchStoreGo = `package store

import (
	"os"
	"path/filepath"
)

type Repository struct{ dir string }

func NewRepository() *Repository {
	base := os.Getenv("MEM_STORE_DIR")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config", "acme", "memory")
	}
	return &Repository{dir: base}
}

func (r *Repository) Save(slug string) error { return nil }
`

// touchRepo adds a store package, a Python writer and two packages that construct the store.
func touchRepo(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"internal/store/store.go": touchStoreGo,
		"scripts/mem_tool.py":     "import os\nBASE = os.environ.get(\"MEM_STORE_DIR\", \"~/.config/acme/memory\")\n",
		"internal/api/api.go":     "package api\n\nimport \"example.com/x/internal/store\"\n\nvar repo = store.NewRepository()\n",
		"internal/cron/cron.go":   "package cron\n\nimport \"example.com/x/internal/store\"\n\nfunc run() {\n\tr := store.NewRepository()\n\t_ = r.Save(\"a\")\n}\n",
		"internal/other/other.go": "package other\n\nfunc New() {}\nfunc NewRepository() {}\n",
	}
	for p, body := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func touchManifest(t *testing.T, review string) (string, string) {
	t.Helper()
	root, slug := reviewManifest(t, "- Memory files move to the v2 format.", "- S1 WHEN the migration crashes THEN the v1 file is unchanged\n- S2 WHEN a flag is unknown THEN exit 2", "- D1 Take a mutex inside `Repository.Save` while files migrate to the v2 format. Rejected: flock because it is per-process.", "- Long migration → progress log.\n\nDeploy: ship the v2 reader before migrating.\n\nRollback: rename the v1 file back.", review)
	touchRepo(t, root)
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := replaceSection(string(raw), "Observed", "- `internal/store/store.go` keeps one JSON per project under `MEM_STORE_DIR`.")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, slug
}

func TestReviewPacketListsUncitedWritersAndCallers(t *testing.T) {
	root, slug := touchManifest(t, "- None: x")
	p, err := BuildReviewPacket(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	var writer, callers *ReviewHint
	for i, h := range p.Hints {
		switch {
		case h.Kind == "uncited-writer":
			writer = &p.Hints[i]
		case h.Kind == "uncited-callers" && strings.Contains(h.Text, "new Repository"):
			callers = &p.Hints[i]
		}
	}
	if writer == nil || !strings.Contains(writer.Text, "scripts/mem_tool.py") || !strings.Contains(strings.Join(writer.Claims, ","), "D1") {
		t.Fatalf("writer hint %+v in %+v", writer, p.Hints)
	}
	if callers == nil || !strings.Contains(callers.Text, "internal/api/api.go") || !strings.Contains(callers.Text, "internal/cron/cron.go") {
		t.Fatalf("callers hint %+v in %+v", callers, p.Hints)
	}
	for _, tch := range p.Touches {
		if tch.Path == "internal/other/other.go" {
			t.Fatalf("unqualified NewRepository in another package must not match: %+v", tch)
		}
	}
	out := FormatReviewPacket(p)
	if !strings.Contains(out, "## Other code using the same data or symbols") || !strings.Contains(out, "scripts/mem_tool.py:2 (uncited)") {
		t.Fatalf("packet\n%s", out)
	}
}

func TestValidateReviewRequiresUncitedWriterAnswer(t *testing.T) {
	root, slug := touchManifest(t, "- R1 S1 vs S2 → not a conflict because they test different inputs")
	err := ValidateChangeManifest(root, slug)
	if err == nil || !strings.Contains(err.Error(), "review hint not answered") {
		t.Fatalf("want unanswered hint, got %v", err)
	}
	root, slug = touchManifest(t, "- R1 D1 vs scripts/mem_tool.py and store.NewRepository callers → D2 flock on the file, shared by every process and instance")
	if err := ValidateChangeManifest(root, slug); err != nil {
		t.Fatal(err)
	}
}

func TestReviewRejectsDeferredPromise(t *testing.T) {
	acceptance := "- No session write is lost during the migration, including Python script saves."
	cases := []struct {
		name, review, nonGoals, want string
	}{
		{"defers acceptance", "- R1 A1 Python script saves after commit are lost → deferred to a follow-up change", "", "defers a case A1 still promises"},
		{"defers with non-goal", "- R1 A1 Python script saves after commit are lost → deferred to a follow-up change", "- Python script saves after commit are handled in a later change.", ""},
		{"real fix", "- R1 A1 Python script saves after commit are lost → D2 the script takes the same flock", "", ""},
		{"defers a decision only", "- R1 D1 the progress log format → later", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scenarios := "- S1 WHEN a flag is unknown THEN exit 2\n- S2 WHEN the store is empty THEN migrate exits 0"
			root, slug := reviewManifest(t, acceptance, scenarios, "- None: no design choice.", revRisks, tc.review)
			if tc.nonGoals != "" {
				path := changeFilePath(root, slug)
				raw, _ := os.ReadFile(path)
				if err := os.WriteFile(path, []byte(replaceSection(string(raw), "Non-goals", tc.nonGoals)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := ValidateChangeManifest(root, slug)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestSoftMitigationHint(t *testing.T) {
	acceptance := "- No session write is lost during the migration."
	risks := "- Old binaries write v1 after commit and their saves are lost → status warns when v1 is newer than v2.\n- Slow disk → progress log.\n\nDeploy: ship the reader first.\n\nRollback: n/a (migration is copy-only)"
	root, slug := reviewManifest(t, acceptance, "- S1 WHEN a flag is unknown THEN exit 2\n- S2 WHEN the store is empty THEN exit 0", "- None: no design choice.", risks, "- None: x")
	p, err := BuildReviewPacket(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	h := hintOf(p, "soft-mitigation-vs-promise")
	if h == nil || h.Claims[0] != "K1" || !strings.Contains(strings.Join(h.Claims, ","), "A1") {
		t.Fatalf("hint %+v in %+v", h, p.Hints)
	}
	for _, other := range p.Hints {
		if other.Kind == "soft-mitigation-vs-promise" && other.Claims[0] == "K2" {
			t.Fatalf("progress log on a non-loss risk flagged: %+v", other)
		}
	}
}
