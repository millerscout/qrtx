package sdd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// RepoName is the repository's normalized origin remote
// (github.com/owner/name), or its directory name without one.
func RepoName(repoRoot string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoRoot
	if out, err := cmd.Output(); err == nil {
		if n := normalizeRemote(strings.TrimSpace(string(out))); n != "" {
			return n
		}
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return filepath.Base(repoRoot)
	}
	return filepath.Base(abs)
}

var scpRemoteRE = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.+)$`)

func normalizeRemote(u string) string {
	u = strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
		if at := strings.Index(u, "@"); at >= 0 && at < strings.Index(u+"/", "/") {
			u = u[at+1:]
		}
		return u
	}
	if m := scpRemoteRE.FindStringSubmatch(u); m != nil {
		return m[1] + "/" + m[2]
	}
	return u
}

func gitBranch(repoRoot string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	b := strings.TrimSpace(string(out))
	if b == "HEAD" {
		return ""
	}
	return b
}

// normalizeRef makes a manifest ref canonical: <backend>:<id>.
func normalizeRef(ref, defaultBackend string) string {
	b, id := ParseIssueRef(ref, defaultBackend)
	return b + ":" + id
}

var manifestIssuesLineRE = regexp.MustCompile(`(?m)^issues:.*\n?`)

// setManifestIssues rewrites the issues line inside the change block and
// leaves the rest of the file alone.
func setManifestIssues(text string, refs []string) (string, error) {
	open := strings.Index(text, "```change")
	if open < 0 {
		return "", fmt.Errorf("missing ```change block")
	}
	bodyStart := strings.Index(text[open:], "\n")
	if bodyStart < 0 {
		return "", fmt.Errorf("unclosed ```change block")
	}
	bodyStart += open + 1
	end := strings.Index(text[bodyStart:], "```")
	if end < 0 {
		return "", fmt.Errorf("unclosed ```change block")
	}
	end += bodyStart
	body := manifestIssuesLineRE.ReplaceAllString(text[bodyStart:end], "")
	if len(refs) > 0 {
		quoted := make([]string, len(refs))
		for i, r := range refs {
			quoted[i] = fmt.Sprintf("%q", r)
		}
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += "issues: [" + strings.Join(quoted, ", ") + "]\n"
	}
	return text[:bodyStart] + body + text[end:], nil
}

// StartResult says what StartFromIssue did.
type StartResult struct {
	Slug    string
	Ref     string
	Created bool
}

// StartFromIssue links issue ref to change slug in both directions. It reads
// the issue first and creates nothing if that fails. It creates the change
// when missing, fills User request only while it is template text, adds the
// ref to the manifest's issues list, and queues a transition to started plus
// an update that adds this change's SDDRef. Unstable until v1.
func StartFromIssue(repoRoot string, t *Tracker, ref, slug string) (StartResult, error) {
	slug = strings.TrimSpace(slug)
	if !ValidChangeSlug(slug) {
		return StartResult{}, fmt.Errorf("invalid change slug %q", slug)
	}
	backend, id := ParseIssueRef(ref, t.Backend)
	impl, err := t.For(backend)
	if err != nil {
		return StartResult{}, err
	}
	is, err := impl.GetIssue(id)
	if err != nil {
		return StartResult{}, err
	}
	res := StartResult{Slug: slug, Ref: backend + ":" + is.ID}
	path := changeFilePath(repoRoot, slug)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := NewChange(repoRoot, slug); err != nil {
			return res, err
		}
		res.Created = true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return res, err
	}
	text := string(raw)
	man, err := ParseChangeMarkdown(text, path)
	if err != nil {
		return res, err
	}
	if strings.Contains(sectionMarkdown(text, "User request"), "(TBD") {
		body := is.Title
		if strings.TrimSpace(is.Description) != "" {
			body += "\n\n" + strings.TrimSpace(is.Description)
		}
		body += "\n\nTracker: `" + res.Ref + "`"
		if is.URL != "" {
			body += " (" + is.URL + ")"
		}
		text = replaceSection(text, "User request", body)
	}
	refs := []string{}
	have := false
	for _, r := range man.Issues {
		n := normalizeRef(r, t.Backend)
		refs = append(refs, n)
		have = have || n == res.Ref
	}
	if !have {
		refs = append(refs, res.Ref)
	}
	if text, err = setManifestIssues(text, refs); err != nil {
		return res, err
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return res, err
	}
	sref := SDDRef{Repo: RepoName(repoRoot), Project: t.Project(), Change: slug, Branch: gitBranch(repoRoot)}
	upd, err := queueEntry(repoRoot, "start:"+slug+":"+res.Ref+":update", OpUpdateIssue, backend, is.ID, outboxUpdateArgs{Input: UpdateIssueInput{AddRefs: []SDDRef{sref}}})
	if err != nil {
		return res, err
	}
	tr, err := queueEntry(repoRoot, "start:"+slug+":"+res.Ref+":transition", OpTransitionIssue, backend, is.ID, outboxTransitionArgs{Category: CategoryStarted, Native: t.Native(CategoryStarted)})
	if err != nil {
		return res, err
	}
	_, err = appendOutbox(repoRoot, []OutboxEntry{upd, tr})
	return res, err
}

// deferredBullets returns the bullets of a change's Deferred section.
func deferredBullets(text string) []string {
	var out []string
	for _, line := range strings.Split(sectionMarkdown(text, "Deferred"), "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "- ") && !strings.HasPrefix(trim, "* ") {
			continue
		}
		b := strings.TrimSpace(trim[2:])
		if b == "" || strings.HasPrefix(b, "(Optional") || strings.HasPrefix(b, "(TBD") {
			continue
		}
		out = append(out, b)
	}
	return out
}

// QueueDeferredIssues queues one create per Deferred bullet of slug. Each
// create's client_ref comes from the change and the bullet text, so a re-run
// never duplicates an issue, even while earlier creates are still pending.
// The issues carry role "deferred" and are not added to the manifest.
func QueueDeferredIssues(repoRoot string, t *Tracker, slug string) (int, error) {
	path := changeFilePath(repoRoot, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	repo := RepoName(repoRoot)
	var entries []OutboxEntry
	for _, b := range deferredBullets(string(raw)) {
		title := truncateRunes(strings.ReplaceAll(b, "`", ""), 120)
		cref := NewClientRef(repo, slug, "deferred\x00"+b, "")
		in := CreateIssueInput{
			Title:       title,
			Description: b + "\n\nDeferred from sdd change `" + slug + "`.",
			Category:    CategoryBacklog,
			Refs:        []SDDRef{{Repo: repo, Project: t.Project(), Change: slug, Role: "deferred"}},
			ClientRef:   cref,
		}
		e, err := queueEntry(repoRoot, "deferred:"+slug+":"+cref, OpCreateIssue, t.Backend, "", in)
		if err != nil {
			return 0, err
		}
		e.ClientRef = cref
		entries = append(entries, e)
	}
	added, err := appendOutbox(repoRoot, entries)
	return len(added), err
}

// queueArchiveTracker queues the tracker writes for archiving slug and
// returns their sequence numbers. With no linked issue it does nothing.
func queueArchiveTracker(repoRoot, slug string, man *ChangeManifest, archivedAs string, requirements int) ([]int, error) {
	if len(man.Issues) == 0 {
		return nil, nil
	}
	s, _, err := loadTrackerSettings(repoRoot)
	if err != nil {
		return nil, err
	}
	others := map[string]bool{}
	slugs, err := ListChanges(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, other := range slugs {
		if other == slug {
			continue
		}
		om, err := ParseChangeFile(repoRoot, other)
		if err != nil {
			continue
		}
		for _, r := range om.Issues {
			others[normalizeRef(r, s.Backend)] = true
		}
	}
	repo := RepoName(repoRoot)
	var entries []OutboxEntry
	for _, r := range man.Issues {
		ref := normalizeRef(r, s.Backend)
		backend, id := ParseIssueRef(ref, s.Backend)
		if s.Comments != "off" {
			msg := fmt.Sprintf("sdd change `%s` was archived in %s as `%s`.", slug, repo, archivedAs)
			if s.Comments == "summary" && requirements > 0 {
				msg += fmt.Sprintf(" It promised %d requirement(s); see its Scenarios for what was tested.", requirements)
			}
			if others[ref] {
				msg += " Another active change still links this issue, so it stays open."
			}
			e, err := queueEntry(repoRoot, "archive:"+slug+":"+ref+":comment", OpComment, backend, id, outboxCommentArgs{Markdown: msg})
			if err != nil {
				return nil, err
			}
			e.ClientRef = NewClientRef(repo, slug, "archive-comment\x00"+ref, "")
			entries = append(entries, e)
		}
		if !others[ref] {
			e, err := queueEntry(repoRoot, "archive:"+slug+":"+ref+":transition", OpTransitionIssue, backend, id, outboxTransitionArgs{Category: CategoryCompleted, Native: s.StatusMap[CategoryCompleted]})
			if err != nil {
				return nil, err
			}
			entries = append(entries, e)
		}
	}
	return appendOutbox(repoRoot, entries)
}
