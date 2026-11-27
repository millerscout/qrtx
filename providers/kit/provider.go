package kit

import (
	"encoding/json"
	"fmt"

	"github.com/millerscout/qrtx/tracker"
)

// Provider is a tracker backend served by the kit. It is IssueTracker with a
// paged list and an Init that receives .sdd.yaml tracker.config each call.
type Provider interface {
	Init(config json.RawMessage) error
	Capabilities() (tracker.TrackerCaps, error)
	// ListIssuesPage returns one page and the cursor of the next ("" = last).
	ListIssuesPage(f tracker.IssueFilter) ([]tracker.Issue, string, error)
	GetIssue(id string) (tracker.Issue, error)
	CreateIssue(in tracker.CreateIssueInput) (tracker.Issue, error)
	UpdateIssue(id string, in tracker.UpdateIssueInput) (tracker.Issue, error)
	TransitionIssue(id, category, native string) (tracker.Issue, error)
	Comment(id, markdown, clientRef string) error
}

// Tracker serves p as the "tracker" kind.
func Tracker(p Provider) map[string]Handler {
	return map[string]Handler{"tracker": TrackerHandler(p)}
}

// TrackerHandler maps tracker ops, in the argument shapes qrtx sends, to p.
func TrackerHandler(p Provider) Handler {
	return HandlerFunc(func(op string, config, args json.RawMessage) (any, error) {
		if err := p.Init(config); err != nil {
			return nil, err
		}
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		var a struct {
			Filter      tracker.IssueFilter `json:"filter"`
			ID          string              `json:"id"`
			Title       *string             `json:"title"`
			Description *string             `json:"description"`
			Labels      []string            `json:"labels"`
			AddRefs     []tracker.SDDRef    `json:"add_sdd"`
			AddLinks    []tracker.IssueLink `json:"add_links"`
			RemoveLinks []tracker.IssueLink `json:"remove_links"`
			Priority    *int                `json:"priority"`
			Category    string              `json:"category"`
			Native      string              `json:"native"`
			Markdown    string              `json:"markdown"`
			ClientRef   string              `json:"client_ref"`
		}
		if op != tracker.OpCreateIssue {
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, invalidf("%s args: %v", op, err)
			}
		}
		needID := func() error {
			if a.ID == "" {
				return invalidf("%s needs an id", op)
			}
			return nil
		}
		switch op {
		case tracker.OpCapabilities:
			return p.Capabilities()
		case tracker.OpListIssues:
			issues, cursor, err := p.ListIssuesPage(a.Filter)
			if err != nil {
				return nil, err
			}
			if issues == nil {
				issues = []tracker.Issue{}
			}
			return map[string]any{"issues": issues, "cursor": cursor}, nil
		case tracker.OpGetIssue:
			if err := needID(); err != nil {
				return nil, err
			}
			return p.GetIssue(a.ID)
		case tracker.OpCreateIssue:
			var in tracker.CreateIssueInput
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, invalidf("create_issue args: %v", err)
			}
			if in.Title == "" {
				return nil, invalidf("create_issue needs a title")
			}
			return p.CreateIssue(in)
		case tracker.OpUpdateIssue:
			if err := needID(); err != nil {
				return nil, err
			}
			return p.UpdateIssue(a.ID, tracker.UpdateIssueInput{Title: a.Title, Description: a.Description, Labels: a.Labels, AddRefs: a.AddRefs,
				AddLinks: a.AddLinks, RemoveLinks: a.RemoveLinks, Priority: a.Priority})
		case tracker.OpTransitionIssue:
			if err := needID(); err != nil {
				return nil, err
			}
			if !tracker.ValidCategory(a.Category) {
				return nil, invalidf("category %q", a.Category)
			}
			return p.TransitionIssue(a.ID, a.Category, a.Native)
		case tracker.OpComment:
			if err := needID(); err != nil {
				return nil, err
			}
			return struct{}{}, p.Comment(a.ID, a.Markdown, a.ClientRef)
		}
		return nil, &tracker.AdapterError{Code: tracker.ErrCodeUnsupported, Message: fmt.Sprintf("op %q", op)}
	})
}

// AsTracker adapts p to tracker.IssueTracker in-process; ListIssues follows
// cursors (at most 50 pages). Init must already have been called.
func AsTracker(p Provider) tracker.IssueTracker { return asTracker{p} }

type asTracker struct{ Provider }

func (a asTracker) ListIssues(f tracker.IssueFilter) ([]tracker.Issue, error) {
	var all []tracker.Issue
	for page := 0; page < 50; page++ {
		issues, cursor, err := a.ListIssuesPage(f)
		if err != nil {
			return nil, err
		}
		all = append(all, issues...)
		if cursor == "" || (f.Limit > 0 && len(all) >= f.Limit) {
			break
		}
		f.Cursor = cursor
	}
	if f.Limit > 0 && len(all) > f.Limit {
		all = all[:f.Limit]
	}
	return all, nil
}
