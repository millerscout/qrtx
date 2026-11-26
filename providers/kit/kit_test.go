package kit_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/millerscout/sdd/providers/kit"
	"github.com/millerscout/sdd/providers/kit/kittest"
	"github.com/millerscout/sdd/tracker"
)

// mem is an in-memory provider for the kit's own tests.
type mem struct {
	issues     []tracker.Issue
	noDedupe   bool
	initErr    error
	commentErr error
}

func (m *mem) Init(json.RawMessage) error { return m.initErr }
func (m *mem) Capabilities() (tracker.TrackerCaps, error) {
	return tracker.TrackerCaps{Protocols: []string{tracker.AdapterProtocol}, Ops: []string{"capabilities", "list_issues", "get_issue", "create_issue", "update_issue", "transition_issue", "comment"}}, nil
}
func (m *mem) ListIssuesPage(f tracker.IssueFilter) ([]tracker.Issue, string, error) {
	return m.issues, "", nil
}
func (m *mem) find(id string) (*tracker.Issue, error) {
	for i := range m.issues {
		if m.issues[i].ID == id {
			return &m.issues[i], nil
		}
	}
	return nil, &tracker.AdapterError{Code: tracker.ErrCodeNotFound, Message: id}
}
func (m *mem) GetIssue(id string) (tracker.Issue, error) {
	is, err := m.find(id)
	if err != nil {
		return tracker.Issue{}, err
	}
	return *is, nil
}
func (m *mem) CreateIssue(in tracker.CreateIssueInput) (tracker.Issue, error) {
	for _, is := range m.issues {
		if !m.noDedupe && is.ClientRef == in.ClientRef {
			return is, nil
		}
	}
	is := tracker.Issue{ID: fmt.Sprint(len(m.issues) + 1), Title: in.Title, Status: tracker.IssueStatus{Category: tracker.CategoryUnstarted}, ClientRef: in.ClientRef}
	m.issues = append(m.issues, is)
	return is, nil
}
func (m *mem) UpdateIssue(id string, in tracker.UpdateIssueInput) (tracker.Issue, error) {
	is, err := m.find(id)
	if err != nil {
		return tracker.Issue{}, err
	}
	if in.Title != nil {
		is.Title = *in.Title
	}
	is.AddRefs(in.AddRefs)
	return *is, nil
}
func (m *mem) TransitionIssue(id, cat, native string) (tracker.Issue, error) {
	is, err := m.find(id)
	if err != nil {
		return tracker.Issue{}, err
	}
	is.Status = tracker.IssueStatus{Category: cat}
	return *is, nil
}
func (m *mem) Comment(id, md, ref string) error { return m.commentErr }

func serve(t *testing.T, kinds map[string]kit.Handler, req string) (int, tracker.AdapterResponse) {
	t.Helper()
	var out bytes.Buffer
	code := kit.Serve(kinds, strings.NewReader(req), &out)
	var resp tracker.AdapterResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("response %q: %v", out.String(), err)
	}
	return code, resp
}

func TestServe(t *testing.T) {
	m := &mem{commentErr: errors.New("plain failure")}
	kinds := kit.Tracker(m)
	code, resp := serve(t, kinds, `{"protocol":"sdd.adapter/v2","kind":"tracker","op":"capabilities"}`)
	if code != 1 || resp.Error.Code != "invalid" || !strings.Contains(resp.Error.Message, "sdd.adapter/v2") {
		t.Fatalf("v2: %d %+v", code, resp.Error)
	}
	code, resp = serve(t, kinds, `{"protocol":"sdd.adapter/v1","kind":"wiki","op":"get"}`)
	if code != 1 || resp.Error.Code != "invalid" || !strings.Contains(resp.Error.Message, "wiki") {
		t.Fatalf("wiki to tracker-only: %d %+v", code, resp.Error)
	}
	code, resp = serve(t, kinds, `{"protocol":"sdd.adapter/v1","kind":"tracker","op":"create_issue","args":{"title":"x","client_ref":"r"}}`)
	if code != 0 || resp.Error != nil || !strings.Contains(string(resp.Result), `"client_ref":"r"`) {
		t.Fatalf("create: %d %s %+v", code, resp.Result, resp.Error)
	}
	code, resp = serve(t, kinds, `{"protocol":"sdd.adapter/v1","kind":"tracker","op":"list_issues","args":{"filter":{}}}`)
	if code != 0 || !strings.Contains(string(resp.Result), `"issues":[`) || !strings.Contains(string(resp.Result), `"cursor":""`) {
		t.Fatalf("list: %s", resp.Result)
	}
	code, resp = serve(t, kinds, `{"protocol":"sdd.adapter/v1","kind":"tracker","op":"comment","args":{"id":"1","markdown":"m","client_ref":"c"}}`)
	if code != 1 || resp.Error.Code != "invalid" || resp.Error.Message != "plain failure" {
		t.Fatalf("plain error: %+v", resp.Error)
	}
	code, resp = serve(t, kinds, `{"protocol":"sdd.adapter/v1","kind":"tracker","op":"get_issue","args":{"id":"404"}}`)
	if code != 1 || resp.Error.Code != "not_found" {
		t.Fatalf("adapter error kept: %+v", resp.Error)
	}
	wiki := map[string]kit.Handler{"wiki": kit.HandlerFunc(func(op string, c, a json.RawMessage) (any, error) {
		return map[string]string{"op": op}, nil
	})}
	code, resp = serve(t, wiki, `{"protocol":"sdd.adapter/v1","kind":"wiki","op":"get_page"}`)
	if code != 0 || string(resp.Result) != `{"op":"get_page"}` {
		t.Fatalf("wiki handler: %d %s", code, resp.Result)
	}
	if code, _ = serve(t, wiki, `{"protocol":"sdd.adapter/v1","kind":"tracker","op":"capabilities"}`); code != 1 {
		t.Fatal("tracker envelope to a wiki-only plugin must fail")
	}
}

func writeCreds(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "credentials.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SDD_CREDENTIALS", p)
	return p
}

func code(err error) string {
	var ae *tracker.AdapterError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func TestCredentials(t *testing.T) {
	path := writeCreds(t, `hosts:
  api.github.com: {token_env: SDD_TEST_TOKEN, scopes: [o/r]}
  127.0.0.1:9999: {token_command: 'printf "%s|%s" "$SDD_CRED_HOST" "$SDD_CRED_SCOPE"', scopes: ["*"]}
  jira.example: {token_env: SDD_TEST_TOKEN, email_env: SDD_TEST_EMPTY, scopes: [SHOP]}
`)
	t.Setenv("SDD_TEST_TOKEN", "tok")
	t.Setenv("SDD_TEST_EMPTY", "")
	if c, err := kit.Credentials("api.github.com", "O/R"); err != nil || c.Token != "tok" {
		t.Fatalf("bound: %+v %v", c, err)
	}
	for _, tc := range []struct{ host, scope, want string }{
		{"evil.example", "o/r", "evil.example"},
		{"127.0.0.1:8888", "x", "127.0.0.1:8888"},
		{"api.github.com", "attacker/x", "attacker/x"},
		{"jira.example", "SHOP", "SDD_TEST_EMPTY"},
	} {
		_, err := kit.Credentials(tc.host, tc.scope)
		if code(err) != "auth" || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), path) {
			t.Fatalf("%s %s: %v", tc.host, tc.scope, err)
		}
	}
	if c, err := kit.Credentials("127.0.0.1:9999", "any/thing"); err != nil || c.Token != "127.0.0.1:9999|any/thing" {
		t.Fatalf("command: %+v %v", c, err)
	}
	writeCreds(t, "hosts:\n  api.github.com: {token: ghp_x, scopes: [o/r]}\n")
	if _, err := kit.Credentials("api.github.com", "o/r"); err == nil || !strings.Contains(err.Error(), "literal tokens are not allowed") {
		t.Fatalf("literal token: %v", err)
	}
}

func TestBindHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sdd", "credentials.yaml")
	changed, err := kit.BindHost(path, "api.github.com", kit.Binding{TokenCommand: "gh auth token", Scopes: []string{"o/r"}})
	if err != nil || !changed {
		t.Fatalf("first: %v %v", changed, err)
	}
	before, _ := os.ReadFile(path)
	if changed, err = kit.BindHost(path, "api.github.com", kit.Binding{TokenCommand: "gh auth token", Scopes: []string{"o/r"}}); err != nil || changed {
		t.Fatalf("re-run: %v %v", changed, err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("re-run rewrote the file")
	}
	if changed, _ = kit.BindHost(path, "api.github.com", kit.Binding{TokenCommand: "gh auth token", Scopes: []string{"o/other"}}); !changed {
		t.Fatal("second scope not merged")
	}
	binds, err := kit.LoadBindings(path)
	if err != nil || strings.Join(binds["api.github.com"].Scopes, ",") != "o/r,o/other" {
		t.Fatalf("scopes: %+v %v", binds, err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	if strings.Contains(string(before), "token:") {
		t.Fatalf("file holds a token key:\n%s", before)
	}
}

func bindServer(t *testing.T, srv *httptest.Server, scope string) string {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	writeCreds(t, fmt.Sprintf("hosts:\n  %s: {token_env: SDD_TEST_TOKEN, scopes: [%s]}\n", u.Host, scope))
	t.Setenv("SDD_TEST_TOKEN", "tok")
	return u.Host
}

func TestHTTP(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("User-Agent") != "sdd-test" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/flaky":
			if n == 1 {
				w.WriteHeader(503)
				return
			}
			fmt.Fprint(w, `{"ok":true}`)
		case "/limited":
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(429)
		case "/post":
			w.WriteHeader(503)
		case "/unauth":
			w.WriteHeader(401)
		}
	}))
	defer srv.Close()
	bindServer(t, srv, "o/r")
	c, err := kit.NewClient(srv.URL, "o/r", "sdd-test", kit.BearerAuth)
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ OK bool }
	if _, err := c.Do(kit.Request{Method: "GET", Path: "/flaky"}, &out); err != nil || !out.OK {
		t.Fatalf("503 then 200: %v", err)
	}
	t.Setenv("SDD_TRACKER_DEADLINE", time.Now().Add(3*time.Second).Format(time.RFC3339Nano))
	c, _ = kit.NewClient(srv.URL, "o/r", "sdd-test", kit.BearerAuth)
	start := time.Now()
	_, err = c.Do(kit.Request{Method: "GET", Path: "/limited"}, nil)
	var ae *tracker.AdapterError
	if !errors.As(err, &ae) || ae.Code != "rate_limited" || ae.RetryAfter != 7 || time.Since(start) > 2*time.Second {
		t.Fatalf("429 past deadline: %v after %v", err, time.Since(start))
	}
	atomic.StoreInt32(&hits, 0)
	if _, err := c.Do(kit.Request{Method: "POST", Path: "/post", Body: map[string]string{"a": "b"}}, nil); code(err) != "unavailable" || atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("POST retried: %v hits=%d", err, hits)
	}
	if _, err := c.Do(kit.Request{Method: "GET", Path: "/unauth"}, nil); code(err) != "auth" {
		t.Fatalf("401: %v", err)
	}
	// Host/port and scope mismatches never reach the server.
	atomic.StoreInt32(&hits, 0)
	other := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
	if _, err := kit.NewClient(other, "o/r", "sdd-test", kit.BearerAuth); code(err) != "auth" || !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("host mismatch: %v", err)
	}
	if _, err := kit.NewClient(srv.URL, "attacker/x", "sdd-test", kit.BearerAuth); code(err) != "auth" {
		t.Fatalf("scope mismatch: %v", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatal("refused client reached the server")
	}
	if _, err := kit.NewClient("http://example.com", "o/r", "x", kit.BearerAuth); code(err) != "invalid" {
		t.Fatalf("plain http to a public host: %v", err)
	}
}

func TestHTTPRedirects(t *testing.T) {
	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&targetHits, 1)
	}))
	defer target.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/away":
			http.Redirect(w, r, target.URL+"/x", http.StatusFound)
		case "/downgrade":
			http.Redirect(w, r, "http://"+r.Host+"/x", http.StatusFound)
		}
	}))
	defer src.Close()
	bindServer(t, src, "o/r")
	c, err := kit.NewClient(src.URL, "o/r", "ua", kit.BearerAuth)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(kit.Request{Method: "GET", Path: "/away"}, nil); err == nil || !strings.Contains(err.Error(), "refused redirect") {
		t.Fatalf("cross-host redirect: %v", err)
	}
	if atomic.LoadInt32(&targetHits) != 0 {
		t.Fatal("redirect target was reached")
	}
	// Same host, same scheme is allowed; a scheme change is not (simulated
	// with an https base that redirects to http on the same host).
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+r.Host+"/x", http.StatusFound)
	}))
	defer tls.Close()
	bindServer(t, tls, "o/r")
	c, _ = kit.NewClient(tls.URL, "o/r", "ua", kit.BearerAuth)
	c.SetHTTPTransport(tls.Client().Transport)
	if _, err := c.Do(kit.Request{Method: "GET", Path: "/"}, nil); err == nil || !strings.Contains(err.Error(), "refused redirect") {
		t.Fatalf("https to http redirect: %v", err)
	}
}

func TestKittest(t *testing.T) {
	kittest.Run(t, &mem{}, nil)
	ft := &fakeT{TB: t}
	kittest.Run(ft, &mem{noDedupe: true}, nil)
	if !ft.failed {
		t.Fatal("kittest passed a provider that breaks idempotent create")
	}
}

type fakeT struct {
	testing.TB
	failed bool
}

func (f *fakeT) Errorf(string, ...any) { f.failed = true }
func (f *fakeT) Fatalf(string, ...any) { f.failed = true }
func (f *fakeT) Helper()               {}
