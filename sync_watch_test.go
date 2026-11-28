package qrtx

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestWatchTaskFiles(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string, age time.Duration) string {
		t.Helper()
		p := filepath.Join(root, "sdd", "changes", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		// Distinct mtimes, whatever the filesystem's resolution.
		at := time.Now().Add(-age)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
		return p
	}
	files := func(box string, age time.Duration) {
		write("phased/change.sdd.md", "- ["+box+"] 1.1 `phases/01-a.md`\n- ["+box+"] V1.1 check\n- ["+box+"] a wiki page\n", age)
		write("phased/phases/01-a.md", "- ["+box+"] 1.1 do it\n", age)
		write("flat/change.sdd.md", "- ["+box+"] 2.1 do it\n", age)
	}
	files(" ", time.Hour)
	watchRoot(root, io.Discard)

	// Entries outside this root's folders: one file is gone, one is there.
	elsewhere := t.TempDir()
	gone, kept := filepath.Join(elsewhere, "gone.md"), filepath.Join(elsewhere, "kept.md")
	if err := os.WriteFile(kept, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := withSync(root, time.Now(), 0, true, func(tx *syncTx) error {
		tx.state.Watch[gone] = &watchEntry{}
		tx.state.Watch[kept] = &watchEntry{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	files("x", time.Minute)
	watchRoot(root, io.Discard)

	evs, err := EventsAfter(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range evs {
		got = append(got, e.Summary()[:strings.Index(e.Summary(), " in ")])
	}
	sort.Strings(got)
	if want := "flat/2.1 done,phased/1.1 done"; strings.Join(got, ",") != want {
		t.Fatalf("events %q, want %q", got, want)
	}
	dir, _ := trackerStateDir(root)
	watch := readSyncStateRO(dir).Watch
	if _, ok := watch[gone]; ok {
		t.Fatal("the entry of a missing file stayed")
	}
	if _, ok := watch[kept]; !ok || len(watch) != 3 {
		t.Fatalf("watch entries: %v", watch)
	}
}

func TestWatchOrder(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "sdd", "changes", "demo", changeFileName)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	var want []string
	for i := 1; i <= 12; i++ {
		want = append(want, fmt.Sprintf("1.%d", i))
	}
	want = append(want, "V1")
	for n, box := range []string{" ", "x"} {
		body := ""
		for _, id := range want {
			body += "- [" + box + "] " + id + " do it\n"
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(time.Duration(n-2) * time.Minute)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
		watchRoot(root, io.Discard)
	}
	evs, err := EventsAfter(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range evs {
		got = append(got, e.Task)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("order %v, want %v", got, want)
	}
}
