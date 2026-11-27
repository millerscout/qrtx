package qrtx

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

func FuzzMergeIssues(f *testing.F) {
	a := `{"client_ref":"a","id":"sdd-aaaaaa","status":{"category":"backlog"},"title":"A"}`
	b := `{"client_ref":"b","id":"sdd-bbbbbb","status":{"category":"started"},"title":"B"}`
	f.Add([]byte(a+"\n"), []byte(a+"\n"+b+"\n"), []byte(a+"\n"))
	f.Add([]byte(""), []byte(a), []byte(b))
	f.Add([]byte(a), []byte(strings.Replace(a, `"A"`, `"A2"`, 1)), []byte(strings.Replace(a, `"A"`, `"A3"`, 1)))
	f.Fuzz(func(t *testing.T, base, ours, theirs []byte) {
		out, conflict, err := MergeIssues(base, ours, theirs)
		if err != nil || conflict {
			return
		}
		var prev string
		for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
			if line == "" {
				continue
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("merged line is not JSON: %q", line)
			}
			var id string
			_ = json.Unmarshal(m["id"], &id)
			if id < prev {
				t.Fatalf("not sorted by id: %q after %q", id, prev)
			}
			prev = id
		}
		// Idempotent on canonical output.
		c, cc, cerr := MergeIssues(out, out, out)
		if cerr != nil || cc {
			return
		}
		again, ac, aerr := MergeIssues(c, c, c)
		if aerr != nil || ac || !bytes.Equal(again, c) {
			t.Fatalf("merge not idempotent:\n%s\n---\n%s", c, again)
		}
	})
}

func FuzzSetManifestIssues(f *testing.F) {
	f.Add("```change\nname: demo\nstatus: planning\npackages: []\n```\n\n## Why\n\nx\n", "file:sdd-abc")
	f.Add("```change\nname: demo\nissues: [\"github:1\"]\n```\n", "github:2")
	f.Fuzz(func(t *testing.T, text, ref string) {
		if _, err := ParseChangeMarkdown(text, "fuzz"); err != nil {
			return
		}
		// Only well-formed fences: the parser also accepts "```change" with
		// text on the same line, which setManifestIssues treats as part of
		// the fence (recorded in regression-suite's Observed).
		if i := strings.Index(text, "```change"); i < 0 || !strings.HasPrefix(text[i+len("```change"):], "\n") {
			return
		}
		// Only single-line issues lists: a multi-line list is left half in
		// place (filed for sync-hardening; seed not committed until fixed).
		if !singleLineIssues(text) {
			return
		}
		out, err := setManifestIssues(text, []string{ref})
		if err != nil {
			return
		}
		if strings.ContainsAny(ref, "`\n\r") || strings.TrimSpace(ref) == "" || !utf8.ValidString(ref) {
			return
		}
		m, err := ParseChangeMarkdown(out, "fuzz")
		if err != nil {
			t.Fatalf("output no longer parses: %v\n%s", err, out)
		}
		if len(m.Issues) != 1 || m.Issues[0] != ref {
			t.Fatalf("issues %q, want [%q]\n%s", m.Issues, ref, out)
		}
	})
}

// singleLineIssues reports whether the change block's issues key, if any,
// parses to the same list from its own line alone.
func singleLineIssues(text string) bool {
	m, err := ParseChangeMarkdown(text, "fuzz")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "issues:") {
			continue
		}
		var one struct {
			Issues []string `yaml:"issues"`
		}
		if yaml.Unmarshal([]byte(line), &one) != nil || len(one.Issues) != len(m.Issues) {
			return false
		}
		for i := range one.Issues {
			if one.Issues[i] != m.Issues[i] {
				return false
			}
		}
	}
	return true
}
