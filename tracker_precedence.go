package qrtx

import (
	"fmt"
	"sort"
	"strings"
)

// normalizeLink returns the backend:id form of a link's id. Links are held
// between issues of one backend, so a bare id belongs to backend.
func normalizeLink(id, backend string) string {
	b, i := ParseIssueRef(id, backend)
	return b + ":" + i
}

// sameBackend resolves both refs and fails when they name different backends.
func sameBackend(t *Tracker, aRef, bRef string) (backend, aID, bID string, err error) {
	ab, aID := ParseIssueRef(aRef, t.Backend)
	bb, bID := ParseIssueRef(bRef, t.Backend)
	if ab != bb {
		return "", "", "", fmt.Errorf("tracker link: %s and %s are on different backends (%s, %s); links join issues on the same backend", aRef, bRef, ab, bb)
	}
	return ab, aID, bID, nil
}

// SetLink adds (or, with remove, drops) a typed link from issue aRef to bRef,
// held on aRef. It calls UpdateIssue directly and checks the returned issue,
// so a backend that ignored the change is reported. Unstable until v1.
func SetLink(t *Tracker, aRef, linkType, bRef string, remove bool) (Issue, error) {
	if !ValidLinkType(linkType) {
		return Issue{}, fmt.Errorf("tracker link: type %q is not one of %s", linkType, strings.Join(LinkTypes, ", "))
	}
	backend, aID, bID, err := sameBackend(t, aRef, bRef)
	if err != nil {
		return Issue{}, err
	}
	if aID == bID {
		return Issue{}, fmt.Errorf("tracker link: %s cannot be linked to itself", aRef)
	}
	impl, err := t.For(backend)
	if err != nil {
		return Issue{}, err
	}
	link := IssueLink{Type: linkType, ID: bID}
	in := UpdateIssueInput{AddLinks: []IssueLink{link}}
	if remove {
		in = UpdateIssueInput{RemoveLinks: []IssueLink{link}}
	}
	is, err := impl.UpdateIssue(aID, in)
	if err != nil {
		return Issue{}, err
	}
	held := false
	for _, l := range is.Links {
		if l.Type == linkType && normalizeLink(l.ID, backend) == backend+":"+bID {
			held = true
		}
	}
	switch {
	case !remove && !held:
		return is, fmt.Errorf("tracker %s did not store the link %s %s %s (update_issue ignored add_links)", backend, aRef, linkType, bRef)
	case remove && held:
		return is, fmt.Errorf("tracker %s did not remove the link %s %s %s (update_issue ignored remove_links)", backend, aRef, linkType, bRef)
	}
	return is, nil
}

// ValidPriority reports whether p is in the schema's 0 (none) to 4 (low) range.
func ValidPriority(p int) bool { return p >= 0 && p <= 4 }

// SetPriority sets an issue's priority with a direct UpdateIssue and checks
// the returned issue. Unstable until v1.
func SetPriority(t *Tracker, ref string, p int) (Issue, error) {
	if !ValidPriority(p) {
		return Issue{}, fmt.Errorf("tracker priority: %d is not between 0 (none) and 4 (low)", p)
	}
	backend, id := ParseIssueRef(ref, t.Backend)
	impl, err := t.For(backend)
	if err != nil {
		return Issue{}, err
	}
	is, err := impl.UpdateIssue(id, UpdateIssueInput{Priority: &p})
	if err != nil {
		return Issue{}, err
	}
	if is.Priority == nil || *is.Priority != p {
		return is, fmt.Errorf("tracker %s did not store priority %d for %s (update_issue ignored priority)", backend, p, ref)
	}
	return is, nil
}

// completeLister is an IssueTracker that knows whether its listing was cut off.
type completeLister interface {
	listComplete(f IssueFilter) (issues []Issue, complete bool, err error)
}

// ListOpenIssues lists the issues not completed or canceled. complete is false
// when the backend's paging stopped at its cap, so some open issues may be
// missing. Trackers that cannot tell count as complete. Unstable until v1.
func ListOpenIssues(impl IssueTracker) (issues []Issue, complete bool, err error) {
	f := IssueFilter{Categories: []string{CategoryBacklog, CategoryUnstarted, CategoryStarted}}
	if cl, ok := impl.(completeLister); ok {
		issues, complete, err = cl.listComplete(f)
	} else {
		issues, err = impl.ListIssues(f)
		complete = true
	}
	if err != nil {
		return nil, false, err
	}
	open := issues[:0:0]
	for _, is := range issues { // a backend may ignore the filter
		if OpenCategory(is.Status.Category) {
			open = append(open, is)
		}
	}
	return open, complete, nil
}

// BlockerIndex maps each issue (as backend:id) to the open issues that block
// it (as backend:id, in listing order). Link ids are compared as normalized
// refs on backend.
func BlockerIndex(backend string, open []Issue) map[string][]string {
	out := map[string][]string{}
	for _, is := range open {
		if !OpenCategory(is.Status.Category) {
			continue
		}
		from := backend + ":" + is.ID
		for _, l := range is.Links {
			if l.Type != "blocks" {
				continue
			}
			to := normalizeLink(l.ID, backend)
			if to == from {
				continue
			}
			out[to] = appendOnce(out[to], from)
		}
	}
	return out
}

func appendOnce(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}

// SortByPriority orders issues by priority, 1 first, with none (nil or 0)
// last. Issues of equal priority keep their order.
func SortByPriority(list []Issue) {
	rank := func(is Issue) int {
		if is.Priority == nil || *is.Priority <= 0 {
			return 1 << 20
		}
		return *is.Priority
	}
	sort.SliceStable(list, func(i, j int) bool { return rank(list[i]) < rank(list[j]) })
}
