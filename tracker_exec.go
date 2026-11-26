package sdd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// execTracker runs sdd-tracker-<name> from PATH, one process per operation.
type execTracker struct {
	name    string
	config  json.RawMessage
	timeout time.Duration
	stderr  io.Writer
	caps    *TrackerCaps
}

func newExecTracker(name string, config map[string]any, timeout time.Duration) (*execTracker, error) {
	if err := checkBackendName(name); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if len(config) > 0 {
		b, err := json.Marshal(config)
		if err != nil {
			return nil, fmt.Errorf("tracker.config: %w", err)
		}
		raw = b
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &execTracker{name: name, config: raw, timeout: timeout, stderr: os.Stderr}, nil
}

func (e *execTracker) bin() string { return "sdd-tracker-" + e.name }

// TrackerCallError names the backend and operation of a failed call. It
// wraps an *AdapterError when the plugin (or the transport) reported one.
type TrackerCallError struct {
	Backend string
	Op      string
	Err     error
}

func (e *TrackerCallError) Error() string {
	return fmt.Sprintf("tracker %s %s: %v", e.Backend, e.Op, e.Err)
}

func (e *TrackerCallError) Unwrap() error { return e.Err }

func (e *execTracker) fail(op string, err error) error {
	return &TrackerCallError{Backend: e.name, Op: op, Err: err}
}

func (e *execTracker) run(op string, args any) (json.RawMessage, error) {
	path, err := exec.LookPath(e.bin())
	if err != nil {
		return nil, e.fail(op, &AdapterError{Code: ErrCodeUnavailable, Message: e.bin() + " not found on PATH"})
	}
	var argRaw json.RawMessage
	if args != nil {
		if argRaw, err = json.Marshal(args); err != nil {
			return nil, e.fail(op, err)
		}
	}
	req, err := json.Marshal(AdapterRequest{Protocol: AdapterProtocol, Kind: "tracker", Op: op, Config: e.config, Args: argRaw})
	if err != nil {
		return nil, e.fail(op, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, op)
	cmd.Stdin = bytes.NewReader(req)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = e.stderr
	cmd.Env = append(os.Environ(), "SDD_TRACKER_DEADLINE="+time.Now().Add(e.timeout).UTC().Format(time.RFC3339Nano))
	cmd.WaitDelay = time.Second
	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, e.fail(op, &AdapterError{Code: ErrCodeUnavailable, Message: fmt.Sprintf("timed out after %s", e.timeout)})
	}
	var resp AdapterResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		msg := "no JSON response"
		if out.Len() > 0 {
			msg = "non-JSON output: " + truncateRunes(strings.TrimSpace(out.String()), 80)
		}
		if runErr != nil {
			msg += " (" + runErr.Error() + ")"
		}
		return nil, e.fail(op, &AdapterError{Code: ErrCodeUnavailable, Message: msg})
	}
	if resp.Error != nil {
		if resp.Error.Code == "" {
			resp.Error.Code = ErrCodeUnavailable
		}
		return nil, e.fail(op, resp.Error)
	}
	if runErr != nil {
		return nil, e.fail(op, &AdapterError{Code: ErrCodeUnavailable, Message: runErr.Error()})
	}
	if resp.Result == nil {
		return nil, e.fail(op, &AdapterError{Code: ErrCodeUnavailable, Message: "response has neither result nor error"})
	}
	return resp.Result, nil
}

// call checks capabilities (once per process) before any other operation.
func (e *execTracker) call(op string, args any, out any) error {
	caps, err := e.Capabilities()
	if err != nil {
		return err
	}
	if !caps.Has(op) {
		return e.fail(op, &AdapterError{Code: ErrCodeUnsupported, Message: "not declared in capabilities"})
	}
	raw, err := e.run(op, args)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return e.fail(op, &AdapterError{Code: ErrCodeUnavailable, Message: "malformed result: " + err.Error()})
	}
	return nil
}

func (e *execTracker) Capabilities() (TrackerCaps, error) {
	if e.caps != nil {
		return *e.caps, nil
	}
	raw, err := e.run(OpCapabilities, struct{}{})
	if err != nil {
		return TrackerCaps{}, err
	}
	var caps TrackerCaps
	if err := json.Unmarshal(raw, &caps); err != nil {
		return TrackerCaps{}, e.fail(OpCapabilities, &AdapterError{Code: ErrCodeUnavailable, Message: "malformed capabilities: " + err.Error()})
	}
	if !containsString(caps.Protocols, AdapterProtocol) {
		return TrackerCaps{}, e.fail(OpCapabilities, &AdapterError{Code: ErrCodeUnsupported,
			Message: fmt.Sprintf("plugin speaks %v, sdd speaks [%s]; no shared protocol version", caps.Protocols, AdapterProtocol)})
	}
	e.caps = &caps
	return caps, nil
}

func (e *execTracker) checkIssue(op string, is Issue) (Issue, error) {
	if err := is.Validate(); err != nil {
		return Issue{}, e.fail(op, &AdapterError{Code: ErrCodeUnavailable, Message: "invalid issue in result: " + err.Error()})
	}
	return is, nil
}

func (e *execTracker) ListIssues(f IssueFilter) ([]Issue, error) {
	var all []Issue
	for page := 0; page < 50; page++ {
		var res struct {
			Issues []Issue `json:"issues"`
			Cursor string  `json:"cursor"`
		}
		if err := e.call(OpListIssues, map[string]any{"filter": f}, &res); err != nil {
			return nil, err
		}
		for _, is := range res.Issues {
			if _, err := e.checkIssue(OpListIssues, is); err != nil {
				return nil, err
			}
		}
		all = append(all, res.Issues...)
		if res.Cursor == "" || (f.Limit > 0 && len(all) >= f.Limit) {
			break
		}
		f.Cursor = res.Cursor
	}
	if f.Limit > 0 && len(all) > f.Limit {
		all = all[:f.Limit]
	}
	if all == nil {
		all = []Issue{}
	}
	return all, nil
}

func (e *execTracker) GetIssue(id string) (Issue, error) {
	var is Issue
	if err := e.call(OpGetIssue, map[string]any{"id": id}, &is); err != nil {
		return Issue{}, err
	}
	return e.checkIssue(OpGetIssue, is)
}

func (e *execTracker) CreateIssue(in CreateIssueInput) (Issue, error) {
	var is Issue
	if err := e.call(OpCreateIssue, in, &is); err != nil {
		return Issue{}, err
	}
	return e.checkIssue(OpCreateIssue, is)
}

func (e *execTracker) UpdateIssue(id string, in UpdateIssueInput) (Issue, error) {
	args := map[string]any{"id": id}
	if in.Title != nil {
		args["title"] = *in.Title
	}
	if in.Description != nil {
		args["description"] = *in.Description
	}
	if in.Labels != nil {
		args["labels"] = in.Labels
	}
	if len(in.AddRefs) > 0 {
		args["add_sdd"] = in.AddRefs
	}
	var is Issue
	if err := e.call(OpUpdateIssue, args, &is); err != nil {
		return Issue{}, err
	}
	return e.checkIssue(OpUpdateIssue, is)
}

func (e *execTracker) TransitionIssue(id, category, native string) (Issue, error) {
	args := map[string]any{"id": id, "category": category}
	if native != "" {
		args["native"] = native
	}
	var is Issue
	if err := e.call(OpTransitionIssue, args, &is); err != nil {
		return Issue{}, err
	}
	return e.checkIssue(OpTransitionIssue, is)
}

func (e *execTracker) Comment(id, markdown, clientRef string) error {
	return e.call(OpComment, map[string]any{"id": id, "markdown": markdown, "client_ref": clientRef}, nil)
}

// adapterCode returns the adapter error code inside err, or "".
func adapterCode(err error) string {
	var ae *AdapterError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}
