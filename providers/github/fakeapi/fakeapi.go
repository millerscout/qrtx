// Package fakeapi is an in-memory fake of the GitHub REST endpoints that
// qrtx-tracker-github uses, for tests.
package fakeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Issue is one stored issue or pull request.
type Issue struct {
	Number      int
	Title       string
	Body        string
	State       string // open | closed
	StateReason string
	Labels      []string
	Creator     string
	PR          bool
	Comments    []string
}

// Server is the fake. Lock Mu before touching fields while it serves.
type Server struct {
	*httptest.Server
	Mu       sync.Mutex
	Repo     string // owner/name it serves
	Login    string
	Issues   []*Issue
	Labels   map[string]bool
	Requests []string // "METHOD path"
	// StallCreate makes the next create store the issue and then sleep this
	// long before answering.
	StallCreate time.Duration
	// HideFromListOnce hides issue number (as a string) from the next list.
	HideFromListOnce string
}

// New starts a fake serving repo.
func New(repo string) *Server {
	s := &Server{Repo: repo, Login: "sdd-bot", Labels: map[string]bool{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Add stores an issue and returns it.
func (s *Server) Add(is Issue) *Issue {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	is.Number = len(s.Issues) + 1
	if is.State == "" {
		is.State = "open"
	}
	if is.Creator == "" {
		is.Creator = s.Login
	}
	p := &is
	s.Issues = append(s.Issues, p)
	return p
}

// Count returns the number of stored non-PR issues.
func (s *Server) Count() int {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	n := 0
	for _, is := range s.Issues {
		if !is.PR {
			n++
		}
	}
	return n
}

func (s *Server) json(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) render(is *Issue) map[string]any {
	labels := []map[string]string{}
	for _, l := range is.Labels {
		labels = append(labels, map[string]string{"name": l})
	}
	m := map[string]any{
		"number": is.Number, "title": is.Title, "body": is.Body, "state": is.State,
		"labels": labels, "user": map[string]string{"login": is.Creator},
		"html_url":   fmt.Sprintf("https://github.example/%s/issues/%d", s.Repo, is.Number),
		"updated_at": "2026-11-26T00:00:00Z",
	}
	if is.StateReason != "" {
		m["state_reason"] = is.StateReason
	} else {
		m["state_reason"] = nil
	}
	if is.PR {
		m["pull_request"] = map[string]string{"url": "x"}
	}
	return m
}

func (s *Server) find(n string) *Issue {
	num, _ := strconv.Atoi(n)
	for _, is := range s.Issues {
		if is.Number == num {
			return is
		}
	}
	return nil
}

func hasLabel(is *Issue, l string) bool {
	for _, x := range is.Labels {
		if x == l {
			return true
		}
	}
	return false
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.Mu.Lock()
	s.Requests = append(s.Requests, r.Method+" "+r.URL.Path)
	if r.Header.Get("Authorization") == "" || r.Header.Get("User-Agent") == "" {
		s.Mu.Unlock()
		s.json(w, 401, map[string]string{"message": "Requires authentication"})
		return
	}
	prefix := "/repos/" + s.Repo
	path := r.URL.Path
	var body map[string]json.RawMessage
	_ = json.NewDecoder(r.Body).Decode(&body)
	str := func(k string) (string, bool) {
		v, ok := body[k]
		if !ok {
			return "", false
		}
		var out string
		_ = json.Unmarshal(v, &out)
		return out, true
	}
	switch {
	case path == "/user" && r.Method == "GET":
		s.Mu.Unlock()
		s.json(w, 200, map[string]string{"login": s.Login})
	case path == prefix+"/issues" && r.Method == "GET":
		q := r.URL.Query()
		var list []*Issue
		hide := s.HideFromListOnce
		s.HideFromListOnce = ""
		for _, is := range s.Issues {
			if hide != "" && strconv.Itoa(is.Number) == hide {
				continue
			}
			st := q.Get("state")
			if st == "" {
				st = "open"
			}
			if st != "all" && is.State != st {
				continue
			}
			if c := q.Get("creator"); c != "" && is.Creator != c {
				continue
			}
			if ls := q.Get("labels"); ls != "" {
				ok := true
				for _, l := range strings.Split(ls, ",") {
					ok = ok && hasLabel(is, l)
				}
				if !ok {
					continue
				}
			}
			list = append(list, is)
		}
		if q.Get("direction") != "asc" {
			sort.Slice(list, func(i, j int) bool { return list[i].Number > list[j].Number })
		}
		per, _ := strconv.Atoi(q.Get("per_page"))
		if per <= 0 {
			per = 30
		}
		page, _ := strconv.Atoi(q.Get("page"))
		if page <= 0 {
			page = 1
		}
		start := (page - 1) * per
		end := start + per
		if start > len(list) {
			start = len(list)
		}
		if end > len(list) {
			end = len(list)
		}
		out := []map[string]any{}
		for _, is := range list[start:end] {
			out = append(out, s.render(is))
		}
		if end < len(list) {
			nq := r.URL.Query()
			nq.Set("page", strconv.Itoa(page+1))
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?%s>; rel="next"`, s.URL, path, nq.Encode()))
		}
		s.Mu.Unlock()
		s.json(w, 200, out)
	case path == prefix+"/issues" && r.Method == "POST":
		title, _ := str("title")
		text, _ := str("body")
		var labels []string
		_ = json.Unmarshal(body["labels"], &labels)
		is := &Issue{Number: len(s.Issues) + 1, Title: title, Body: text, State: "open", Labels: labels, Creator: s.Login}
		for _, l := range labels {
			s.Labels[l] = true
		}
		s.Issues = append(s.Issues, is)
		stall := s.StallCreate
		s.StallCreate = 0
		rendered := s.render(is)
		s.Mu.Unlock()
		if stall > 0 {
			time.Sleep(stall)
		}
		s.json(w, 201, rendered)
	case path == prefix+"/labels" && r.Method == "POST":
		name, _ := str("name")
		if s.Labels[name] {
			s.Mu.Unlock()
			s.json(w, 422, map[string]any{"message": "Validation Failed", "errors": []map[string]string{{"resource": "Label", "code": "already_exists", "field": "name"}}})
			return
		}
		s.Labels[name] = true
		s.Mu.Unlock()
		s.json(w, 201, map[string]string{"name": name})
	case strings.HasPrefix(path, prefix+"/issues/"):
		rest := strings.Split(strings.TrimPrefix(path, prefix+"/issues/"), "/")
		is := s.find(rest[0])
		if is == nil {
			s.Mu.Unlock()
			s.json(w, 404, map[string]string{"message": "Not Found"})
			return
		}
		switch {
		case len(rest) == 1 && r.Method == "GET":
		case len(rest) == 1 && r.Method == "PATCH":
			if v, ok := str("title"); ok {
				is.Title = v
			}
			if v, ok := str("body"); ok {
				is.Body = v
			}
			if v, ok := str("state"); ok {
				is.State = v
				if v == "open" {
					is.StateReason = ""
				}
			}
			if v, ok := str("state_reason"); ok && is.State == "closed" {
				is.StateReason = v
			}
			if raw, ok := body["labels"]; ok {
				var labels []string
				_ = json.Unmarshal(raw, &labels)
				is.Labels = labels
				for _, l := range labels {
					s.Labels[l] = true
				}
			}
		case len(rest) == 2 && rest[1] == "comments" && r.Method == "POST":
			c, _ := str("body")
			is.Comments = append(is.Comments, c)
			s.Mu.Unlock()
			s.json(w, 201, map[string]any{"id": len(is.Comments), "body": c})
			return
		case len(rest) == 2 && rest[1] == "labels" && r.Method == "POST":
			var labels []string
			_ = json.Unmarshal(body["labels"], &labels)
			for _, l := range labels {
				if !s.Labels[l] {
					s.Mu.Unlock()
					s.json(w, 422, map[string]string{"message": "label does not exist: " + l})
					return
				}
				if !hasLabel(is, l) {
					is.Labels = append(is.Labels, l)
				}
			}
		case len(rest) == 3 && rest[1] == "labels" && r.Method == "DELETE":
			keep := is.Labels[:0]
			found := false
			for _, l := range is.Labels {
				if l == rest[2] {
					found = true
					continue
				}
				keep = append(keep, l)
			}
			is.Labels = keep
			if !found {
				s.Mu.Unlock()
				s.json(w, 404, map[string]string{"message": "Label does not exist"})
				return
			}
		default:
			s.Mu.Unlock()
			s.json(w, 404, map[string]string{"message": "Not Found"})
			return
		}
		out := s.render(is)
		s.Mu.Unlock()
		s.json(w, 200, out)
	default:
		s.Mu.Unlock()
		s.json(w, 404, map[string]string{"message": "Not Found"})
	}
}
