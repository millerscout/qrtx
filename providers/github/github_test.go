package github_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/millerscout/qrtx/providers/github"
	"github.com/millerscout/qrtx/providers/github/fakeapi"
	"github.com/millerscout/qrtx/providers/kit"
	"github.com/millerscout/qrtx/providers/kit/kittest"
	"github.com/millerscout/qrtx/tracker"
)

func setup(t *testing.T) (*fakeapi.Server, *github.Provider, json.RawMessage) {
	t.Helper()
	fake := fakeapi.New("o/r")
	t.Cleanup(fake.Close)
	u, _ := url.Parse(fake.URL)
	creds := filepath.Join(t.TempDir(), "credentials.yaml")
	if err := os.WriteFile(creds, []byte(fmt.Sprintf("hosts:\n  %s: {token_env: QRTX_TEST_GH, scopes: [o/r]}\n", u.Host)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QRTX_CREDENTIALS", creds)
	t.Setenv("QRTX_TEST_GH", "tok")
	cfg := json.RawMessage(fmt.Sprintf(`{"repo":"o/r","api_url":%q}`, fake.URL))
	p := &github.Provider{Sleep: func(time.Duration) {}}
	if err := p.Init(cfg); err != nil {
		t.Fatal(err)
	}
	return fake, p, cfg
}

func TestFakeListsPullRequests(t *testing.T) {
	fake, p, _ := setup(t)
	fake.Add(fakeapi.Issue{Title: "issue"})
	fake.Add(fakeapi.Issue{Title: "pr", PR: true})
	list, _, err := p.ListIssuesPage(tracker.IssueFilter{})
	if err != nil || len(list) != 1 || list[0].Title != "issue" {
		t.Fatalf("pull request not skipped: %+v %v", list, err)
	}
	if len(fake.Issues) != 2 {
		t.Fatal("fake should hold the PR too")
	}
}

func TestGitHubConformance(t *testing.T) {
	_, p, cfg := setup(t)
	kittest.Run(t, p, cfg)
}

func TestGitHubMapping(t *testing.T) {
	fake, p, _ := setup(t)
	fake.Add(fakeapi.Issue{Title: "open"})
	fake.Add(fakeapi.Issue{Title: "started", Labels: []string{"in progress"}})
	fake.Add(fakeapi.Issue{Title: "done", State: "closed", StateReason: "completed"})
	fake.Add(fakeapi.Issue{Title: "dropped", State: "closed", StateReason: "not_planned"})
	fake.Add(fakeapi.Issue{Title: "dupe", State: "closed", StateReason: "duplicate", Body: "text\n\n<!-- sdd:v1 {\"client_ref\":\"x\"} -->"})
	fake.Add(fakeapi.Issue{Title: "pr", PR: true})
	list, _, err := p.ListIssuesPage(tracker.IssueFilter{})
	if err != nil || len(list) != 5 {
		t.Fatalf("%d issues %v", len(list), err)
	}
	want := map[string]string{"open": "unstarted", "started": "started", "done": "completed", "dropped": "canceled", "dupe": "canceled"}
	for _, is := range list {
		if want[is.Title] != is.Status.Category {
			t.Fatalf("%s: %s, want %s", is.Title, is.Status.Category, want[is.Title])
		}
		if strings.Contains(is.Description, "<!-- sdd:v1") {
			t.Fatalf("marker leaked: %q", is.Description)
		}
		if is.Title == "dupe" && (is.ClientRef != "x" || is.Description != "text") {
			t.Fatalf("marker not decoded: %+v", is)
		}
	}
}

func TestGitHubPaging(t *testing.T) {
	fake, p, _ := setup(t)
	for i := 0; i < 150; i++ {
		fake.Add(fakeapi.Issue{Title: fmt.Sprint("issue ", i)})
	}
	first, cursor, err := p.ListIssuesPage(tracker.IssueFilter{})
	if err != nil || len(first) != 100 || cursor == "" {
		t.Fatalf("first page %d cursor %q %v", len(first), cursor, err)
	}
	second, cursor, err := p.ListIssuesPage(tracker.IssueFilter{Cursor: cursor})
	if err != nil || len(second) != 50 || cursor != "" {
		t.Fatalf("second page %d cursor %q %v", len(second), cursor, err)
	}
	all, err := kit.AsTracker(p).ListIssues(tracker.IssueFilter{})
	if err != nil || len(all) != 150 {
		t.Fatalf("all %d %v", len(all), err)
	}
}

func TestGitHubStartedLabel(t *testing.T) {
	fake, p, _ := setup(t)
	is, err := p.CreateIssue(tracker.CreateIssueInput{Title: "t", ClientRef: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.TransitionIssue(is.ID, tracker.CategoryStarted, "")
	if err != nil || got.Status.Category != tracker.CategoryStarted || !fake.Labels["in progress"] {
		t.Fatalf("started: %+v %v labels=%v", got, err, fake.Labels)
	}
	// The label exists now: a second issue gets "already_exists" and still works.
	is2, _ := p.CreateIssue(tracker.CreateIssueInput{Title: "t2", ClientRef: "c2"})
	if got, err := p.TransitionIssue(is2.ID, tracker.CategoryStarted, ""); err != nil || got.Status.Category != tracker.CategoryStarted {
		t.Fatalf("already exists: %+v %v", got, err)
	}
	// A labels replace keeps the started label.
	got, err = p.UpdateIssue(is.ID, tracker.UpdateIssueInput{Labels: []string{"ui"}})
	if err != nil || got.Status.Category != tracker.CategoryStarted || strings.Join(got.Labels, ",") != "ui" {
		t.Fatalf("labels replace: %+v %v", got, err)
	}
	// Completing clears the label and closes as completed.
	got, err = p.TransitionIssue(is.ID, tracker.CategoryCompleted, "")
	if err != nil || got.Status.Category != tracker.CategoryCompleted || strings.Contains(strings.Join(fake.Issues[0].Labels, ","), "in progress") {
		t.Fatalf("completed: %+v %v %v", got, err, fake.Issues[0].Labels)
	}
	// Refs live in the marker and survive.
	got, err = p.UpdateIssue(is.ID, tracker.UpdateIssueInput{AddRefs: []tracker.SDDRef{{Repo: "r", Change: "demo"}}})
	if err != nil || len(got.Refs) != 1 || got.ClientRef != "c1" {
		t.Fatalf("refs: %+v %v", got, err)
	}
	if list, _, _ := p.ListIssuesPage(tracker.IssueFilter{Change: "demo"}); len(list) != 1 {
		t.Fatalf("change filter: %d", len(list))
	}
}

func TestCreateRetry(t *testing.T) {
	fake, p, cfg := setup(t)
	fake.Mu.Lock()
	fake.StallCreate = 3 * time.Second
	fake.Mu.Unlock()
	t.Setenv("QRTX_TRACKER_DEADLINE", time.Now().Add(time.Second).Format(time.RFC3339Nano))
	_ = p.Init(cfg)
	in := tracker.CreateIssueInput{Title: "once", ClientRef: "retry-1"}
	if _, err := p.CreateIssue(in); err == nil {
		t.Fatal("stalled create should time out")
	}
	t.Setenv("QRTX_TRACKER_DEADLINE", time.Now().Add(10*time.Second).Format(time.RFC3339Nano))
	_ = p.Init(cfg)
	is, err := p.CreateIssue(in)
	if err != nil || is.ClientRef != "retry-1" {
		t.Fatalf("retry: %+v %v", is, err)
	}
	if n := fake.Count(); n != 1 {
		t.Fatalf("fake holds %d issues", n)
	}
}

// lagFake hides new issues from the list for the first scan, as real GitHub
// does for 3-4s after a create.
func TestCreateFindsLaggedIssue(t *testing.T) {
	fake, p, _ := setup(t)
	slept := 0
	p.Sleep = func(time.Duration) { slept++ }
	first, err := p.CreateIssue(tracker.CreateIssueInput{Title: "a", ClientRef: "lag1"})
	if err != nil {
		t.Fatal(err)
	}
	fake.Mu.Lock()
	fake.HideFromListOnce = first.ID
	fake.Mu.Unlock()
	again, err := p.CreateIssue(tracker.CreateIssueInput{Title: "a", ClientRef: "lag1"})
	if err != nil || again.ID != first.ID || fake.Count() != 1 || slept < 1 {
		t.Fatalf("lagged retry: %+v %v count=%d slept=%d", again, err, fake.Count(), slept)
	}
}

// A fresh client_ref was never sent, so create skips the lookup and its 3s
// lag wait.
func TestCreateFreshSkipsLookup(t *testing.T) {
	fake, p, _ := setup(t)
	slept := 0
	p.Sleep = func(time.Duration) { slept++ }
	is, err := p.CreateIssue(tracker.CreateIssueInput{Title: "a", ClientRef: "fresh1", Fresh: true})
	if err != nil || is.ClientRef != "fresh1" || fake.Count() != 1 || slept != 0 {
		t.Fatalf("fresh create: %+v %v count=%d slept=%d", is, err, fake.Count(), slept)
	}
}
