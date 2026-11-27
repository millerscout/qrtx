package tracker

import (
	"os/exec"
	"strings"
	"testing"
)

func TestTrackerPackageStdlibOnly(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs go")
	}
	out, err := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "github.com/millerscout/qrtx/tracker" {
		t.Fatalf("non-standard deps:\n%s", got)
	}
}

func TestMarker(t *testing.T) {
	m := Marker{ClientRef: "abc", Refs: []SDDRef{{Repo: "r", Change: "c"}}}
	body := EncodeMarker("Some text.\n", m)
	if !strings.HasSuffix(body, `<!-- sdd:v1 {"client_ref":"abc","sdd":[{"repo":"r","change":"c"}]} -->`) {
		t.Fatalf("encoded %q", body)
	}
	desc, got, ok := DecodeMarker(body)
	if !ok || desc != "Some text." || got.ClientRef != "abc" || len(got.Refs) != 1 {
		t.Fatalf("decode %q %+v %v", desc, got, ok)
	}
	// Re-encoding replaces, never stacks.
	again := EncodeMarker(body, Marker{ClientRef: "xyz"})
	if strings.Count(again, "<!-- sdd:v1") != 1 || !strings.Contains(again, "xyz") {
		t.Fatalf("re-encode %q", again)
	}
	// Marker only.
	if d, got, ok := DecodeMarker(EncodeMarker("", m)); !ok || d != "" || got.ClientRef != "abc" {
		t.Fatalf("marker only: %q %+v %v", d, got, ok)
	}
	// No marker, a marker in the middle, and a malformed marker stay text.
	for _, s := range []string{"plain", "a\n<!-- sdd:v1 {} -->\nb", "x\n<!-- sdd:v1 {nope -->"} {
		if d, _, ok := DecodeMarker(s); ok || d != s {
			t.Fatalf("%q: %q %v", s, d, ok)
		}
	}
}

func TestIssuePrecedence(t *testing.T) {
	two := 2
	p := Precedence{Priority: &two, Links: []IssueLink{{Type: "blocks", ID: "7"}}}
	body := EncodeMarker(EncodePrec("Some text.", p), Marker{ClientRef: "abc", Refs: []SDDRef{{Repo: "r"}}})
	lines := strings.Split(body, "\n")
	if !strings.HasPrefix(lines[len(lines)-1], "<!-- sdd:v1 ") || !strings.Contains(body, "<!-- sdd-prec:v1 ") {
		t.Fatalf("the sdd marker must stay last, after the prec line: %q", body)
	}

	// An older plugin decodes only the sdd marker and rewrites it whole; the
	// prec line stays in the description it hands back.
	oldDesc, oldM, ok := DecodeMarker(body)
	if !ok || oldM.ClientRef != "abc" {
		t.Fatalf("old decode: %+v %v", oldM, ok)
	}
	oldM.Refs = append(oldM.Refs, SDDRef{Repo: "r", Change: "c"})
	rewritten := EncodeMarker(oldDesc, oldM)

	desc, m, _ := DecodeMarker(rewritten)
	desc, got, ok := DecodePrec(desc)
	if !ok || desc != "Some text." || len(m.Refs) != 2 || got.Priority == nil || *got.Priority != 2 || len(got.Links) != 1 || got.Links[0].ID != "7" {
		t.Fatalf("prec lost across an old rewrite: %q %+v %+v", desc, m, got)
	}

	if out := EncodePrec(EncodePrec("x", p), Precedence{}); out != "x" {
		t.Fatalf("empty precedence must remove the line: %q", out)
	}
	if _, _, ok := DecodePrec("text\n\n<!-- sdd-prec:v1 {bad -->"); ok {
		t.Fatal("malformed prec accepted")
	}

	var prec Precedence
	if !prec.Apply(UpdateIssueInput{AddLinks: []IssueLink{{Type: "blocks", ID: "1"}}, Priority: &two}) {
		t.Fatal("apply reported no change")
	}
	if prec.Apply(UpdateIssueInput{AddLinks: []IssueLink{{Type: "blocks", ID: "1"}}, Priority: &two}) {
		t.Fatal("second apply must be a no-op")
	}
	prec.Apply(UpdateIssueInput{RemoveLinks: []IssueLink{{Type: "blocks", ID: "1"}}})
	if len(prec.Links) != 0 || prec.Priority == nil {
		t.Fatalf("remove: %+v", prec)
	}
}
