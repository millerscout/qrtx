// Package tracker is the sdd.adapter/v1 issue tracker protocol: the issue
// model, the operation set, the plugin envelope and the conformance checks.
// It imports only the standard library, so provider plugins can use it
// without the rest of qrtx. Unstable until qrtx v1.
package tracker

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Status categories shared by every tracker backend (sdd.adapter/v1).
const (
	CategoryBacklog   = "backlog"
	CategoryUnstarted = "unstarted"
	CategoryStarted   = "started"
	CategoryCompleted = "completed"
	CategoryCanceled  = "canceled"
)

// StatusCategories lists the valid Status.Category values in lifecycle order.
var StatusCategories = []string{CategoryBacklog, CategoryUnstarted, CategoryStarted, CategoryCompleted, CategoryCanceled}

// ValidCategory reports whether c is one of StatusCategories.
func ValidCategory(c string) bool {
	for _, v := range StatusCategories {
		if v == c {
			return true
		}
	}
	return false
}

// OpenCategory reports whether work in category c is not finished.
func OpenCategory(c string) bool {
	return c == CategoryBacklog || c == CategoryUnstarted || c == CategoryStarted
}

// IssueStatus is a backend's native status name plus its shared category.
type IssueStatus struct {
	Name     string `json:"name,omitempty"`
	Category string `json:"category"`
}

// SDDRef ties an issue to the repository, project and change that work on it.
type SDDRef struct {
	Repo      string   `json:"repo"`
	Project   string   `json:"project,omitempty"`
	Change    string   `json:"change,omitempty"`
	Role      string   `json:"role,omitempty"` // empty, or "deferred" for work a change split off
	Phase     string   `json:"phase,omitempty"`
	Tasks     []string `json:"tasks,omitempty"`
	Scenarios []string `json:"scenarios,omitempty"`
	Branch    string   `json:"branch,omitempty"`
	Commit    string   `json:"commit,omitempty"`
}

func (r SDDRef) sameLink(o SDDRef) bool {
	return r.Repo == o.Repo && r.Project == o.Project && r.Change == o.Change && r.Role == o.Role
}

// IssueLink relates two issues of the same backend.
type IssueLink struct {
	Type string `json:"type"` // blocks | relates | duplicates
	ID   string `json:"id"`
}

// Issue is one work item in the sdd.adapter/v1 tracker schema
// (docs/schema/tracker/v1/issue.json). Unknown top-level keys survive a
// decode and encode unchanged.
type Issue struct {
	ID          string                     `json:"id"`
	URL         string                     `json:"url,omitempty"`
	Title       string                     `json:"title"`
	Description string                     `json:"description,omitempty"`
	Status      IssueStatus                `json:"status"`
	Type        string                     `json:"type,omitempty"`
	Priority    *int                       `json:"priority,omitempty"`
	Labels      []string                   `json:"labels,omitempty"`
	Assignee    string                     `json:"assignee,omitempty"`
	Parent      string                     `json:"parent,omitempty"`
	Links       []IssueLink                `json:"links,omitempty"`
	UpdatedAt   string                     `json:"updated_at,omitempty"`
	Extra       map[string]json.RawMessage `json:"extra,omitempty"`
	Refs        []SDDRef                   `json:"sdd,omitempty"`
	ClientRef   string                     `json:"client_ref,omitempty"`

	unknown map[string]json.RawMessage
}

// issueFields mirrors Issue without the custom methods.
type issueFields struct {
	ID          string                     `json:"id"`
	URL         string                     `json:"url,omitempty"`
	Title       string                     `json:"title"`
	Description string                     `json:"description,omitempty"`
	Status      IssueStatus                `json:"status"`
	Type        string                     `json:"type,omitempty"`
	Priority    *int                       `json:"priority,omitempty"`
	Labels      []string                   `json:"labels,omitempty"`
	Assignee    string                     `json:"assignee,omitempty"`
	Parent      string                     `json:"parent,omitempty"`
	Links       []IssueLink                `json:"links,omitempty"`
	UpdatedAt   string                     `json:"updated_at,omitempty"`
	Extra       map[string]json.RawMessage `json:"extra,omitempty"`
	Refs        []SDDRef                   `json:"sdd,omitempty"`
	ClientRef   string                     `json:"client_ref,omitempty"`
}

var issueKnownKeys = map[string]bool{
	"id": true, "url": true, "title": true, "description": true, "status": true,
	"type": true, "priority": true, "labels": true, "assignee": true, "parent": true,
	"links": true, "updated_at": true, "extra": true, "sdd": true, "client_ref": true,
}

// UnmarshalJSON decodes the schema fields and keeps every other key.
func (i *Issue) UnmarshalJSON(b []byte) error {
	var f issueFields
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	*i = Issue{
		ID: f.ID, URL: f.URL, Title: f.Title, Description: f.Description, Status: f.Status,
		Type: f.Type, Priority: f.Priority, Labels: f.Labels, Assignee: f.Assignee, Parent: f.Parent,
		Links: f.Links, UpdatedAt: f.UpdatedAt, Extra: f.Extra, Refs: f.Refs, ClientRef: f.ClientRef,
	}
	for k, v := range all {
		if issueKnownKeys[k] {
			continue
		}
		if i.unknown == nil {
			i.unknown = map[string]json.RawMessage{}
		}
		i.unknown[k] = v
	}
	return nil
}

// MarshalJSON encodes the issue canonically: object keys sorted at every
// level, numbers kept as written, HTML characters not escaped.
func (i Issue) MarshalJSON() ([]byte, error) {
	known, err := json.Marshal(issueFields{
		ID: i.ID, URL: i.URL, Title: i.Title, Description: i.Description, Status: i.Status,
		Type: i.Type, Priority: i.Priority, Labels: i.Labels, Assignee: i.Assignee, Parent: i.Parent,
		Links: i.Links, UpdatedAt: i.UpdatedAt, Extra: i.Extra, Refs: i.Refs, ClientRef: i.ClientRef,
	})
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(known, &m); err != nil {
		return nil, err
	}
	for k, v := range i.unknown {
		if _, dup := m[k]; !dup {
			m[k] = v
		}
	}
	whole, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return CanonicalJSON(whole)
}

// CanonicalJSON re-encodes raw with sorted keys, json.Number values and no
// HTML escaping, so equal documents encode to equal bytes.
func CanonicalJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Validate checks the fields issue.json requires.
func (i Issue) Validate() error {
	if i.ID == "" {
		return fmt.Errorf("issue: missing id")
	}
	if i.Title == "" {
		return fmt.Errorf("issue %s: missing title", i.ID)
	}
	if !ValidCategory(i.Status.Category) {
		return fmt.Errorf("issue %s: status.category %q is not one of %v", i.ID, i.Status.Category, StatusCategories)
	}
	return nil
}

// AddRefs appends refs not already linked; it reports whether anything changed.
func (i *Issue) AddRefs(refs []SDDRef) bool {
	changed := false
	for _, r := range refs {
		found := false
		for n, have := range i.Refs {
			if have.sameLink(r) {
				found = true
				if r.Branch != "" && have.Branch != r.Branch || r.Commit != "" && have.Commit != r.Commit {
					if r.Branch != "" {
						i.Refs[n].Branch = r.Branch
					}
					if r.Commit != "" {
						i.Refs[n].Commit = r.Commit
					}
					changed = true
				}
				break
			}
		}
		if !found {
			i.Refs = append(i.Refs, r)
			changed = true
		}
	}
	return changed
}

// IssueFilter narrows ListIssues. Empty fields match everything.
type IssueFilter struct {
	Categories []string `json:"categories,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	Assignee   string   `json:"assignee,omitempty"`
	Text       string   `json:"text,omitempty"`
	Change     string   `json:"change,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	Cursor     string   `json:"cursor,omitempty"`
}

// CreateIssueInput is a new issue. ClientRef makes the create idempotent:
// a backend that already holds an issue with it returns that issue.
type CreateIssueInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Refs        []SDDRef `json:"sdd,omitempty"`
	// Priority is 0 (none) to 4 (low), 1 being urgent.
	Priority  *int   `json:"priority,omitempty"`
	ClientRef string `json:"client_ref"`
	// Fresh says ClientRef was never sent before, so a provider may skip
	// its idempotency lookup. Retries of an earlier create leave it false.
	Fresh bool `json:"fresh,omitempty"`
}

// UpdateIssueInput is a partial update; nil fields are left alone.
type UpdateIssueInput struct {
	Title       *string  `json:"title,omitempty"`
	Description *string  `json:"description,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	AddRefs     []SDDRef `json:"add_sdd,omitempty"`
	// AddLinks and RemoveLinks edit the issue's typed links; both are
	// idempotent. Priority sets the priority (0 clears it).
	AddLinks    []IssueLink `json:"add_links,omitempty"`
	RemoveLinks []IssueLink `json:"remove_links,omitempty"`
	Priority    *int        `json:"priority,omitempty"`
}

// LinkTypes lists the valid IssueLink.Type values.
var LinkTypes = []string{"blocks", "relates", "duplicates"}

// ValidLinkType reports whether t is one of LinkTypes.
func ValidLinkType(t string) bool {
	for _, v := range LinkTypes {
		if v == t {
			return true
		}
	}
	return false
}

// HasLink reports whether the issue holds link l, comparing ids exactly.
func (i Issue) HasLink(l IssueLink) bool {
	for _, have := range i.Links {
		if have == l {
			return true
		}
	}
	return false
}

// AddLinks appends links not already held; it reports whether anything changed.
func (i *Issue) AddLinks(links []IssueLink) bool {
	changed := false
	for _, l := range links {
		if !i.HasLink(l) {
			i.Links = append(i.Links, l)
			changed = true
		}
	}
	return changed
}

// RemoveLinks drops the given links; it reports whether anything changed.
func (i *Issue) RemoveLinks(links []IssueLink) bool {
	changed := false
	for _, l := range links {
		kept := i.Links[:0:0]
		for _, have := range i.Links {
			if have == l {
				changed = true
				continue
			}
			kept = append(kept, have)
		}
		i.Links = kept
	}
	if len(i.Links) == 0 {
		i.Links = nil
	}
	return changed
}

// SetPriority sets the priority (0 stores 0); it reports whether it changed.
func (i *Issue) SetPriority(p *int) bool {
	if p == nil {
		return false
	}
	if i.Priority != nil && *i.Priority == *p {
		return false
	}
	v := *p
	i.Priority = &v
	return true
}

// TrackerCaps is what a backend declares in its capabilities response.
type TrackerCaps struct {
	Protocols  []string `json:"protocols"`
	Interfaces []string `json:"interfaces,omitempty"`
	Ops        []string `json:"ops"`
	LinkTypes  []string `json:"link_types,omitempty"`
	Serve      bool     `json:"serve,omitempty"`
}

// Has reports whether op is declared.
func (c TrackerCaps) Has(op string) bool {
	for _, o := range c.Ops {
		if o == op {
			return true
		}
	}
	return false
}

// Tracker operation names (sdd.adapter/v1, kind tracker).
const (
	OpCapabilities    = "capabilities"
	OpListIssues      = "list_issues"
	OpGetIssue        = "get_issue"
	OpCreateIssue     = "create_issue"
	OpUpdateIssue     = "update_issue"
	OpTransitionIssue = "transition_issue"
	OpComment         = "comment"
)

// IssueTracker is a tracker backend. The built-in file backend, exec plugins
// (qrtx-tracker-<name>) and host implementations all satisfy it.
type IssueTracker interface {
	Capabilities() (TrackerCaps, error)
	ListIssues(f IssueFilter) ([]Issue, error)
	GetIssue(id string) (Issue, error)
	CreateIssue(in CreateIssueInput) (Issue, error)
	UpdateIssue(id string, in UpdateIssueInput) (Issue, error)
	// TransitionIssue moves the issue to category; native, when set, names
	// the exact backend status (from .sdd.yaml tracker.status_map).
	TransitionIssue(id, category, native string) (Issue, error)
	Comment(id, markdown, clientRef string) error
}
