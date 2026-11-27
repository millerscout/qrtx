// Package fakeapi is an in-memory fake of the Jira Cloud REST v3 endpoints
// that qrtx-tracker-jira uses, for tests.
package fakeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Transition is one workflow transition the fake offers from every status.
type Transition struct {
	ID            string
	Name          string
	To            string // status name
	Category      string // new | indeterminate | done
	HasResolution bool   // resolution field on the transition screen
}

// Issue is one stored issue.
type Issue struct {
	Key         string
	Summary     string
	Description json.RawMessage
	Status      string
	Category    string
	Resolution  string
	Labels      []string
	Properties  map[string]json.RawMessage
	Comments    []json.RawMessage
	hiddenFor   int
	seq         int
}

// Server is the fake. Lock Mu before touching fields while it serves.
type Server struct {
	*httptest.Server
	Mu          sync.Mutex
	Project     string
	Issues      []*Issue
	Transitions []Transition
	// HideNewFromSearch hides each new issue from the next N searches.
	HideNewFromSearch int
	// StallCreate makes the next create store the issue and then sleep.
	StallCreate time.Duration
	Requests    []string
}

// DefaultTransitions is a standard software workflow.
func DefaultTransitions() []Transition {
	return []Transition{
		{ID: "11", Name: "Back to do", To: "To Do", Category: "new"},
		{ID: "21", Name: "Start", To: "In Progress", Category: "indeterminate"},
		{ID: "31", Name: "Review", To: "In Review", Category: "indeterminate"},
		{ID: "41", Name: "Done", To: "Done", Category: "done", HasResolution: true},
	}
}

// New starts a fake for project.
func New(project string) *Server {
	s := &Server{Project: project, Transitions: DefaultTransitions()}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Count returns the number of stored issues.
func (s *Server) Count() int {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return len(s.Issues)
}

// Get returns the issue with key.
func (s *Server) Get(key string) *Issue {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.find(key)
}

func (s *Server) find(key string) *Issue {
	for _, is := range s.Issues {
		if is.Key == key {
			return is
		}
	}
	return nil
}

func (s *Server) reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func (s *Server) render(is *Issue, props string) map[string]any {
	var res any
	if is.Resolution != "" {
		res = map[string]string{"name": is.Resolution}
	}
	desc := any(nil)
	if len(is.Description) > 0 {
		desc = is.Description
	}
	m := map[string]any{
		"id": strconv.Itoa(is.seq), "key": is.Key, "self": s.URL + "/rest/api/3/issue/" + is.Key,
		"fields": map[string]any{
			"summary": is.Summary, "description": desc, "labels": is.Labels, "resolution": res,
			"status":  map[string]any{"name": is.Status, "statusCategory": map[string]string{"key": is.Category}},
			"updated": "2026-11-26T00:00:00.000+0000", "assignee": nil,
		},
	}
	if props != "" {
		pm := map[string]json.RawMessage{}
		for _, k := range strings.Split(props, ",") {
			if v, ok := is.Properties[k]; ok {
				pm[k] = v
			}
		}
		m["properties"] = pm
	}
	return m
}

var labelJQL = regexp.MustCompile(`labels\s*=\s*"?([^"\s]+)"?`)

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Requests = append(s.Requests, r.Method+" "+r.URL.Path)
	if _, _, ok := r.BasicAuth(); !ok {
		s.reply(w, 401, map[string]any{"errorMessages": []string{"unauthorized"}})
		return
	}
	var body map[string]json.RawMessage
	_ = json.NewDecoder(r.Body).Decode(&body)
	path := r.URL.Path
	const base = "/rest/api/3"
	switch {
	case path == base+"/search/jql" && r.Method == "GET":
		q := r.URL.Query()
		jql := q.Get("jql")
		var list []*Issue
		for _, is := range s.Issues {
			if is.hiddenFor > 0 {
				is.hiddenFor--
				continue
			}
			if !strings.Contains(jql, s.Project) {
				continue
			}
			if m := labelJQL.FindStringSubmatch(jql); m != nil {
				found := false
				for _, l := range is.Labels {
					found = found || l == m[1]
				}
				if !found {
					continue
				}
			}
			list = append(list, is)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].seq > list[j].seq })
		start, _ := strconv.Atoi(q.Get("nextPageToken"))
		max, _ := strconv.Atoi(q.Get("maxResults"))
		if max <= 0 {
			max = 50
		}
		end := start + max
		if end > len(list) {
			end = len(list)
		}
		out := []map[string]any{}
		for _, is := range list[start:end] {
			out = append(out, s.render(is, q.Get("properties")))
		}
		resp := map[string]any{"issues": out, "isLast": end >= len(list)}
		if end < len(list) {
			resp["nextPageToken"] = strconv.Itoa(end)
		}
		s.reply(w, 200, resp)
	case path == base+"/issue" && r.Method == "POST":
		var f struct {
			Project struct {
				Key string `json:"key"`
			} `json:"project"`
			Summary     string          `json:"summary"`
			Description json.RawMessage `json:"description"`
			Labels      []string        `json:"labels"`
		}
		_ = json.Unmarshal(body["fields"], &f)
		if f.Project.Key != s.Project || f.Summary == "" {
			s.reply(w, 400, map[string]any{"errors": map[string]string{"project": "bad project or summary"}})
			return
		}
		for _, l := range f.Labels {
			if strings.ContainsAny(l, " \t") {
				s.reply(w, 400, map[string]any{"errors": map[string]string{"labels": "label contains spaces"}})
				return
			}
		}
		seq := len(s.Issues) + 10000
		is := &Issue{Key: fmt.Sprintf("%s-%d", s.Project, len(s.Issues)+1), Summary: f.Summary, Description: f.Description,
			Status: "To Do", Category: "new", Labels: f.Labels, Properties: map[string]json.RawMessage{}, hiddenFor: s.HideNewFromSearch, seq: seq}
		s.Issues = append(s.Issues, is)
		stall := s.StallCreate
		s.StallCreate = 0
		resp := map[string]string{"id": strconv.Itoa(seq), "key": is.Key, "self": s.URL + base + "/issue/" + is.Key}
		if stall > 0 {
			s.Mu.Unlock()
			time.Sleep(stall)
			s.Mu.Lock()
		}
		s.reply(w, 201, resp)
	case strings.HasPrefix(path, base+"/issue/"):
		rest := strings.Split(strings.TrimPrefix(path, base+"/issue/"), "/")
		is := s.find(rest[0])
		if is == nil {
			s.reply(w, 404, map[string]any{"errorMessages": []string{"Issue does not exist or you do not have permission to see it."}})
			return
		}
		switch {
		case len(rest) == 1 && r.Method == "GET":
			s.reply(w, 200, s.render(is, r.URL.Query().Get("properties")))
		case len(rest) == 1 && r.Method == "PUT":
			var f map[string]json.RawMessage
			_ = json.Unmarshal(body["fields"], &f)
			if v, ok := f["summary"]; ok {
				_ = json.Unmarshal(v, &is.Summary)
			}
			if v, ok := f["description"]; ok {
				is.Description = v
			}
			if v, ok := f["labels"]; ok {
				_ = json.Unmarshal(v, &is.Labels)
			}
			s.reply(w, 204, nil)
		case len(rest) == 2 && rest[1] == "transitions" && r.Method == "GET":
			var ts []map[string]any
			for _, t := range s.Transitions {
				m := map[string]any{"id": t.ID, "name": t.Name, "to": map[string]any{"name": t.To, "statusCategory": map[string]string{"key": t.Category}}}
				if r.URL.Query().Get("expand") == "transitions.fields" {
					fields := map[string]any{}
					if t.HasResolution {
						fields["resolution"] = map[string]any{"required": false, "name": "Resolution"}
					}
					m["fields"] = fields
				}
				ts = append(ts, m)
			}
			s.reply(w, 200, map[string]any{"transitions": ts})
		case len(rest) == 2 && rest[1] == "transitions" && r.Method == "POST":
			var tr struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(body["transition"], &tr)
			var f struct {
				Resolution *struct {
					Name string `json:"name"`
				} `json:"resolution"`
			}
			_ = json.Unmarshal(body["fields"], &f)
			for _, t := range s.Transitions {
				if t.ID != tr.ID {
					continue
				}
				if f.Resolution != nil && !t.HasResolution {
					s.reply(w, 400, map[string]any{"errors": map[string]string{"resolution": "Field 'resolution' cannot be set. It is not on the appropriate screen."}})
					return
				}
				is.Status, is.Category = t.To, t.Category
				switch {
				case f.Resolution != nil:
					is.Resolution = f.Resolution.Name
				case t.Category == "done":
					is.Resolution = "Done"
				default:
					is.Resolution = ""
				}
				s.reply(w, 204, nil)
				return
			}
			s.reply(w, 400, map[string]any{"errorMessages": []string{"Transition id '" + tr.ID + "' is not valid for this issue."}})
		case len(rest) == 2 && rest[1] == "comment" && r.Method == "POST":
			var doc struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(body["body"], &doc); err != nil || doc.Type != "doc" {
				s.reply(w, 400, map[string]any{"errorMessages": []string{"Comment body is not valid ADF."}})
				return
			}
			is.Comments = append(is.Comments, body["body"])
			s.reply(w, 201, map[string]string{"id": strconv.Itoa(len(is.Comments))})
		case len(rest) == 3 && rest[1] == "properties" && r.Method == "PUT":
			raw, _ := json.Marshal(body)
			if len(raw) > 32768 {
				s.reply(w, 400, map[string]any{"errorMessages": []string{"property value too long"}})
				return
			}
			_, existed := is.Properties[rest[2]]
			is.Properties[rest[2]] = raw
			if existed {
				s.reply(w, 200, nil)
			} else {
				s.reply(w, 201, nil)
			}
		default:
			s.reply(w, 404, map[string]any{"errorMessages": []string{"not found"}})
		}
	default:
		s.reply(w, 404, map[string]any{"errorMessages": []string{"not found"}})
	}
}
