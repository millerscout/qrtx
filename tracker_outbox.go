package sdd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OutboxEntry is one tracker write waiting to be sent. Entries live in the
// shared state directory and leave it only after the backend confirms them.
type OutboxEntry struct {
	Seq       int             `json:"seq"`
	Key       string          `json:"key"`
	RepoRoot  string          `json:"repo_root"`
	Op        string          `json:"op"`
	Backend   string          `json:"backend"`
	IssueID   string          `json:"issue_id,omitempty"`
	Args      json.RawMessage `json:"args"`
	ClientRef string          `json:"client_ref,omitempty"`
	Attempts  int             `json:"attempts"`
	LastError string          `json:"last_error,omitempty"`
	Failed    bool            `json:"failed,omitempty"` // a non-retryable error; drop it after reading LastError
	QueuedAt  string          `json:"queued_at"`
}

// DrainReport says what one drain did.
type DrainReport struct {
	Sent     int
	Pending  int
	Warnings []string
}

func canonicalRoot(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	return abs
}

func outboxPath(repoRoot string) string {
	dir, _ := trackerStateDir(repoRoot)
	return filepath.Join(dir, "outbox.jsonl")
}

func readOutbox(path string) ([]OutboxEntry, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []OutboxEntry
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	n := 0
	for sc.Scan() {
		n++
		text := bytes.TrimSpace(sc.Bytes())
		if len(text) == 0 {
			continue
		}
		var e OutboxEntry
		if err := json.Unmarshal(text, &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, n, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func writeOutbox(path string, entries []OutboxEntry) error {
	var buf bytes.Buffer
	for _, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// queueEntry builds an entry for this root; args is marshalled now.
func queueEntry(repoRoot, key, op, backend, issueID string, args any) (OutboxEntry, error) {
	b, err := json.Marshal(args)
	if err != nil {
		return OutboxEntry{}, err
	}
	return OutboxEntry{Key: key, RepoRoot: canonicalRoot(repoRoot), Op: op, Backend: backend, IssueID: issueID, Args: b,
		QueuedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

// appendOutbox adds entries whose key is not already queued for this root
// and returns the sequence numbers it added.
func appendOutbox(repoRoot string, entries []OutboxEntry) ([]int, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	var added []int
	err := withStateLock(repoRoot, "outbox", func() error {
		path := outboxPath(repoRoot)
		have, err := readOutbox(path)
		if err != nil {
			return err
		}
		maxSeq := 0
		queued := map[string]bool{}
		for _, e := range have {
			if e.Seq > maxSeq {
				maxSeq = e.Seq
			}
			queued[e.RepoRoot+"\x00"+e.Key] = true
		}
		for _, e := range entries {
			if queued[e.RepoRoot+"\x00"+e.Key] {
				continue
			}
			maxSeq++
			e.Seq = maxSeq
			queued[e.RepoRoot+"\x00"+e.Key] = true
			have = append(have, e)
			added = append(added, e.Seq)
		}
		if len(added) == 0 {
			return nil
		}
		return writeOutbox(path, have)
	})
	return added, err
}

// removeOutbox deletes the given sequence numbers.
func removeOutbox(repoRoot string, seqs []int) error {
	if len(seqs) == 0 {
		return nil
	}
	drop := map[int]bool{}
	for _, s := range seqs {
		drop[s] = true
	}
	return withStateLock(repoRoot, "outbox", func() error {
		path := outboxPath(repoRoot)
		have, err := readOutbox(path)
		if err != nil {
			return err
		}
		var keep []OutboxEntry
		for _, e := range have {
			if !drop[e.Seq] {
				keep = append(keep, e)
			}
		}
		return writeOutbox(path, keep)
	})
}

// ListOutbox returns this root's pending entries without creating anything.
func ListOutbox(repoRoot string) ([]OutboxEntry, error) {
	have, err := readOutbox(outboxPath(repoRoot))
	if err != nil {
		return nil, err
	}
	me := canonicalRoot(repoRoot)
	out := []OutboxEntry{}
	for _, e := range have {
		if e.RepoRoot == me {
			out = append(out, e)
		}
	}
	return out, nil
}

// DropOutboxEntry removes one of this root's entries by sequence number.
func DropOutboxEntry(repoRoot string, seq int) error {
	list, err := ListOutbox(repoRoot)
	if err != nil {
		return err
	}
	for _, e := range list {
		if e.Seq == seq {
			return removeOutbox(repoRoot, []int{seq})
		}
	}
	return fmt.Errorf("outbox has no entry %d for this repository", seq)
}

// DrainOutbox sends this root's pending writes in order. An entry leaves
// the outbox only when the backend confirms it. A retryable failure keeps
// it for the next drain; later writes to the same issue wait behind it.
// With no outbox file it does nothing and creates nothing.
func DrainOutbox(repoRoot string, t *Tracker) DrainReport {
	var rep DrainReport
	path := outboxPath(repoRoot)
	if _, err := os.Stat(path); err != nil {
		return rep
	}
	err := withStateLock(repoRoot, "outbox", func() error {
		have, err := readOutbox(path)
		if err != nil {
			return err
		}
		me := canonicalRoot(repoRoot)
		blocked := map[string]bool{}
		var keep []OutboxEntry
		for _, e := range have {
			if e.RepoRoot != me {
				keep = append(keep, e)
				continue
			}
			issueKey := e.Backend + ":" + e.IssueID
			if e.Failed || (e.IssueID != "" && blocked[issueKey]) {
				keep = append(keep, e)
				rep.Pending++
				continue
			}
			err := sendEntry(t, e)
			if err == nil {
				rep.Sent++
				continue
			}
			if errors.Is(err, errDropEntry) {
				continue
			}
			e.Attempts++
			e.LastError = err.Error()
			code := adapterCode(err)
			if code != "" && !(&AdapterError{Code: code}).retryable() {
				e.Failed = true
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("outbox %d (%s %s) failed and will not be retried: %v; drop it with `%s tracker outbox --drop %d`", e.Seq, e.Op, issueKey, err, CLIName, e.Seq))
			}
			if e.IssueID != "" {
				blocked[issueKey] = true
			}
			keep = append(keep, e)
			rep.Pending++
		}
		return writeOutbox(path, keep)
	})
	if err != nil {
		rep.Warnings = append(rep.Warnings, "outbox: "+err.Error())
	}
	return rep
}

var errDropEntry = errors.New("entry dropped")

type outboxUpdateArgs struct {
	Input UpdateIssueInput `json:"input"`
}

type outboxTransitionArgs struct {
	Category string `json:"category"`
	Native   string `json:"native,omitempty"`
}

type outboxCommentArgs struct {
	Markdown string `json:"markdown"`
}

func sendEntry(t *Tracker, e OutboxEntry) error {
	impl, err := t.For(e.Backend)
	if err != nil {
		return err
	}
	switch e.Op {
	case OpCreateIssue:
		var in CreateIssueInput
		if err := json.Unmarshal(e.Args, &in); err != nil {
			return &AdapterError{Code: ErrCodeInvalid, Message: err.Error()}
		}
		_, err := impl.CreateIssue(in)
		return err
	case OpUpdateIssue:
		var a outboxUpdateArgs
		if err := json.Unmarshal(e.Args, &a); err != nil {
			return &AdapterError{Code: ErrCodeInvalid, Message: err.Error()}
		}
		_, err := impl.UpdateIssue(e.IssueID, a.Input)
		return err
	case OpTransitionIssue:
		var a outboxTransitionArgs
		if err := json.Unmarshal(e.Args, &a); err != nil {
			return &AdapterError{Code: ErrCodeInvalid, Message: err.Error()}
		}
		_, err := impl.TransitionIssue(e.IssueID, a.Category, a.Native)
		return err
	case OpComment:
		caps, err := impl.Capabilities()
		if err != nil {
			return err
		}
		if !caps.Has(OpComment) {
			return errDropEntry
		}
		var a outboxCommentArgs
		if err := json.Unmarshal(e.Args, &a); err != nil {
			return &AdapterError{Code: ErrCodeInvalid, Message: err.Error()}
		}
		return impl.Comment(e.IssueID, a.Markdown, e.ClientRef)
	default:
		return &AdapterError{Code: ErrCodeInvalid, Message: "unknown outbox op " + e.Op}
	}
}
