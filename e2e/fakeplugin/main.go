// Command fakeplugin is a test-only tracker plugin for the e2e suite. It
// keeps its issues in $QRTX_FAKE_DIR/state.json and reads its behaviour from
// $QRTX_FAKE_DIR/mode: "down" answers every op with unavailable, "bump"
// advances each issue's updated_at on every list. It is built by the e2e
// tests as qrtx-tracker-fake.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/millerscout/qrtx/providers/kit"
	"github.com/millerscout/qrtx/tracker"
)

type state struct {
	Issues []tracker.Issue `json:"issues"`
	Bumps  int             `json:"bumps"`
}

type fake struct{ dir string }

func (f *fake) path(name string) string { return filepath.Join(f.dir, name) }

func (f *fake) mode() string {
	b, _ := os.ReadFile(f.path("mode"))
	return strings.TrimSpace(string(b))
}

func (f *fake) load() state {
	var s state
	b, _ := os.ReadFile(f.path("state.json"))
	_ = json.Unmarshal(b, &s)
	return s
}

func (f *fake) save(s state) {
	b, _ := json.Marshal(s)
	_ = os.WriteFile(f.path("state.json"), b, 0o644)
}

func (f *fake) log(op string) {
	fh, err := os.OpenFile(f.path("log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintln(fh, op)
		fh.Close()
	}
}

func (f *fake) check(op string) error {
	f.log(op)
	if f.mode() == "down" {
		return &tracker.AdapterError{Code: tracker.ErrCodeUnavailable, Message: "fake is down"}
	}
	return nil
}

func (f *fake) Init(json.RawMessage) error {
	f.dir = os.Getenv("QRTX_FAKE_DIR")
	if f.dir == "" {
		return &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: "QRTX_FAKE_DIR is not set"}
	}
	return nil
}

func (f *fake) Capabilities() (tracker.TrackerCaps, error) {
	if err := f.check(tracker.OpCapabilities); err != nil {
		return tracker.TrackerCaps{}, err
	}
	return tracker.TrackerCaps{Protocols: []string{tracker.AdapterProtocol}, Ops: []string{
		tracker.OpCapabilities, tracker.OpListIssues, tracker.OpGetIssue, tracker.OpCreateIssue,
		tracker.OpUpdateIssue, tracker.OpTransitionIssue, tracker.OpComment}}, nil
}

func (f *fake) ListIssuesPage(tracker.IssueFilter) ([]tracker.Issue, string, error) {
	if err := f.check(tracker.OpListIssues); err != nil {
		return nil, "", err
	}
	s := f.load()
	if f.mode() == "bump" {
		s.Bumps++
		for i := range s.Issues {
			s.Issues[i].UpdatedAt = fmt.Sprintf("bump-%d", s.Bumps)
		}
		f.save(s)
	}
	return s.Issues, "", nil
}

func (f *fake) find(s *state, id string) (*tracker.Issue, error) {
	for i := range s.Issues {
		if s.Issues[i].ID == id {
			return &s.Issues[i], nil
		}
	}
	return nil, &tracker.AdapterError{Code: tracker.ErrCodeNotFound, Message: id}
}

func (f *fake) GetIssue(id string) (tracker.Issue, error) {
	if err := f.check(tracker.OpGetIssue); err != nil {
		return tracker.Issue{}, err
	}
	s := f.load()
	is, err := f.find(&s, id)
	if err != nil {
		return tracker.Issue{}, err
	}
	return *is, nil
}

func (f *fake) CreateIssue(in tracker.CreateIssueInput) (tracker.Issue, error) {
	if err := f.check(tracker.OpCreateIssue); err != nil {
		return tracker.Issue{}, err
	}
	s := f.load()
	for _, is := range s.Issues {
		if in.ClientRef != "" && is.ClientRef == in.ClientRef {
			return is, nil
		}
	}
	cat := in.Category
	if cat == "" {
		cat = tracker.CategoryBacklog
	}
	is := tracker.Issue{ID: fmt.Sprintf("F-%d", len(s.Issues)+1), Title: in.Title, Description: in.Description,
		Status: tracker.IssueStatus{Category: cat}, Refs: in.Refs, ClientRef: in.ClientRef, UpdatedAt: "0"}
	s.Issues = append(s.Issues, is)
	f.save(s)
	return is, nil
}

func (f *fake) UpdateIssue(id string, in tracker.UpdateIssueInput) (tracker.Issue, error) {
	if err := f.check(tracker.OpUpdateIssue); err != nil {
		return tracker.Issue{}, err
	}
	s := f.load()
	is, err := f.find(&s, id)
	if err != nil {
		return tracker.Issue{}, err
	}
	if in.Title != nil {
		is.Title = *in.Title
	}
	is.AddRefs(in.AddRefs)
	f.save(s)
	return *is, nil
}

func (f *fake) TransitionIssue(id, category, native string) (tracker.Issue, error) {
	if err := f.check(tracker.OpTransitionIssue); err != nil {
		return tracker.Issue{}, err
	}
	s := f.load()
	is, err := f.find(&s, id)
	if err != nil {
		return tracker.Issue{}, err
	}
	is.Status = tracker.IssueStatus{Name: native, Category: category}
	f.save(s)
	return *is, nil
}

func (f *fake) Comment(id, markdown, clientRef string) error { return f.check(tracker.OpComment) }

func main() { kit.Main(kit.Tracker(&fake{})) }
