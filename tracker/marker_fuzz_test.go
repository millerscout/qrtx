package tracker

import (
	"strings"
	"testing"
)

func FuzzMarker(f *testing.F) {
	f.Add("Some text.", "abc", "demo")
	f.Add("", "", "")
	f.Add("x\n<!-- sdd:v1 {} -->", "r", "c")
	f.Fuzz(func(t *testing.T, desc, cref, change string) {
		m := Marker{ClientRef: cref}
		if change != "" {
			m.Refs = []SDDRef{{Repo: "r", Change: change}}
		}
		body := EncodeMarker(desc, m)
		gotDesc, got, ok := DecodeMarker(body)
		if !ok {
			t.Fatalf("own marker not decoded: %q", body)
		}
		wantDesc, _, _ := DecodeMarker(desc)
		if gotDesc != strings.TrimRight(wantDesc, " \t\r\n") {
			t.Fatalf("description %q, want %q", gotDesc, strings.TrimRight(wantDesc, " \t\r\n"))
		}
		norm := func(s string) string { return string([]rune(s)) } // JSON: one U+FFFD per invalid byte
		if got.ClientRef != norm(cref) {
			t.Fatalf("client_ref %q, want %q", got.ClientRef, norm(cref))
		}
		if (len(got.Refs) > 0) != (change != "") || (change != "" && got.Refs[0].Change != norm(change)) {
			t.Fatalf("refs %+v, want change %q", got.Refs, change)
		}
	})
}
