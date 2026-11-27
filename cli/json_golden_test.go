package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/millerscout/qrtx"
)

// The JSON that agents and scripts read is a contract. These goldens pin
// it; QRTX_UPDATE_GOLDEN=1 rewrites them on purpose.

var (
	goldenTimeRE   = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
	goldenIDRE     = regexp.MustCompile(`sdd-[a-z2-7]{6,}`)
	goldenRefRE    = regexp.MustCompile(`"client_ref": "[0-9a-f]{12}"`)
	goldenPIDRE    = regexp.MustCompile(`"pid": \d+`)
	goldenPPIDRE   = regexp.MustCompile(`<ROOT>:\d+`)
	goldenDurRE    = regexp.MustCompile(`"detail": "(capabilities,[^"]*|\d+ issues|sdd-[^"]*)"`)
	goldenTitleRE  = regexp.MustCompile(`qrtx tracker check <TIME>`)
	goldenUpdateRE = regexp.MustCompile(`"updated_at": "[^"]*"`)
)

// normalizeGolden makes JSON stable: arrays of objects are sorted by their
// encoding, keys come out sorted, and volatile values become placeholders.
func normalizeGolden(t *testing.T, raw []byte, roots ...string) []byte {
	t.Helper()
	text := string(raw)
	for _, r := range roots {
		forms := []string{r}
		if real, err := filepath.EvalSymlinks(r); err == nil && real != r {
			forms = append(forms, real)
		}
		for _, f := range append([]string{}, forms...) {
			esc, _ := json.Marshal(f)
			forms = append(forms, strings.Trim(string(esc), `"`))
		}
		sort.Slice(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
		for _, f := range forms {
			text = strings.ReplaceAll(text, f, "<ROOT>")
		}
	}
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	// Placeholders first (on an indented encoding), then sort arrays, so
	// random ids never decide the order.
	s := encodeGolden(v)
	s = goldenTimeRE.ReplaceAllString(s, "<TIME>")
	s = goldenIDRE.ReplaceAllString(s, "<ID>")
	s = goldenRefRE.ReplaceAllString(s, `"client_ref": "<REF>"`)
	s = goldenPIDRE.ReplaceAllString(s, `"pid": "<PID>"`)
	s = goldenPPIDRE.ReplaceAllString(s, "<ROOT>:<PID>")
	s = goldenDurRE.ReplaceAllString(s, `"detail": "<DETAIL>"`)
	s = goldenTitleRE.ReplaceAllString(s, "qrtx tracker check <TIME>")
	s = goldenUpdateRE.ReplaceAllString(s, `"updated_at": "<TIME>"`)
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("normalized JSON: %v\n%s", err, s)
	}
	s = encodeGolden(sortArrays(v))
	return []byte(s + "\n")
}

func encodeGolden(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return strings.TrimRight(buf.String(), "\n")
}

func sortArrays(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = sortArrays(e)
		}
		return x
	case []any:
		for i := range x {
			x[i] = sortArrays(x[i])
		}
		sort.SliceStable(x, func(i, j int) bool {
			a, _ := json.Marshal(x[i])
			b, _ := json.Marshal(x[j])
			return string(a) < string(b)
		})
		return x
	}
	return v
}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("..", "testdata", "golden", name+".json")
	if os.Getenv("QRTX_UPDATE_GOLDEN") == "1" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with QRTX_UPDATE_GOLDEN=1): %v", path, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("%s changed (QRTX_UPDATE_GOLDEN=1 accepts it on purpose)\n--- want\n%s\n--- got\n%s", path, want, got)
	}
}

func TestJSONGolden(t *testing.T) {
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	t.Setenv("QRTX_SESSION", "")
	mustRun := func(args ...string) string {
		t.Helper()
		out, errOut, err := run(t, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut)
		}
		return out
	}
	// Tracker: two issues in the file backend.
	mustRun("tracker", "add", "Dark-mode toggle", "--description", "Settings page")
	mustRun("tracker", "add", "Export to CSV")
	checkGolden(t, "tracker_list", normalizeGolden(t, []byte(mustRun("tracker", "list", "--json")), root))
	checkGolden(t, "tracker_check", normalizeGolden(t, []byte(mustRun("tracker", "check", "--json")), root))

	// propose-context: the backlog key.
	mustRun("change", "new", "demo")
	var pc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(mustRun("change", "propose-context", "--change", "demo", "--json")), &pc); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "propose_backlog", normalizeGolden(t, pc["backlog"], root))

	// Sync: two sessions, a claim, B's inbox, status and one event line.
	mustRun("sync", "heartbeat", "--session", "A", "--quiet")
	mustRun("sync", "heartbeat", "--session", "B", "--quiet")
	mustRun("sync", "claim", "demo/1.1", "--session", "A")
	checkGolden(t, "sync_inbox", normalizeGolden(t, []byte(mustRun("sync", "inbox", "--json", "--session", "B")), root))
	checkGolden(t, "sync_status", normalizeGolden(t, []byte(mustRun("sync", "status", "--json")), root))
	events, err := os.ReadFile(filepath.Join(root, ".sdd", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(string(events), "\n", 2)[0]
	checkGolden(t, "events_line", normalizeGolden(t, []byte(first), root))
}
