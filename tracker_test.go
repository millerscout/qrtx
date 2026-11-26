package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueRoundTrip(t *testing.T) {
	line := `{"client_ref":"abc","extra":{"jira":{"customfield_1":"x","z":[1,2]}},"id":"sdd-aaaaaa","rank":3,"status":{"category":"started","name":"Doing"},"title":"T"}`
	var is Issue
	if err := json.Unmarshal([]byte(line), &is); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(is)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != line {
		t.Fatalf("round trip changed bytes:\n%s\n%s", line, out)
	}
	is.Status = IssueStatus{Category: CategoryCompleted}
	out, _ = json.Marshal(is)
	for _, want := range []string{`"extra":{"jira":{"customfield_1":"x","z":[1,2]}}`, `"rank":3`, `"category":"completed"`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("closed issue lost %s: %s", want, out)
		}
	}
}

func TestAdapterEnvelope(t *testing.T) {
	for _, bad := range []string{"../evil", "a/b", "", "Upper", "x y"} {
		if ValidBackendName(bad) {
			t.Fatalf("%q should be invalid", bad)
		}
	}
	for _, good := range []string{"file", "github", "azure-devops", "x2"} {
		if !ValidBackendName(good) {
			t.Fatalf("%q should be valid", good)
		}
	}
	req := AdapterRequest{Protocol: AdapterProtocol, Kind: "tracker", Op: OpGetIssue, Args: json.RawMessage(`{"id":"1"}`)}
	b, _ := json.Marshal(req)
	var back AdapterRequest
	if err := json.Unmarshal(b, &back); err != nil || back.Op != OpGetIssue || string(back.Args) != `{"id":"1"}` {
		t.Fatalf("envelope round trip: %s %v", b, err)
	}
}

// schemaChecks is a hand-rolled subset of JSON Schema: required keys and enums.
func schemaRequired(t *testing.T, file string) (required []string, props map[string]any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("docs", "schema", "tracker", "v1", file))
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	for _, r := range s["required"].([]any) {
		required = append(required, r.(string))
	}
	props, _ = s["properties"].(map[string]any)
	return required, props
}

func enumOf(prop any) []string {
	m, _ := prop.(map[string]any)
	var out []string
	for _, v := range m["enum"].([]any) {
		out = append(out, v.(string))
	}
	return out
}

func checkAgainst(t *testing.T, file string, doc []byte) map[string]any {
	t.Helper()
	req, props := schemaRequired(t, file)
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range req {
		if _, ok := m[k]; !ok {
			t.Fatalf("%s: %s missing required %q", file, doc, k)
		}
	}
	for k := range m {
		if _, ok := props[k]; !ok && file != "issue.json" {
			t.Fatalf("%s: unexpected key %q", file, k)
		}
	}
	return m
}

func TestTrackerSchema(t *testing.T) {
	_, props := schemaRequired(t, "issue.json")
	status := props["status"].(map[string]any)["properties"].(map[string]any)
	if got := strings.Join(enumOf(status["category"]), ","); got != strings.Join(StatusCategories, ",") {
		t.Fatalf("category enum %s != %v", got, StatusCategories)
	}
	refProps := props["sdd"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if got := enumOf(refProps["role"]); len(got) != 2 || got[1] != "deferred" {
		t.Fatalf("role enum %v", got)
	}
	if _, ok := props["client_ref"]; !ok {
		t.Fatal("issue.json lacks client_ref")
	}
	p := 1
	full := Issue{ID: "PROJ-1", URL: "u", Title: "T", Description: "d", Status: IssueStatus{Name: "To Do", Category: CategoryUnstarted},
		Type: "Bug", Priority: &p, Labels: []string{"a"}, Assignee: "me", Parent: "PROJ-0",
		Links: []IssueLink{{Type: "blocks", ID: "PROJ-2"}}, UpdatedAt: "2026-11-26T00:00:00Z",
		Extra: map[string]json.RawMessage{"k": json.RawMessage(`1`)}, Refs: []SDDRef{{Repo: "r", Change: "c", Role: "deferred"}}, ClientRef: "x"}
	for _, is := range []Issue{full, {ID: "1", Title: "min", Status: IssueStatus{Category: CategoryBacklog}}} {
		b, err := json.Marshal(is)
		if err != nil {
			t.Fatal(err)
		}
		m := checkAgainst(t, "issue.json", b)
		for k := range m {
			if _, ok := props[k]; !ok {
				t.Fatalf("Issue key %q is not in issue.json", k)
			}
		}
		if err := is.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := (Issue{ID: "1", Title: "x", Status: IssueStatus{Category: "done"}}).Validate(); err == nil {
		t.Fatal("bad category accepted")
	}
	b, _ := json.Marshal(AdapterRequest{Protocol: AdapterProtocol, Kind: "tracker", Op: OpListIssues, Args: json.RawMessage(`{}`)})
	m := checkAgainst(t, "request.json", b)
	_, rprops := schemaRequired(t, "request.json")
	ops := enumOf(rprops["op"])
	for _, op := range []string{OpCapabilities, OpListIssues, OpGetIssue, OpCreateIssue, OpUpdateIssue, OpTransitionIssue, OpComment} {
		if !strings.Contains(strings.Join(ops, ","), op) {
			t.Fatalf("request.json lacks op %s", op)
		}
	}
	if m["protocol"] != AdapterProtocol {
		t.Fatal("protocol")
	}
	eb, _ := json.Marshal(AdapterResponse{Error: &AdapterError{Code: ErrCodeAuth, Message: "no token"}})
	raw, _ := os.ReadFile(filepath.Join("docs", "schema", "tracker", "v1", "response.json"))
	var rs map[string]any
	_ = json.Unmarshal(raw, &rs)
	codes := enumOf(rs["properties"].(map[string]any)["error"].(map[string]any)["properties"].(map[string]any)["code"])
	for _, c := range []string{ErrCodeNotFound, ErrCodeUnsupported, ErrCodeAuth, ErrCodeInvalid, ErrCodeInvalidTransition, ErrCodeRateLimited, ErrCodeUnavailable} {
		if !strings.Contains(strings.Join(codes, ","), c) {
			t.Fatalf("response.json lacks code %s", c)
		}
	}
	if !strings.Contains(string(eb), `"code":"auth"`) || strings.Contains(string(eb), `"result"`) {
		t.Fatalf("error response %s", eb)
	}
}

func TestTrackerConfig(t *testing.T) {
	root := t.TempDir()
	if s, _, err := loadTrackerSettings(root); err != nil || s.Backend != "file" || s.Comments != "summary" || s.Timeout != 30 {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	if err := os.WriteFile(filepath.Join(root, RepoConfigFile), []byte("tracker:\n  mirror: tasks\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRepoConfig(root); err != nil {
		t.Fatalf("LoadRepoConfig must not validate the tracker block: %v", err)
	}
	if _, _, err := loadTrackerSettings(root); err == nil || !strings.Contains(err.Error(), "only change is implemented") {
		t.Fatalf("mirror tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, RepoConfigFile), []byte("tracker:\n  backend: ../evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadTrackerSettings(root); err == nil || !strings.Contains(err.Error(), "../evil") {
		t.Fatalf("bad backend: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, RepoConfigFile), []byte("tracker:\n  comments: loud\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadTrackerSettings(root); err == nil {
		t.Fatal("comments loud accepted")
	}
}
