package jira_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/millerscout/qrtx/providers/jira"
	"github.com/millerscout/qrtx/providers/jira/fakeapi"
	"github.com/millerscout/qrtx/providers/kit/kittest"
	"github.com/millerscout/qrtx/tracker"
)

func setup(t *testing.T) (*fakeapi.Server, *jira.Provider, json.RawMessage) {
	t.Helper()
	fake := fakeapi.New("SHOP")
	t.Cleanup(fake.Close)
	u, _ := url.Parse(fake.URL)
	creds := filepath.Join(t.TempDir(), "credentials.yaml")
	body := fmt.Sprintf("hosts:\n  %s: {email_env: QRTX_TEST_EMAIL, token_env: QRTX_TEST_JIRA, scopes: [SHOP]}\n", u.Host)
	if err := os.WriteFile(creds, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QRTX_CREDENTIALS", creds)
	t.Setenv("QRTX_TEST_EMAIL", "me@example.com")
	t.Setenv("QRTX_TEST_JIRA", "tok")
	cfg := json.RawMessage(fmt.Sprintf(`{"url":%q,"project":"SHOP"}`, fake.URL))
	p := &jira.Provider{Sleep: func(time.Duration) {}}
	if err := p.Init(cfg); err != nil {
		t.Fatal(err)
	}
	return fake, p, cfg
}

func TestADF(t *testing.T) {
	md := "First line with `code`\nsecond line with [a link](https://example.com/x).\n\nSecond paragraph."
	raw := jira.MarkdownToADF(md)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil || doc["type"] != "doc" || doc["version"].(float64) != 1 {
		t.Fatalf("not ADF: %s", raw)
	}
	if !strings.Contains(string(raw), `"hardBreak"`) || !strings.Contains(string(raw), `"code"`) || !strings.Contains(string(raw), `"href":"https://example.com/x"`) {
		t.Fatalf("missing constructs: %s", raw)
	}
	if back := jira.ADFToMarkdown(raw); back != md {
		t.Fatalf("round trip:\n%q\n%q", md, back)
	}
	unknown := `{"type":"doc","version":1,"content":[{"type":"panel","content":[{"type":"paragraph","content":[{"type":"text","text":"inside"}]}]},{"type":"mention","attrs":{"text":"@x"}}]}`
	if got := jira.ADFToMarkdown(json.RawMessage(unknown)); got != "inside" {
		t.Fatalf("unknown nodes: %q", got)
	}
	if got := jira.ADFToMarkdown(json.RawMessage(`"plain"`)); got != "plain" {
		t.Fatalf("string body: %q", got)
	}
}

func TestFakeSearchPages(t *testing.T) {
	fake, p, _ := setup(t)
	for i := 0; i < 150; i++ {
		if _, err := p.CreateIssue(tracker.CreateIssueInput{Title: fmt.Sprint("i", i)}); err != nil {
			t.Fatal(err)
		}
	}
	first, cursor, err := p.ListIssuesPage(tracker.IssueFilter{})
	if err != nil || len(first) != 100 || cursor == "" {
		t.Fatalf("first %d %q %v", len(first), cursor, err)
	}
	second, cursor, _ := p.ListIssuesPage(tracker.IssueFilter{Cursor: cursor})
	if len(second) != 50 || cursor != "" || fake.Count() != 150 {
		t.Fatalf("second %d %q", len(second), cursor)
	}
}

func TestJiraConformance(t *testing.T) {
	fake, p, cfg := setup(t)
	kittest.Run(t, p, cfg)
	for _, is := range fake.Issues {
		if len(is.Description) > 0 && !strings.Contains(string(is.Description), `"type":"doc"`) {
			t.Fatalf("description not ADF: %s", is.Description)
		}
		for _, c := range is.Comments {
			if !strings.Contains(string(c), `"type":"doc"`) {
				t.Fatalf("comment not ADF: %s", c)
			}
		}
	}
}

func TestJiraTransitions(t *testing.T) {
	fake, p, _ := setup(t)
	is, err := p.CreateIssue(tracker.CreateIssueInput{Title: "x", ClientRef: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	// Only In Review and Done are reachable; canceled without a resolution
	// field has no transition.
	fake.Mu.Lock()
	fake.Transitions = []fakeapi.Transition{
		{ID: "31", Name: "Review", To: "In Review", Category: "indeterminate"},
		{ID: "41", Name: "Done", To: "Done", Category: "done"},
	}
	fake.Mu.Unlock()
	got, err := p.TransitionIssue(is.ID, tracker.CategoryStarted, "")
	if err != nil || got.Status.Category != tracker.CategoryStarted || got.Status.Name != "In Review" {
		t.Fatalf("started: %+v %v", got, err)
	}
	_, err = p.TransitionIssue(is.ID, tracker.CategoryStarted, "In Progress")
	var ae *tracker.AdapterError
	if !asAdapter(err, &ae) || ae.Code != "invalid_transition" || strings.Join(ae.Allowed, ",") != "In Review,Done" {
		t.Fatalf("native not reachable: %v %+v", err, ae)
	}
	// Done with a resolution on its screen: canceled sets Won't Do.
	fake.Mu.Lock()
	fake.Transitions = fakeapi.DefaultTransitions()
	fake.Mu.Unlock()
	got, err = p.TransitionIssue(is.ID, tracker.CategoryCanceled, "")
	if err != nil || got.Status.Category != tracker.CategoryCanceled || fake.Get(is.ID).Resolution != "Won't Do" {
		t.Fatalf("canceled: %+v %v res=%q", got, err, fake.Get(is.ID).Resolution)
	}
	// sdd-cref labels are hidden on read and kept on a labels replace.
	if got.ClientRef != "abc" || strings.Contains(strings.Join(got.Labels, ","), "sdd-cref-") {
		t.Fatalf("labels on read: %+v", got)
	}
	got, err = p.UpdateIssue(is.ID, tracker.UpdateIssueInput{Labels: []string{"ui"}, AddRefs: []tracker.SDDRef{{Repo: "r", Change: "demo"}}})
	if err != nil || strings.Join(got.Labels, ",") != "ui" || len(got.Refs) != 1 {
		t.Fatalf("update: %+v %v", got, err)
	}
	if strings.Join(fake.Get(is.ID).Labels, ",") != "ui,sdd-cref-abc" {
		t.Fatalf("stored labels %v", fake.Get(is.ID).Labels)
	}
	if list, _, _ := p.ListIssuesPage(tracker.IssueFilter{Change: "demo"}); len(list) != 1 {
		t.Fatalf("change filter: %d", len(list))
	}
}

func asAdapter(err error, target **tracker.AdapterError) bool {
	ae, ok := err.(*tracker.AdapterError)
	if ok {
		*target = ae
	}
	return ok
}

func TestCreateRetry(t *testing.T) {
	fake, p, cfg := setup(t)
	fake.Mu.Lock()
	fake.StallCreate = 3 * time.Second
	fake.Mu.Unlock()
	t.Setenv("QRTX_TRACKER_DEADLINE", time.Now().Add(time.Second).Format(time.RFC3339Nano))
	_ = p.Init(cfg)
	in := tracker.CreateIssueInput{Title: "once", ClientRef: "retry1"}
	if _, err := p.CreateIssue(in); err == nil {
		t.Fatal("stalled create should time out")
	}
	t.Setenv("QRTX_TRACKER_DEADLINE", time.Now().Add(10*time.Second).Format(time.RFC3339Nano))
	_ = p.Init(cfg)
	is, err := p.CreateIssue(in)
	if err != nil || is.ClientRef != "retry1" || fake.Count() != 1 {
		t.Fatalf("retry: %+v %v count=%d", is, err, fake.Count())
	}
	// Search lag: the next issue stays hidden for two searches.
	fake.Mu.Lock()
	fake.HideNewFromSearch = 2
	fake.Mu.Unlock()
	in2 := tracker.CreateIssueInput{Title: "lagged", ClientRef: "retry2"}
	if _, err := p.CreateIssue(in2); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CreateIssue(in2); err != nil {
		t.Fatal(err)
	}
	if fake.Count() != 2 {
		t.Fatalf("lagged search made a duplicate: %d issues", fake.Count())
	}
}

func TestJiraPrecedence(t *testing.T) {
	fake, p, _ := setup(t)
	two := 2
	a, err := p.CreateIssue(tracker.CreateIssueInput{Title: "A", ClientRef: "pa", Priority: &two})
	if err != nil || a.Priority == nil || *a.Priority != 2 {
		t.Fatalf("create with priority: %+v %v", a, err)
	}
	b, _ := p.CreateIssue(tracker.CreateIssueInput{Title: "B", ClientRef: "pb"})
	link := tracker.IssueLink{Type: "blocks", ID: b.ID}
	one := 1
	got, err := p.UpdateIssue(a.ID, tracker.UpdateIssueInput{AddLinks: []tracker.IssueLink{link}, Priority: &one, AddRefs: []tracker.SDDRef{{Repo: "r", Change: "rename"}}})
	if err != nil || len(got.Links) != 1 || got.Links[0] != link || got.Priority == nil || *got.Priority != 1 || len(got.Refs) != 1 {
		t.Fatalf("link: %+v %v", got, err)
	}
	list, _, _ := p.ListIssuesPage(tracker.IssueFilter{})
	found := false
	for _, is := range list {
		found = found || (is.ID == a.ID && len(is.Links) == 1)
	}
	if !found {
		t.Fatalf("list lost the link: %+v", list)
	}

	// S10: an older plugin rewrites the whole sdd property.
	fake.Mu.Lock()
	raw, _ := json.Marshal(map[string]any{"sdd": []tracker.SDDRef{{Repo: "r", Change: "rename"}, {Repo: "r", Change: "release"}}})
	fake.Issues[0].Properties["sdd"] = raw
	fake.Mu.Unlock()
	after, err := p.GetIssue(a.ID)
	if err != nil || len(after.Links) != 1 || after.Priority == nil || *after.Priority != 1 || len(after.Refs) != 2 {
		t.Fatalf("links or priority lost after an old rewrite: %+v %v", after, err)
	}
	got, _ = p.UpdateIssue(a.ID, tracker.UpdateIssueInput{RemoveLinks: []tracker.IssueLink{link}})
	if len(got.Links) != 0 || got.Priority == nil || *got.Priority != 1 {
		t.Fatalf("unlink: %+v", got)
	}
}
