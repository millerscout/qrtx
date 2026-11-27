// Package github is the sdd tracker provider for GitHub Issues (REST API).
// It is served as the sdd-tracker-github plugin.
package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/millerscout/sdd/providers/kit"
	"github.com/millerscout/sdd/tracker"
)

// Config is .sdd.yaml tracker.config for backend github.
type Config struct {
	Repo         string `json:"repo"`
	APIURL       string `json:"api_url"`
	APIVersion   string `json:"api_version"`
	StartedLabel string `json:"started_label"`
}

// Provider implements kit.Provider for GitHub Issues.
type Provider struct {
	cfg    Config
	client *kit.Client
	login  string
	// Sleep waits before the second idempotency scan; tests replace it.
	Sleep func(time.Duration)
}

var repoRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Init reads config and resolves the credential bound to the API host for repo.
func (p *Provider) Init(raw json.RawMessage) error {
	var c Config
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &c); err != nil {
			return &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: "tracker.config: " + err.Error()}
		}
	}
	if !repoRE.MatchString(c.Repo) {
		return &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: fmt.Sprintf("tracker.config.repo %q must be owner/name", c.Repo)}
	}
	if c.APIURL == "" {
		c.APIURL = "https://api.github.com"
	}
	if c.APIVersion == "" {
		c.APIVersion = "2022-11-28"
	}
	if c.StartedLabel == "" {
		c.StartedLabel = "in progress"
	}
	client, err := kit.NewClient(c.APIURL, c.Repo, "sdd-tracker-github", kit.BearerAuth)
	if err != nil {
		return err
	}
	client.Headers.Set("Accept", "application/vnd.github+json")
	client.Headers.Set("X-GitHub-Api-Version", c.APIVersion)
	p.cfg, p.client, p.login = c, client, ""
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

type ghIssue struct {
	Number      int     `json:"number"`
	Title       string  `json:"title"`
	Body        string  `json:"body"`
	State       string  `json:"state"`
	StateReason *string `json:"state_reason"`
	HTMLURL     string  `json:"html_url"`
	UpdatedAt   string  `json:"updated_at"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignee *struct {
		Login string `json:"login"`
	} `json:"assignee"`
	PullRequest json.RawMessage `json:"pull_request"`
}

func (g ghIssue) labels() []string {
	var out []string
	for _, l := range g.Labels {
		out = append(out, l.Name)
	}
	return out
}

func (g ghIssue) has(label string) bool {
	for _, l := range g.Labels {
		if l.Name == label {
			return true
		}
	}
	return false
}

func (p *Provider) path(format string, a ...any) string {
	return "/repos/" + p.cfg.Repo + fmt.Sprintf(format, a...)
}

func (p *Provider) toIssue(g ghIssue) tracker.Issue {
	desc, m, _ := tracker.DecodeMarker(g.Body)
	reason := ""
	if g.StateReason != nil {
		reason = *g.StateReason
	}
	cat := tracker.CategoryUnstarted
	switch {
	case g.State == "closed" && (reason == "not_planned" || reason == "duplicate"):
		cat = tracker.CategoryCanceled
	case g.State == "closed":
		cat = tracker.CategoryCompleted
	case g.has(p.cfg.StartedLabel):
		cat = tracker.CategoryStarted
	}
	name := g.State
	if reason != "" && g.State == "closed" {
		name += " (" + reason + ")"
	} else if cat == tracker.CategoryStarted {
		name += " (" + p.cfg.StartedLabel + ")"
	}
	var labels []string
	for _, l := range g.labels() {
		if l != p.cfg.StartedLabel {
			labels = append(labels, l)
		}
	}
	is := tracker.Issue{
		ID: strconv.Itoa(g.Number), URL: g.HTMLURL, Title: g.Title, Description: desc,
		Status: tracker.IssueStatus{Name: name, Category: cat}, Labels: labels, UpdatedAt: g.UpdatedAt,
		Refs: m.Refs, ClientRef: m.ClientRef,
	}
	if g.Assignee != nil {
		is.Assignee = g.Assignee.Login
	}
	return is
}

var linkNextRE = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func nextPage(h http.Header) string {
	m := linkNextRE.FindStringSubmatch(h.Get("Link"))
	if m == nil {
		return ""
	}
	u, err := url.Parse(m[1])
	if err != nil {
		return ""
	}
	return u.Query().Get("page")
}

func hasRefChange(is tracker.Issue, change string) bool {
	for _, r := range is.Refs {
		if r.Change == change {
			return true
		}
	}
	return false
}

func matches(is tracker.Issue, f tracker.IssueFilter) bool {
	if len(f.Categories) > 0 {
		ok := false
		for _, c := range f.Categories {
			ok = ok || c == is.Status.Category
		}
		if !ok {
			return false
		}
	}
	if f.Change != "" && !hasRefChange(is, f.Change) {
		return false
	}
	if f.Text != "" && !strings.Contains(strings.ToLower(is.Title+"\n"+is.Description), strings.ToLower(f.Text)) {
		return false
	}
	if f.Assignee != "" && is.Assignee != f.Assignee {
		return false
	}
	return true
}

// ListIssuesPage lists one page of issues (pull requests skipped). The
// cursor is the next page number.
func (p *Provider) ListIssuesPage(f tracker.IssueFilter) ([]tracker.Issue, string, error) {
	state := "all"
	open, closed := false, false
	for _, c := range f.Categories {
		if tracker.OpenCategory(c) {
			open = true
		} else {
			closed = true
		}
	}
	if open && !closed {
		state = "open"
	} else if closed && !open {
		state = "closed"
	}
	q := url.Values{"state": {state}, "per_page": {"100"}, "sort": {"created"}, "direction": {"desc"}}
	if len(f.Labels) > 0 {
		q.Set("labels", strings.Join(f.Labels, ","))
	}
	if f.Cursor != "" {
		q.Set("page", f.Cursor)
	}
	var page []ghIssue
	hdr, err := p.client.Do(kit.Request{Method: "GET", Path: p.path("/issues"), Query: q}, &page)
	if err != nil {
		return nil, "", err
	}
	var out []tracker.Issue
	for _, g := range page {
		if len(g.PullRequest) > 0 && string(g.PullRequest) != "null" {
			continue
		}
		is := p.toIssue(g)
		if matches(is, f) {
			out = append(out, is)
		}
	}
	return out, nextPage(hdr), nil
}

func (p *Provider) get(id string) (ghIssue, error) {
	var g ghIssue
	if _, err := strconv.Atoi(id); err != nil {
		return g, &tracker.AdapterError{Code: tracker.ErrCodeNotFound, Message: "issue " + id + " is not a GitHub issue number"}
	}
	if _, err := p.client.Do(kit.Request{Method: "GET", Path: p.path("/issues/%s", id)}, &g); err != nil {
		return g, err
	}
	if len(g.PullRequest) > 0 && string(g.PullRequest) != "null" {
		return g, &tracker.AdapterError{Code: tracker.ErrCodeNotFound, Message: "#" + id + " is a pull request"}
	}
	return g, nil
}

// GetIssue reads one issue.
func (p *Provider) GetIssue(id string) (tracker.Issue, error) {
	g, err := p.get(id)
	if err != nil {
		return tracker.Issue{}, err
	}
	return p.toIssue(g), nil
}

func (p *Provider) whoami() (string, error) {
	if p.login != "" {
		return p.login, nil
	}
	var u struct {
		Login string `json:"login"`
	}
	if _, err := p.client.Do(kit.Request{Method: "GET", Path: "/user"}, &u); err != nil {
		return "", err
	}
	p.login = u.Login
	return u.Login, nil
}

// findByClientRef scans the authenticated user's 100 newest issues (all
// states) for a marker with clientRef, and scans once more after 3s: GitHub's
// issue list shows a new issue only 3-4s after it is created (measured
// 2026-11-26). Search is not used: it is rate limited and lags more.
func (p *Provider) findByClientRef(clientRef string) (*ghIssue, error) {
	g, err := p.scanForClientRef(clientRef)
	if g != nil || err != nil {
		return g, err
	}
	p.Sleep(3 * time.Second)
	return p.scanForClientRef(clientRef)
}

func (p *Provider) scanForClientRef(clientRef string) (*ghIssue, error) {
	login, err := p.whoami()
	if err != nil {
		return nil, err
	}
	q := url.Values{"creator": {login}, "state": {"all"}, "sort": {"created"}, "direction": {"desc"}, "per_page": {"100"}}
	var page []ghIssue
	if _, err := p.client.Do(kit.Request{Method: "GET", Path: p.path("/issues"), Query: q}, &page); err != nil {
		return nil, err
	}
	for i := range page {
		if _, m, ok := tracker.DecodeMarker(page[i].Body); ok && m.ClientRef == clientRef {
			return &page[i], nil
		}
	}
	return nil, nil
}

// CreateIssue is idempotent per client_ref (see findByClientRef). The POST
// itself is never retried.
func (p *Provider) CreateIssue(in tracker.CreateIssueInput) (tracker.Issue, error) {
	if in.ClientRef != "" && !in.Fresh {
		g, err := p.findByClientRef(in.ClientRef)
		if err != nil {
			return tracker.Issue{}, err
		}
		if g != nil {
			return p.toIssue(*g), nil
		}
	}
	body := tracker.EncodeMarker(in.Description, tracker.Marker{ClientRef: in.ClientRef, Refs: in.Refs})
	req := map[string]any{"title": in.Title, "body": body}
	if len(in.Labels) > 0 {
		req["labels"] = in.Labels
	}
	var g ghIssue
	if _, err := p.client.Do(kit.Request{Method: "POST", Path: p.path("/issues"), Body: req}, &g); err != nil {
		return tracker.Issue{}, err
	}
	switch in.Category {
	case tracker.CategoryStarted, tracker.CategoryCompleted, tracker.CategoryCanceled:
		return p.TransitionIssue(strconv.Itoa(g.Number), in.Category, "")
	}
	return p.toIssue(g), nil
}

// UpdateIssue patches title, description, labels and sdd refs. A labels
// replace keeps the started label.
func (p *Provider) UpdateIssue(id string, in tracker.UpdateIssueInput) (tracker.Issue, error) {
	g, err := p.get(id)
	if err != nil {
		return tracker.Issue{}, err
	}
	desc, m, _ := tracker.DecodeMarker(g.Body)
	patch := map[string]any{}
	if in.Title != nil {
		patch["title"] = *in.Title
	}
	changedBody := false
	if in.Description != nil {
		desc, changedBody = *in.Description, true
	}
	is := tracker.Issue{Refs: m.Refs}
	if is.AddRefs(in.AddRefs) {
		changedBody = true
	}
	if changedBody {
		patch["body"] = tracker.EncodeMarker(desc, tracker.Marker{ClientRef: m.ClientRef, Refs: is.Refs})
	}
	if in.Labels != nil {
		labels := append([]string{}, in.Labels...)
		if g.has(p.cfg.StartedLabel) {
			labels = append(labels, p.cfg.StartedLabel)
		}
		patch["labels"] = labels
	}
	if len(patch) == 0 {
		return p.toIssue(g), nil
	}
	var out ghIssue
	if _, err := p.client.Do(kit.Request{Method: "PATCH", Path: p.path("/issues/%s", id), Body: patch}, &out); err != nil {
		return tracker.Issue{}, err
	}
	return p.toIssue(out), nil
}

func (p *Provider) ensureLabel(name string) error {
	_, err := p.client.Do(kit.Request{Method: "POST", Path: p.path("/labels"), Body: map[string]string{"name": name, "color": "fbca04"}}, nil)
	if err != nil && strings.Contains(err.Error(), "already_exists") {
		return nil
	}
	return err
}

// TransitionIssue opens or closes the issue and sets or clears the started
// label. native "duplicate" closes as a duplicate.
func (p *Provider) TransitionIssue(id, category, native string) (tracker.Issue, error) {
	g, err := p.get(id)
	if err != nil {
		return tracker.Issue{}, err
	}
	patch := map[string]any{}
	switch category {
	case tracker.CategoryBacklog, tracker.CategoryUnstarted, tracker.CategoryStarted:
		if g.State != "open" {
			patch["state"], patch["state_reason"] = "open", "reopened"
		}
	case tracker.CategoryCompleted:
		patch["state"], patch["state_reason"] = "closed", "completed"
	case tracker.CategoryCanceled:
		reason := "not_planned"
		if native == "duplicate" {
			reason = "duplicate"
		}
		patch["state"], patch["state_reason"] = "closed", reason
	default:
		return tracker.Issue{}, &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: "category " + category}
	}
	if len(patch) > 0 {
		if _, err := p.client.Do(kit.Request{Method: "PATCH", Path: p.path("/issues/%s", id), Body: patch}, nil); err != nil {
			return tracker.Issue{}, err
		}
	}
	label := p.cfg.StartedLabel
	if category == tracker.CategoryStarted && !g.has(label) {
		if err := p.ensureLabel(label); err != nil {
			return tracker.Issue{}, err
		}
		if _, err := p.client.Do(kit.Request{Method: "POST", Path: p.path("/issues/%s/labels", id), Body: map[string][]string{"labels": {label}}, Safe: true}, nil); err != nil {
			return tracker.Issue{}, err
		}
	} else if category != tracker.CategoryStarted && g.has(label) {
		_, err := p.client.Do(kit.Request{Method: "DELETE", Path: p.path("/issues/%s/labels/%s", id, label)}, nil)
		if err != nil && !strings.Contains(err.Error(), "HTTP 404") {
			return tracker.Issue{}, err
		}
	}
	return p.GetIssue(id)
}

// Comment posts an issue comment. It is not retried, so it never posts twice.
func (p *Provider) Comment(id, markdown, clientRef string) error {
	if _, err := strconv.Atoi(id); err != nil {
		return &tracker.AdapterError{Code: tracker.ErrCodeNotFound, Message: "issue " + id}
	}
	_, err := p.client.Do(kit.Request{Method: "POST", Path: p.path("/issues/%s/comments", id), Body: map[string]string{"body": markdown}}, nil)
	return err
}
