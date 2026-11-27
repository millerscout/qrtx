package tracker

import (
	"fmt"
	"strings"
	"time"
)

// CheckResult is one operation of a conformance run.
type CheckResult struct {
	Op     string `json:"op"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Conformance runs the qrtx tracker conformance sequence against impl. Without
// allowWrites it only reads (capabilities, list_issues). With it, it creates
// an issue twice with one client_ref, then gets, updates, transitions, lists,
// comments (when declared) and finally cancels it.
func Conformance(impl IssueTracker, allowWrites bool) []CheckResult {
	return ConformanceWith(impl, allowWrites, 0)
}

// ConformanceWith is Conformance with a wait before the retried create, the
// way a real retry from qrtx's outbox comes on a later command. Hosted
// trackers whose lists lag behind a new issue need it.
func ConformanceWith(impl IssueTracker, allowWrites bool, retryDelay time.Duration) []CheckResult {
	var out []CheckResult
	record := func(op string, err error, detail string) bool {
		r := CheckResult{Op: op, OK: err == nil, Detail: detail}
		if err != nil {
			r.Detail = err.Error()
		}
		out = append(out, r)
		return err == nil
	}
	caps, err := impl.Capabilities()
	if !record(OpCapabilities, err, strings.Join(caps.Ops, ",")) {
		return out
	}
	if caps.Has(OpListIssues) {
		list, err := impl.ListIssues(IssueFilter{Limit: 5})
		record(OpListIssues, err, fmt.Sprintf("%d issues", len(list)))
	}
	if !allowWrites || !caps.Has(OpCreateIssue) {
		return out
	}
	ref := NewClientRef("sdd-check", "", "qrtx tracker check", NewNonce())
	in := CreateIssueInput{Title: "qrtx tracker check " + time.Now().UTC().Format(time.RFC3339), Description: "Created by `qrtx tracker check`; safe to delete.", ClientRef: ref}
	first, err := impl.CreateIssue(in)
	if !record(OpCreateIssue, err, first.ID) {
		return out
	}
	if retryDelay > 0 {
		time.Sleep(retryDelay)
	}
	second, err := impl.CreateIssue(in)
	if err == nil && second.ID != first.ID {
		err = fmt.Errorf("retried create with the same client_ref made a second issue (%s, %s)", first.ID, second.ID)
	}
	record(OpCreateIssue+" (idempotent)", err, "")
	if caps.Has(OpGetIssue) {
		got, err := impl.GetIssue(first.ID)
		if err == nil && got.ID != first.ID {
			err = fmt.Errorf("got %s", got.ID)
		}
		record(OpGetIssue, err, "")
	}
	if caps.Has(OpUpdateIssue) {
		title := in.Title + " (updated)"
		got, err := impl.UpdateIssue(first.ID, UpdateIssueInput{Title: &title, AddRefs: []SDDRef{{Repo: "sdd-check", Change: "check"}}})
		if err == nil && (got.Title != title || len(got.Refs) == 0) {
			err = fmt.Errorf("update not applied or sdd refs not kept (title %q, %d refs)", got.Title, len(got.Refs))
		}
		record(OpUpdateIssue, err, "")
	}
	if caps.Has(OpTransitionIssue) {
		got, err := impl.TransitionIssue(first.ID, CategoryStarted, "")
		if err == nil && got.Status.Category != CategoryStarted {
			err = fmt.Errorf("category %s after transition to started", got.Status.Category)
		}
		record(OpTransitionIssue, err, "")
	}
	if caps.Has(OpListIssues) {
		list, err := impl.ListIssues(IssueFilter{})
		found := false
		for _, is := range list {
			found = found || is.ID == first.ID
		}
		if err == nil && !found {
			err = fmt.Errorf("created issue %s missing from list_issues", first.ID)
		}
		record(OpListIssues+" (after create)", err, "")
	}
	if caps.Has(OpComment) {
		record(OpComment, impl.Comment(first.ID, "qrtx tracker check", NewNonce()), "")
	}
	if caps.Has(OpTransitionIssue) {
		_, err := impl.TransitionIssue(first.ID, CategoryCanceled, "")
		record(OpTransitionIssue+" (cleanup: canceled)", err, "")
	}
	return out
}
