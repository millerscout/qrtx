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
