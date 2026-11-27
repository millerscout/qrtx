package qrtx

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IssuesFile is the built-in file backend's store, repository-relative.
const IssuesFile = "sdd/issues.jsonl"

// fileTracker is the built-in backend: one canonical JSON issue per line in
// sdd/issues.jsonl, sorted by id. Lines it does not change keep their bytes.
type fileTracker struct {
	root string
	now  func() time.Time
}

func newFileTracker(root string) *fileTracker {
	return &fileTracker{root: root, now: time.Now}
}

func (f *fileTracker) path() string { return filepath.Join(f.root, filepath.FromSlash(IssuesFile)) }

type issueLine struct {
	issue Issue
	raw   []byte
	dirty bool
}

// load reads every issue. Malformed lines are skipped and named in bad.
func (f *fileTracker) load() (lines []*issueLine, bad []string, err error) {
	raw, err := os.ReadFile(f.path())
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	n := 0
	for sc.Scan() {
		n++
		text := bytes.TrimSpace(sc.Bytes())
		if len(text) == 0 {
			continue
		}
		var is Issue
		if err := json.Unmarshal(text, &is); err != nil {
			bad = append(bad, fmt.Sprintf("%s:%d: %v", IssuesFile, n, err))
			continue
		}
		if err := is.Validate(); err != nil {
			bad = append(bad, fmt.Sprintf("%s:%d: %v", IssuesFile, n, err))
			continue
		}
		lines = append(lines, &issueLine{issue: is, raw: append([]byte(nil), text...)})
	}
	return lines, bad, sc.Err()
}

// Warnings returns the malformed-line locations of the current file.
func (f *fileTracker) Warnings() []string {
	_, bad, _ := f.load()
	return bad
}

// update loads the file under the lock, lets fn change it and rewrites it.
func (f *fileTracker) update(fn func(lines []*issueLine) ([]*issueLine, error)) error {
	return withStateLock(f.root, "issues-"+rootKey(f.root), func() error {
		lines, bad, err := f.load()
		if err != nil {
			return err
		}
		if len(bad) > 0 {
			return fmt.Errorf("refusing to rewrite %s: fix %s first", IssuesFile, strings.Join(bad, "; "))
		}
		lines, err = fn(lines)
		if err != nil {
			return err
		}
		sort.SliceStable(lines, func(i, j int) bool { return lines[i].issue.ID < lines[j].issue.ID })
		var buf bytes.Buffer
		for _, l := range lines {
			if l.dirty || l.raw == nil {
				b, err := json.Marshal(l.issue)
				if err != nil {
					return err
				}
				l.raw = b
			}
			buf.Write(l.raw)
			buf.WriteByte('\n')
		}
		if err := os.MkdirAll(filepath.Dir(f.path()), 0o755); err != nil {
			return err
		}
		tmp := f.path() + ".tmp"
		if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, f.path())
	})
}

func (f *fileTracker) stamp() string { return f.now().UTC().Format(time.RFC3339) }

func (f *fileTracker) Capabilities() (TrackerCaps, error) {
	return TrackerCaps{
		Protocols:  []string{AdapterProtocol},
		Interfaces: []string{"tracker@v1"},
		Ops:        []string{OpCapabilities, OpListIssues, OpGetIssue, OpCreateIssue, OpUpdateIssue, OpTransitionIssue},
		LinkTypes:  []string{"blocks", "relates", "duplicates"},
	}, nil
}

func (f *fileTracker) ListIssues(flt IssueFilter) ([]Issue, error) {
	lines, _, err := f.load()
	if err != nil {
		return nil, err
	}
	out := []Issue{}
	for _, l := range lines {
		if matchFilter(l.issue, flt) {
			out = append(out, l.issue)
			if flt.Limit > 0 && len(out) == flt.Limit {
				break
			}
		}
	}
	return out, nil
}

func matchFilter(is Issue, f IssueFilter) bool {
	if len(f.Categories) > 0 && !containsString(f.Categories, is.Status.Category) {
		return false
	}
	for _, l := range f.Labels {
		if !containsString(is.Labels, l) {
			return false
		}
	}
	if f.Assignee != "" && is.Assignee != f.Assignee {
		return false
	}
	if f.Text != "" {
		q := strings.ToLower(f.Text)
		if !strings.Contains(strings.ToLower(is.Title+"\n"+is.Description), q) {
			return false
		}
	}
	if f.Change != "" {
		found := false
		for _, r := range is.Refs {
			if r.Change == f.Change {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func notFound(id string) error {
	return &AdapterError{Code: ErrCodeNotFound, Message: fmt.Sprintf("issue %s does not exist", id)}
}

func (f *fileTracker) GetIssue(id string) (Issue, error) {
	lines, _, err := f.load()
	if err != nil {
		return Issue{}, err
	}
	for _, l := range lines {
		if l.issue.ID == id {
			return l.issue, nil
		}
	}
	return Issue{}, notFound(id)
}

func (f *fileTracker) CreateIssue(in CreateIssueInput) (Issue, error) {
	if strings.TrimSpace(in.Title) == "" {
		return Issue{}, &AdapterError{Code: ErrCodeInvalid, Message: "title is required"}
	}
	if in.ClientRef == "" {
		in.ClientRef = NewClientRef("", "", in.Title, NewNonce())
	}
	cat := in.Category
	if cat == "" {
		cat = CategoryBacklog
	}
	if !ValidCategory(cat) {
		return Issue{}, &AdapterError{Code: ErrCodeInvalid, Message: fmt.Sprintf("category %q", cat)}
	}
	var got Issue
	err := f.update(func(lines []*issueLine) ([]*issueLine, error) {
		taken := map[string]string{}
		for _, l := range lines {
			if l.issue.ClientRef == in.ClientRef {
				got = l.issue
				return lines, nil
			}
			taken[l.issue.ID] = l.issue.ClientRef
		}
		got = Issue{
			ID:          issueIDFor(in.ClientRef, taken),
			Title:       strings.TrimSpace(in.Title),
			Description: in.Description,
			Status:      IssueStatus{Category: cat},
			Labels:      in.Labels,
			Refs:        in.Refs,
			ClientRef:   in.ClientRef,
			UpdatedAt:   f.stamp(),
		}
		return append(lines, &issueLine{issue: got, dirty: true}), nil
	})
	return got, err
}

// change applies fn to the issue with id and rewrites only its line.
func (f *fileTracker) change(id string, fn func(*Issue) (bool, error)) (Issue, error) {
	var got Issue
	err := f.update(func(lines []*issueLine) ([]*issueLine, error) {
		for _, l := range lines {
			if l.issue.ID != id {
				continue
			}
			changed, err := fn(&l.issue)
			if err != nil {
				return nil, err
			}
			if changed {
				l.issue.UpdatedAt = f.stamp()
				l.dirty = true
			}
			got = l.issue
			return lines, nil
		}
		return nil, notFound(id)
	})
	return got, err
}

func (f *fileTracker) UpdateIssue(id string, in UpdateIssueInput) (Issue, error) {
	return f.change(id, func(is *Issue) (bool, error) {
		changed := false
		if in.Title != nil && *in.Title != is.Title {
			is.Title, changed = *in.Title, true
		}
		if in.Description != nil && *in.Description != is.Description {
			is.Description, changed = *in.Description, true
		}
		if in.Labels != nil {
			is.Labels, changed = in.Labels, true
		}
		if is.AddRefs(in.AddRefs) {
			changed = true
		}
		return changed, nil
	})
}

func (f *fileTracker) TransitionIssue(id, category, native string) (Issue, error) {
	if !ValidCategory(category) {
		return Issue{}, &AdapterError{Code: ErrCodeInvalid, Message: fmt.Sprintf("category %q", category)}
	}
	return f.change(id, func(is *Issue) (bool, error) {
		next := IssueStatus{Name: native, Category: category}
		if is.Status == next {
			return false, nil
		}
		is.Status = next
		return true, nil
	})
}

func (f *fileTracker) Comment(id, markdown, clientRef string) error {
	return &AdapterError{Code: ErrCodeUnsupported, Message: "the file backend keeps no comments"}
}

var idEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// issueIDFor derives sdd-<6 chars> from clientRef, lengthening it while the
// id is taken by an issue with a different client_ref.
func issueIDFor(clientRef string, taken map[string]string) string {
	return issueIDFrom(clientRef, taken, 6)
}

// issueIDFrom is issueIDFor starting at minLen characters.
func issueIDFrom(clientRef string, taken map[string]string, minLen int) string {
	sum := sha256.Sum256([]byte(clientRef))
	enc := idEncoding.EncodeToString(sum[:])
	for n := minLen; n <= len(enc); n += 2 {
		id := "sdd-" + enc[:n]
		if owner, ok := taken[id]; !ok || owner == clientRef {
			return id
		}
	}
	return "sdd-" + enc
}
