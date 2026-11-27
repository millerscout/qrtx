package qrtx

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as two helpers, chosen by the name it runs under:
//
//	qrtx-tracker-fake  a tracker plugin whose behaviour comes from files
//	sdd-child         a qrtx process the outbox tests can kill
func runTestHelper() bool {
	switch filepath.Base(os.Args[0]) {
	case "qrtx-tracker-fake":
		os.Exit(fakeTrackerMain())
	case "sdd-child":
		os.Exit(childMain())
	}
	return false
}

type fakeState struct {
	Issues   []Issue        `json:"issues"`
	Comments map[string]int `json:"comments"`
}

func fakeDir() string { return os.Getenv("QRTX_FAKE_DIR") }

func fakeMode() string {
	b, _ := os.ReadFile(filepath.Join(fakeDir(), "mode"))
	return strings.TrimSpace(string(b))
}

func loadFake() fakeState {
	var s fakeState
	b, _ := os.ReadFile(filepath.Join(fakeDir(), "state.json"))
	_ = json.Unmarshal(b, &s)
	if s.Comments == nil {
		s.Comments = map[string]int{}
	}
	return s
}

func saveFake(s fakeState) {
	b, _ := json.Marshal(s)
	_ = os.WriteFile(filepath.Join(fakeDir(), "state.json"), b, 0o644)
}

func fakeReply(result any, errCode string) int {
	if errCode != "" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"error": map[string]string{"code": errCode, "message": "fake " + errCode}})
		return 1
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"result": result})
	return 0
}

func fakeTrackerMain() int {
	raw, _ := io.ReadAll(os.Stdin)
	var req struct {
		Op   string          `json:"op"`
		Args json.RawMessage `json:"args"`
	}
	_ = json.Unmarshal(raw, &req)
	f, _ := os.OpenFile(filepath.Join(fakeDir(), "log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	fmt.Fprintln(f, req.Op)
	f.Close()
	mode := fakeMode()
	_ = os.WriteFile(filepath.Join(fakeDir(), "deadline"), []byte(os.Getenv("QRTX_TRACKER_DEADLINE")), 0o644)
	switch mode {
	case "down":
		return fakeReply(nil, ErrCodeUnavailable)
	case "auth":
		return fakeReply(nil, ErrCodeAuth)
	case "nonjson":
		fmt.Println("hello, not json")
		return 0
	case "sleep":
		time.Sleep(10 * time.Second)
	case "stall":
		// A plugin whose request hangs until its own deadline, then reports.
		if d, err := time.Parse(time.RFC3339Nano, os.Getenv("QRTX_TRACKER_DEADLINE")); err == nil {
			time.Sleep(time.Until(d))
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"error": map[string]any{"code": ErrCodeUnavailable, "message": "GET /issues: context deadline exceeded"}})
		return 0
	}
	if req.Op == OpCapabilities {
		caps := TrackerCaps{Protocols: []string{AdapterProtocol}, Ops: []string{OpCapabilities, OpListIssues, OpGetIssue, OpCreateIssue, OpUpdateIssue, OpTransitionIssue, OpComment}}
		if mode == "v2" {
			caps.Protocols = []string{"sdd.adapter/v2"}
		}
		if mode == "nocomment" {
			caps.Ops = []string{OpCapabilities, OpListIssues, OpGetIssue, OpCreateIssue, OpUpdateIssue, OpTransitionIssue}
		}
		if mode == "nolist" {
			caps.Ops = []string{OpCapabilities, OpGetIssue, OpCreateIssue, OpUpdateIssue, OpTransitionIssue, OpComment}
		}
		return fakeReply(caps, "")
	}
	s := loadFake()
	find := func(id string) int {
		for i, is := range s.Issues {
			if is.ID == id {
				return i
			}
		}
		return -1
	}
	var a map[string]json.RawMessage
	_ = json.Unmarshal(req.Args, &a)
	str := func(k string) string { var v string; _ = json.Unmarshal(a[k], &v); return v }
	switch req.Op {
	case OpListIssues:
		if mode == "endless" {
			// A listing that never ends: every page has a next cursor.
			return fakeReply(map[string]any{"issues": []Issue{}, "cursor": "more"}, "")
		}
		return fakeReply(map[string]any{"issues": s.Issues}, "")
	case OpGetIssue:
		i := find(str("id"))
		if i < 0 {
			return fakeReply(nil, ErrCodeNotFound)
		}
		return fakeReply(s.Issues[i], "")
	case OpCreateIssue:
		var in CreateIssueInput
		_ = json.Unmarshal(req.Args, &in)
		for _, is := range s.Issues {
			if mode != "nodedupe" && in.ClientRef != "" && is.ClientRef == in.ClientRef {
				return fakeReply(is, "")
			}
		}
		cat := in.Category
		if cat == "" {
			cat = CategoryBacklog
		}
		is := Issue{ID: fmt.Sprintf("F-%d", len(s.Issues)+1), Title: in.Title, Description: in.Description, Status: IssueStatus{Category: cat}, Refs: in.Refs, ClientRef: in.ClientRef, Priority: in.Priority}
		s.Issues = append(s.Issues, is)
		saveFake(s)
		if mode == "block" {
			_ = os.WriteFile(filepath.Join(fakeDir(), "signal"), []byte("created"), 0o644)
			time.Sleep(time.Minute)
		}
		return fakeReply(is, "")
	case OpUpdateIssue:
		i := find(str("id"))
		if i < 0 {
			return fakeReply(nil, ErrCodeNotFound)
		}
		var refs []SDDRef
		_ = json.Unmarshal(a["add_sdd"], &refs)
		s.Issues[i].AddRefs(refs)
		if mode != "droplinks" {
			var add, rm []IssueLink
			_ = json.Unmarshal(a["add_links"], &add)
			_ = json.Unmarshal(a["remove_links"], &rm)
			s.Issues[i].AddLinks(add)
			s.Issues[i].RemoveLinks(rm)
			var pr *int
			if json.Unmarshal(a["priority"], &pr) == nil {
				s.Issues[i].SetPriority(pr)
			}
		}
		if t := str("title"); t != "" {
			s.Issues[i].Title = t
		}
		saveFake(s)
		return fakeReply(s.Issues[i], "")
	case OpTransitionIssue:
		i := find(str("id"))
		if i < 0 {
			return fakeReply(nil, ErrCodeNotFound)
		}
		s.Issues[i].Status = IssueStatus{Name: str("native"), Category: str("category")}
		saveFake(s)
		return fakeReply(s.Issues[i], "")
	case OpComment:
		s.Comments[str("id")]++
		saveFake(s)
		return fakeReply(map[string]any{}, "")
	}
	return fakeReply(nil, ErrCodeUnsupported)
}

// childMain runs `from-deferred` for the outbox crash test.
func childMain() int {
	root := os.Getenv(EnvRepo)
	t, err := OpenTracker(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := QueueDeferredIssues(root, t, os.Getenv("QRTX_CHILD_SLUG")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	DrainOutbox(root, t)
	return 0
}

// installHelper links the test binary into a temp bin dir under name and
// puts that dir first on PATH.
func installHelper(t *testing.T, names ...string) string {
	t.Helper()
	bin := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.Symlink(exe, filepath.Join(bin, n)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
}

// fakeRepo is a repo whose .sdd.yaml selects the fake plugin.
func fakeRepo(t *testing.T, extraYAML string) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = t.TempDir()
	t.Setenv("QRTX_FAKE_DIR", dir)
	installHelper(t, "qrtx-tracker-fake", "sdd-child")
	if err := os.WriteFile(filepath.Join(root, RepoConfigFile), []byte("tracker:\n  backend: fake\n  timeout: 2\n"+extraYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

func setFakeMode(t *testing.T, dir, mode string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "mode"), []byte(mode), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fakeLog(dir string) []string {
	b, _ := os.ReadFile(filepath.Join(dir, "log"))
	return strings.Fields(string(b))
}

func seedFake(t *testing.T, dir string, issues ...Issue) {
	t.Helper()
	saveFake(fakeState{Issues: issues})
}
