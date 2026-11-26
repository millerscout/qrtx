package sdd

import "github.com/millerscout/sdd/tracker"

// The tracker protocol lives in package tracker (standard library only, so
// provider plugins can import it). These aliases keep the root API.

// Status categories shared by every tracker backend (sdd.adapter/v1).
const (
	CategoryBacklog   = tracker.CategoryBacklog
	CategoryUnstarted = tracker.CategoryUnstarted
	CategoryStarted   = tracker.CategoryStarted
	CategoryCompleted = tracker.CategoryCompleted
	CategoryCanceled  = tracker.CategoryCanceled
)

// StatusCategories lists the valid IssueStatus.Category values in lifecycle order.
var StatusCategories = tracker.StatusCategories

// ValidCategory reports whether c is one of StatusCategories.
func ValidCategory(c string) bool { return tracker.ValidCategory(c) }

type (
	IssueStatus      = tracker.IssueStatus
	SDDRef           = tracker.SDDRef
	IssueLink        = tracker.IssueLink
	Issue            = tracker.Issue
	IssueFilter      = tracker.IssueFilter
	CreateIssueInput = tracker.CreateIssueInput
	UpdateIssueInput = tracker.UpdateIssueInput
	TrackerCaps      = tracker.TrackerCaps
	IssueTracker     = tracker.IssueTracker
	CheckResult      = tracker.CheckResult
)

// Tracker operation names (sdd.adapter/v1, kind tracker).
const (
	OpCapabilities    = tracker.OpCapabilities
	OpListIssues      = tracker.OpListIssues
	OpGetIssue        = tracker.OpGetIssue
	OpCreateIssue     = tracker.OpCreateIssue
	OpUpdateIssue     = tracker.OpUpdateIssue
	OpTransitionIssue = tracker.OpTransitionIssue
	OpComment         = tracker.OpComment
)

// NewNonce returns random hex for one create; retries reuse it.
func NewNonce() string { return tracker.NewNonce() }

// NewClientRef is the idempotency key of one create.
func NewClientRef(repo, change, title, nonce string) string {
	return tracker.NewClientRef(repo, change, title, nonce)
}

func canonicalJSON(raw []byte) ([]byte, error) { return tracker.CanonicalJSON(raw) }

var hostTracker IssueTracker

// SetIssueTracker installs a host tracker that replaces the backend named in
// .sdd.yaml. Call it once, before serving. Unstable until v1.
func SetIssueTracker(t IssueTracker) { hostTracker = t }
