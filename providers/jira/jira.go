// Package jira is the sdd tracker provider for Jira Cloud (REST API v3).
// It is served as the sdd-tracker-jira plugin.
package jira

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/millerscout/sdd/providers/kit"
	"github.com/millerscout/sdd/tracker"
)

// Config is .sdd.yaml tracker.config for backend jira.
type Config struct {
	URL                 string   `json:"url"`
	Project             string   `json:"project"`
	IssueType           string   `json:"issue_type"`
	CanceledResolutions []string `json:"canceled_resolutions"`
}

// Provider implements kit.Provider for Jira Cloud.
type Provider struct {
	cfg    Config
	client *kit.Client
	// Sleep waits between idempotency lookups; tests replace it.
	Sleep func(time.Duration)
}

const crefPrefix = "sdd-cref-"
const propKey = "sdd"
const fieldList = "summary,description,status,resolution,labels,updated,assignee"

var projectRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`)

// Init reads config and resolves the credential bound to the site for the project.
func (p *Provider) Init(raw json.RawMessage) error {
	var c Config
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &c); err != nil {
			return &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: "tracker.config: " + err.Error()}
		}
	}
	if c.URL == "" {
		return &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: "tracker.config.url is required (https://<site>.atlassian.net)"}
	}
	if !projectRE.MatchString(c.Project) {
		return &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: fmt.Sprintf("tracker.config.project %q must be a project key such as SHOP", c.Project)}
	}
	if c.IssueType == "" {
		c.IssueType = "Task"
	}
	if len(c.CanceledResolutions) == 0 {
		c.CanceledResolutions = []string{"Won't Do", "Won't Fix", "Duplicate", "Declined", "Cannot Reproduce"}
	}
	client, err := kit.NewClient(c.URL, c.Project, "sdd-tracker-jira", kit.BasicAuth)
	if err != nil {
		return err
	}
	p.cfg, p.client = c, client
	if p.Sleep == nil {
		p.Sleep = time.Sleep
	}
	return nil
}

// Capabilities declares every tracker op.
func (p *Provider) Capabilities() (tracker.TrackerCaps, error) {
	return tracker.TrackerCaps{
		Protocols:  []string{tracker.AdapterProtocol},
		Interfaces: []string{"tracker@v1"},
		Ops: []string{tracker.OpCapabilities, tracker.OpListIssues, tracker.OpGetIssue, tracker.OpCreateIssue,
			tracker.OpUpdateIssue, tracker.OpTransitionIssue, tracker.OpComment},
	}, nil
}

type jIssue struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields struct {
		Summary     string          `json:"summary"`
		Description json.RawMessage `json:"description"`
		Labels      []string        `json:"labels"`
		Updated     string          `json:"updated"`
		Status      struct {
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		Resolution *struct {
			Name string `json:"name"`
		} `json:"resolution"`
		Assignee *struct {
			DisplayName string `json:"displayName"`
		} `json:"assignee"`
	} `json:"fields"`
	Properties map[string]json.RawMessage `json:"properties"`
}

type sddProp struct {
	Refs []tracker.SDDRef `json:"sdd"`
}

// categoryOf maps a Jira status category key to an sdd category. The keys
// are new/indeterminate/done; Atlassian's examples also show to-do,
// in-flight and completed, so those are accepted too.
func categoryOf(key string) string {
	switch key {
	case "indeterminate", "in-flight":
		return tracker.CategoryStarted
	case "done", "completed":
		return tracker.CategoryCompleted
	}
	return tracker.CategoryUnstarted
}

func wantKey(category string) string {
	switch category {
	case tracker.CategoryStarted:
		return tracker.CategoryStarted
	case tracker.CategoryCompleted, tracker.CategoryCanceled:
		return tracker.CategoryCompleted
	}
	return tracker.CategoryUnstarted
}

func (p *Provider) canceled(resolution string) bool {
	for _, r := range p.cfg.CanceledResolutions {
		if strings.EqualFold(r, resolution) {
			return true
		}
	}
	return false
}

func (p *Provider) toIssue(j jIssue) tracker.Issue {
	cat := categoryOf(j.Fields.Status.StatusCategory.Key)
	name := j.Fields.Status.Name
	if j.Fields.Resolution != nil && j.Fields.Resolution.Name != "" {
		name += " (" + j.Fields.Resolution.Name + ")"
		if cat == tracker.CategoryCompleted && p.canceled(j.Fields.Resolution.Name) {
			cat = tracker.CategoryCanceled
		}
	}
	is := tracker.Issue{
		ID: j.Key, URL: strings.TrimRight(p.cfg.URL, "/") + "/browse/" + j.Key, Title: j.Fields.Summary,
		Description: ADFToMarkdown(j.Fields.Description), Status: tracker.IssueStatus{Name: name, Category: cat},
		UpdatedAt: j.Fields.Updated,
	}
	for _, l := range j.Fields.Labels {
		if strings.HasPrefix(l, crefPrefix) {
			is.ClientRef = strings.TrimPrefix(l, crefPrefix)
			continue
		}
		is.Labels = append(is.Labels, l)
	}
	if j.Fields.Assignee != nil {
		is.Assignee = j.Fields.Assignee.DisplayName
	}
	if raw, ok := j.Properties[propKey]; ok {
		var sp sddProp
		if json.Unmarshal(raw, &sp) == nil {
			is.Refs = sp.Refs
		}
	}
	return is
}

func (p *Provider) search(jql, token string, max int) ([]jIssue, string, error) {
	q := url.Values{"jql": {jql}, "maxResults": {fmt.Sprint(max)}, "fields": {fieldList}, "properties": {propKey}}
	if token != "" {
		q.Set("nextPageToken", token)
	}
	var res struct {
		Issues        []jIssue `json:"issues"`
		NextPageToken string   `json:"nextPageToken"`
		IsLast        bool     `json:"isLast"`
	}
	if _, err := p.client.Do(kit.Request{Method: "GET", Path: "/rest/api/3/search/jql", Query: q}, &res); err != nil {
		return nil, "", err
	}
	next := res.NextPageToken
	if res.IsLast {
		next = ""
	}
	return res.Issues, next, nil
}

func quoteJQL(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// ListIssuesPage lists one page of the project's issues, newest first.
func (p *Provider) ListIssuesPage(f tracker.IssueFilter) ([]tracker.Issue, string, error) {
	jql := "project = " + quoteJQL(p.cfg.Project)
	for _, l := range f.Labels {
		jql += " AND labels = " + quoteJQL(l)
	}
	jql += " ORDER BY created DESC"
	page, next, err := p.search(jql, f.Cursor, 100)
	if err != nil {
		return nil, "", err
	}
	var out []tracker.Issue
	for _, j := range page {
		is := p.toIssue(j)
		if keep(is, f) {
			out = append(out, is)
		}
	}
	return out, next, nil
}

func keep(is tracker.Issue, f tracker.IssueFilter) bool {
	if len(f.Categories) > 0 {
		ok := false
		for _, c := range f.Categories {
			ok = ok || c == is.Status.Category || (c == tracker.CategoryBacklog && is.Status.Category == tracker.CategoryUnstarted)
		}
		if !ok {
			return false
		}
	}
	if f.Change != "" {
		ok := false
		for _, r := range is.Refs {
			ok = ok || r.Change == f.Change
		}
		if !ok {
			return false
		}
	}
	if f.Text != "" && !strings.Contains(strings.ToLower(is.Title+"\n"+is.Description), strings.ToLower(f.Text)) {
		return false
	}
	return f.Assignee == "" || f.Assignee == is.Assignee
}

func (p *Provider) get(key string) (jIssue, error) {
	var j jIssue
	q := url.Values{"fields": {fieldList}, "properties": {propKey}}
	_, err := p.client.Do(kit.Request{Method: "GET", Path: "/rest/api/3/issue/" + url.PathEscape(key), Query: q}, &j)
	return j, err
}

// GetIssue reads one issue by key.
func (p *Provider) GetIssue(id string) (tracker.Issue, error) {
	j, err := p.get(id)
	if err != nil {
		return tracker.Issue{}, err
	}
	return p.toIssue(j), nil
}

// findByClientRef searches the sdd-cref label, retrying while Jira's search
// index catches up with a create that may have just happened.
func (p *Provider) findByClientRef(clientRef string) (string, error) {
	jql := "project = " + quoteJQL(p.cfg.Project) + " AND labels = " + quoteJQL(crefPrefix+clientRef)
	for try := 0; try < 3; try++ {
		if try > 0 {
			p.Sleep(2 * time.Second)
		}
		page, _, err := p.search(jql, "", 1)
		if err != nil {
			return "", err
		}
		if len(page) > 0 {
			return page[0].Key, nil
		}
	}
	return "", nil
}

func (p *Provider) setRefs(key string, refs []tracker.SDDRef) error {
	_, err := p.client.Do(kit.Request{Method: "PUT", Path: "/rest/api/3/issue/" + url.PathEscape(key) + "/properties/" + propKey, Body: sddProp{Refs: refs}}, nil)
	return err
}

// CreateIssue is idempotent per client_ref: the sdd-cref label is written
// with the issue and searched before creating. The POST is never retried.
func (p *Provider) CreateIssue(in tracker.CreateIssueInput) (tracker.Issue, error) {
	if in.ClientRef != "" {
		key, err := p.findByClientRef(in.ClientRef)
		if err != nil {
			return tracker.Issue{}, err
		}
		if key != "" {
			return p.GetIssue(key)
		}
	}
	labels := append([]string{}, in.Labels...)
	if in.ClientRef != "" {
		labels = append(labels, crefPrefix+in.ClientRef)
	}
	fields := map[string]any{
		"project":   map[string]string{"key": p.cfg.Project},
		"summary":   in.Title,
		"issuetype": map[string]string{"name": p.cfg.IssueType},
		"labels":    labels,
	}
	if strings.TrimSpace(in.Description) != "" {
		fields["description"] = MarkdownToADF(in.Description)
	}
	var created struct {
		Key string `json:"key"`
	}
	if _, err := p.client.Do(kit.Request{Method: "POST", Path: "/rest/api/3/issue", Body: map[string]any{"fields": fields}}, &created); err != nil {
		return tracker.Issue{}, err
	}
	if len(in.Refs) > 0 {
		if err := p.setRefs(created.Key, in.Refs); err != nil {
			return tracker.Issue{}, err
		}
	}
	switch in.Category {
	case tracker.CategoryStarted, tracker.CategoryCompleted, tracker.CategoryCanceled:
		return p.TransitionIssue(created.Key, in.Category, "")
	}
	return p.GetIssue(created.Key)
}

// UpdateIssue edits summary, description and labels (keeping sdd-cref
// labels) and merges sdd refs into the issue property.
func (p *Provider) UpdateIssue(id string, in tracker.UpdateIssueInput) (tracker.Issue, error) {
	j, err := p.get(id)
	if err != nil {
		return tracker.Issue{}, err
	}
	fields := map[string]any{}
	if in.Title != nil {
		fields["summary"] = *in.Title
	}
	if in.Description != nil {
		fields["description"] = MarkdownToADF(*in.Description)
	}
	if in.Labels != nil {
		labels := append([]string{}, in.Labels...)
		for _, l := range j.Fields.Labels {
			if strings.HasPrefix(l, crefPrefix) {
				labels = append(labels, l)
			}
		}
		fields["labels"] = labels
	}
	if len(fields) > 0 {
		if _, err := p.client.Do(kit.Request{Method: "PUT", Path: "/rest/api/3/issue/" + url.PathEscape(id), Body: map[string]any{"fields": fields}}, nil); err != nil {
			return tracker.Issue{}, err
		}
	}
	if len(in.AddRefs) > 0 {
		cur := p.toIssue(j)
		if cur.AddRefs(in.AddRefs) {
			if err := p.setRefs(id, cur.Refs); err != nil {
				return tracker.Issue{}, err
			}
		}
	}
	return p.GetIssue(id)
}

type jTransition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   struct {
		Name           string `json:"name"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"to"`
	Fields map[string]json.RawMessage `json:"fields"`
}

// TransitionIssue moves the issue through an available workflow transition:
// the one reaching the native status name when given, else the first whose
// target category matches. Canceled uses a done transition and sets the
// first canceled resolution when the transition screen has a resolution.
func (p *Provider) TransitionIssue(id, category, native string) (tracker.Issue, error) {
	cur, err := p.GetIssue(id)
	if err != nil {
		return tracker.Issue{}, err
	}
	if cur.Status.Category == category && native == "" {
		return cur, nil
	}
	var res struct {
		Transitions []jTransition `json:"transitions"`
	}
	q := url.Values{"expand": {"transitions.fields"}}
	if _, err := p.client.Do(kit.Request{Method: "GET", Path: "/rest/api/3/issue/" + url.PathEscape(id) + "/transitions", Query: q}, &res); err != nil {
		return tracker.Issue{}, err
	}
	var pick *jTransition
	var allowed []string
	for i, t := range res.Transitions {
		allowed = append(allowed, t.To.Name)
		if pick != nil {
			continue
		}
		if native != "" {
			if strings.EqualFold(t.To.Name, native) {
				pick = &res.Transitions[i]
			}
			continue
		}
		if categoryOf(t.To.StatusCategory.Key) == wantKey(category) {
			pick = &res.Transitions[i]
		}
	}
	if pick == nil {
		return tracker.Issue{}, &tracker.AdapterError{Code: tracker.ErrCodeInvalidTransition,
			Message: fmt.Sprintf("no transition from %q reaches %s", cur.Status.Name, firstNonEmpty(native, category)), Allowed: allowed}
	}
	body := map[string]any{"transition": map[string]string{"id": pick.ID}}
	if category == tracker.CategoryCanceled {
		if _, ok := pick.Fields["resolution"]; ok {
			body["fields"] = map[string]any{"resolution": map[string]string{"name": p.cfg.CanceledResolutions[0]}}
		}
	}
	if _, err := p.client.Do(kit.Request{Method: "POST", Path: "/rest/api/3/issue/" + url.PathEscape(id) + "/transitions", Body: body}, nil); err != nil {
		return tracker.Issue{}, err
	}
	return p.GetIssue(id)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Comment adds an ADF comment. It is not retried, so it never posts twice.
func (p *Provider) Comment(id, markdown, clientRef string) error {
	_, err := p.client.Do(kit.Request{Method: "POST", Path: "/rest/api/3/issue/" + url.PathEscape(id) + "/comment", Body: map[string]json.RawMessage{"body": MarkdownToADF(markdown)}}, nil)
	return err
}
