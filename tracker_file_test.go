package qrtx

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func readIssueLines(t *testing.T, root string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, IssuesFile))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
}

func TestTrackerLock(t *testing.T) {
	root := t.TempDir()
	counter := filepath.Join(root, "n")
	_ = os.WriteFile(counter, []byte("0"), 0o644)
	var wg sync.WaitGroup
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if err := withStateLock(root, "test", func() error {
					b, _ := os.ReadFile(counter)
					n, _ := strconv.Atoi(string(b))
					return os.WriteFile(counter, []byte(strconv.Itoa(n+1)), 0o644)
				}); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if b, _ := os.ReadFile(counter); string(b) != "100" {
		t.Fatalf("lost updates: %s", b)
	}
	if _, err := os.Stat(filepath.Join(root, ".sdd", ".gitignore")); err != nil {
		t.Fatal("state dir outside git needs its .gitignore")
	}
	if _, err := os.Stat(filepath.Join(root, "sdd")); err == nil {
		t.Fatal("lock files must not live under sdd/")
	}
}

func TestTrackerFile(t *testing.T) {
	root := t.TempDir()
	ft := newFileTracker(root)
	var ids []string
	for _, title := range []string{"one", "two", "three"} {
		is, err := ft.CreateIssue(CreateIssueInput{Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(is.ID, "sdd-") || len(is.ID) != 10 {
			t.Fatalf("id %q", is.ID)
		}
		ids = append(ids, is.ID)
	}
	before := readIssueLines(t, root)
	if len(before) != 3 || !sort.StringsAreSorted(lineIDs(before)) {
		t.Fatalf("want 3 sorted lines: %v", before)
	}
	if _, err := ft.TransitionIssue(ids[1], CategoryCompleted, ""); err != nil {
		t.Fatal(err)
	}
	after := readIssueLines(t, root)
	diff := 0
	for i := range after {
		if after[i] != before[i] {
			diff++
			if !strings.Contains(after[i], ids[1]) || !strings.Contains(after[i], `"completed"`) {
				t.Fatalf("wrong line changed: %s", after[i])
			}
		}
	}
	if diff != 1 {
		t.Fatalf("%d lines changed, want 1", diff)
	}

	// S3/Q3: a retried create returns the first issue.
	a, err := ft.CreateIssue(CreateIssueInput{Title: "retry", ClientRef: "fixedref0001"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ft.CreateIssue(CreateIssueInput{Title: "retry", ClientRef: "fixedref0001"})
	if err != nil || a.ID != b.ID {
		t.Fatalf("retry made %s and %s (%v)", a.ID, b.ID, err)
	}
	if n := len(readIssueLines(t, root)); n != 4 {
		t.Fatalf("want 4 issues, got %d", n)
	}

	// Q2: an id clash with another client_ref lengthens the id.
	id6 := issueIDFor("x", nil)
	if got := issueIDFor("x", map[string]string{id6: "other"}); len(got) != len(id6)+2 || !strings.HasPrefix(got, id6) {
		t.Fatalf("clash id %q from %q", got, id6)
	}

	// Q2: same titles never collide.
	c1, _ := ft.CreateIssue(CreateIssueInput{Title: "Same title"})
	c2, _ := ft.CreateIssue(CreateIssueInput{Title: "Same title"})
	if c1.ID == c2.ID {
		t.Fatal("same title, same id")
	}
}

func TestTrackerFileKeepsUnknownFields(t *testing.T) {
	root := t.TempDir()
	line := `{"extra":{"jira":{"customfield_1":"x"}},"id":"sdd-aaaaaa","rank":3,"status":{"category":"started"},"title":"T"}`
	if err := os.MkdirAll(filepath.Join(root, "sdd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, IssuesFile), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newFileTracker(root).TransitionIssue("sdd-aaaaaa", CategoryCompleted, ""); err != nil {
		t.Fatal(err)
	}
	got := readIssueLines(t, root)[0]
	for _, want := range []string{`"extra":{"jira":{"customfield_1":"x"}}`, `"rank":3`, `"category":"completed"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("lost %s: %s", want, got)
		}
	}
}

func TestTrackerFileMalformedLine(t *testing.T) {
	root := t.TempDir()
	lines := []string{
		`{"id":"sdd-aaaaaa","status":{"category":"backlog"},"title":"a"}`,
		`{"id":"sdd-bbbbbb","status":{"category":"backlog"},"title":"b"}`,
		`{"id":"sdd-cccccc","status":{"category":"backlog"},"title":"c"}`,
		`{not json`,
		`{"id":"sdd-dddddd","status":{"category":"backlog"},"title":"d"}`,
	}
	_ = os.MkdirAll(filepath.Join(root, "sdd"), 0o755)
	orig := []byte(strings.Join(lines, "\n") + "\n")
	_ = os.WriteFile(filepath.Join(root, IssuesFile), orig, 0o644)
	ft := newFileTracker(root)
	list, err := ft.ListIssues(IssueFilter{})
	if err != nil || len(list) != 4 {
		t.Fatalf("list %d %v", len(list), err)
	}
	if w := ft.Warnings(); len(w) != 1 || !strings.HasPrefix(w[0], "sdd/issues.jsonl:4") {
		t.Fatalf("warnings %v", w)
	}
	if _, err := ft.TransitionIssue("sdd-aaaaaa", CategoryStarted, ""); err == nil || !strings.Contains(err.Error(), "sdd/issues.jsonl:4") {
		t.Fatalf("write should refuse: %v", err)
	}
	if now, _ := os.ReadFile(filepath.Join(root, IssuesFile)); !bytes.Equal(now, orig) {
		t.Fatal("file rewritten despite malformed line")
	}
}

func lineIDs(lines []string) []string {
	var out []string
	for _, l := range lines {
		i := strings.Index(l, `"id":"`)
		rest := l[i+6:]
		out = append(out, rest[:strings.Index(rest, `"`)])
	}
	return out
}

func TestTrackerFileLinks(t *testing.T) {
	root := t.TempDir()
	ft := newFileTracker(root)
	a, _ := ft.CreateIssue(CreateIssueInput{Title: "A"})
	b, _ := ft.CreateIssue(CreateIssueInput{Title: "B", Priority: intp(3)})
	if b.Priority == nil || *b.Priority != 3 {
		t.Fatalf("create ignored priority: %+v", b)
	}
	link := IssueLink{Type: "blocks", ID: b.ID}
	for i := 0; i < 2; i++ {
		got, err := ft.UpdateIssue(a.ID, UpdateIssueInput{AddLinks: []IssueLink{link}})
		if err != nil || len(got.Links) != 1 || got.Links[0] != link {
			t.Fatalf("link %d: %+v %v", i, got.Links, err)
		}
	}
	before := strings.Join(readIssueLines(t, root), "\n")
	if _, err := ft.UpdateIssue(a.ID, UpdateIssueInput{AddLinks: []IssueLink{link}}); err != nil {
		t.Fatal(err)
	}
	if after := strings.Join(readIssueLines(t, root), "\n"); after != before {
		t.Fatalf("a repeated link rewrote the file:\n%s\n%s", before, after)
	}
	got, err := ft.UpdateIssue(b.ID, UpdateIssueInput{Priority: intp(2)})
	if err != nil || got.Priority == nil || *got.Priority != 2 {
		t.Fatalf("priority: %+v %v", got, err)
	}
	got, err = ft.UpdateIssue(a.ID, UpdateIssueInput{RemoveLinks: []IssueLink{link}})
	if err != nil || len(got.Links) != 0 {
		t.Fatalf("unlink: %+v %v", got.Links, err)
	}
	if strings.Contains(strings.Join(readIssueLines(t, root), "\n"), `"links"`) {
		t.Fatal("an empty links key was written")
	}
}

func intp(n int) *int { return &n }
