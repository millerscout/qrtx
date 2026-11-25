package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const wiringGateGo = `package gate

import "os"

type Gate struct{ lease int }

func New(enabled bool) *Gate { return &Gate{} }

func Init(g *Gate) {}

type Settings struct{ Enabled bool }

func SettingsFromEnv() Settings {
	return Settings{Enabled: os.Getenv("APP_LLM_GATE_ENABLED") != "0"}
}

func (g *Gate) Acquire() func() { return func() {} }
`

// wiringRepo writes a gate package built two ways (server New+Init, worker
// SettingsFromEnv) plus a streaming caller that releases on body close.
func wiringRepo(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"internal/gate/gate.go":     wiringGateGo,
		"internal/config/config.go": "package config\n\nimport \"os\"\n\nvar Queue = os.Getenv(\"APP_LLM_GATE_QUEUE\")\n",
		"internal/server/http.go":   "package server\n\nimport \"example.com/x/internal/gate\"\n\nfunc start() {\n\tgate.Init(gate.New(true))\n}\n",
		"internal/worker/worker.go": "package worker\n\nimport \"example.com/x/internal/gate\"\n\nfunc run() {\n\t_ = gate.SettingsFromEnv()\n}\n",
		"internal/stream/stream.go": "package stream\n\nimport \"example.com/x/internal/gate\"\n\nfunc serve(g *gate.Gate, body *Body) {\n\trel := g.Acquire()\n\tbody.onClose = rel\n}\n",
		"internal/ide/ide.go":       "package ide\n\nimport \"example.com/x/internal/gate\"\n\nfunc post(g *gate.Gate) {\n\tdefer g.Acquire()()\n}\n",
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

func wiringManifest(t *testing.T, decisions, review string) (string, string) {
	t.Helper()
	root, slug := reviewManifest(t, "- A held slot is reclaimed after the lease.", "- S1 WHEN a holder never releases THEN the slot frees after the lease\n- S2 WHEN a flag is unknown THEN exit 2", decisions, "- Lease too short → default 15m.\n\nDeploy: ship the server and worker together.\n\nRollback: set `APP_LLM_GATE_LEASE_SEC=0`.", review)
	wiringRepo(t, root)
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := replaceSection(string(raw), "Observed", "- `internal/gate/gate.go` reads settings in `SettingsFromEnv`.")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, slug
}

const wiringDecision = "- D1 Read `APP_LLM_GATE_LEASE_SEC` in `SettingsFromEnv` and reclaim after it. Rejected: a fixed lease because operators need to tune it."

func hintOf(p ReviewPacket, kind string) *ReviewHint {
	for i, h := range p.Hints {
		if h.Kind == kind {
			return &p.Hints[i]
		}
	}
	return nil
}

func TestSettingUnwiredHint(t *testing.T) {
	root, slug := wiringManifest(t, wiringDecision, "- None: x")
	p, err := BuildReviewPacket(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	h := hintOf(p, "setting-unwired")
	if h == nil {
		t.Fatalf("no setting-unwired hint in %+v", p.Hints)
	}
	for _, want := range []string{"APP_LLM_GATE_LEASE_SEC", "internal/config", "internal/gate", "internal/server/http.go", "Init()", "SettingsFromEnv()"} {
		if !strings.Contains(h.Text, want) {
			t.Fatalf("hint lacks %q: %s", want, h.Text)
		}
	}
	if strings.Join(h.Claims, ",") != "D1" {
		t.Fatalf("claims %v", h.Claims)
	}
	if err := ValidateChangeManifest(root, slug); err == nil || !strings.Contains(err.Error(), "review hint not answered") {
		t.Fatalf("want unanswered hint, got %v", err)
	}
	root, slug = wiringManifest(t, wiringDecision, "- R1 D1 vs internal/server/http.go New(true) → D2 server passes the lease into New")
	if err := ValidateChangeManifest(root, slug); err != nil {
		t.Fatal(err)
	}
}

func TestSettingUnwiredSkipsExistingSetting(t *testing.T) {
	root, slug := wiringManifest(t, "- D1 Keep `APP_LLM_GATE_ENABLED` as the switch. Rejected: a new flag because one exists.", "- None: x")
	p, err := BuildReviewPacket(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	if h := hintOf(p, "setting-unwired"); h != nil && strings.Contains(h.Text, "APP_LLM_GATE_ENABLED") {
		t.Fatalf("existing setting flagged: %s", h.Text)
	}
}

func TestAllCallersHint(t *testing.T) {
	decision := "- D1 Every call site releases `Gate.Acquire` with defer, as in `internal/worker/worker.go`. Rejected: manual release because it leaks on panic."
	root, slug := wiringManifest(t, decision, "- None: x")
	p, err := BuildReviewPacket(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	h := hintOf(p, "all-callers")
	if h == nil {
		t.Fatalf("no all-callers hint in %+v", p.Hints)
	}
	if !strings.Contains(h.Text, "internal/stream/stream.go") || !strings.Contains(h.Text, "internal/ide/ide.go") {
		t.Fatalf("hint lacks call sites: %s", h.Text)
	}
	if !strings.Contains(h.Text, "internal/worker/worker.go does not call it") {
		t.Fatalf("hint should flag the non-caller: %s", h.Text)
	}
	found := false
	for _, c := range p.Callers {
		if c.Path == "internal/stream/stream.go" && strings.Contains(c.Snippet, "body.onClose = rel") {
			found = true
		}
	}
	if !found {
		t.Fatalf("callers lack the body-close site: %+v", p.Callers)
	}
	if out := FormatReviewPacket(p); !strings.Contains(out, "## Every call site") {
		t.Fatalf("packet lacks call-site section\n%s", out)
	}
}
