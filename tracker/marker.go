package tracker

import (
	"encoding/json"
	"strings"
)

// Marker is what a provider keeps in an issue body when its backend has no
// field for it: the create's client_ref and the issue's sdd refs.
type Marker struct {
	ClientRef string   `json:"client_ref,omitempty"`
	Refs      []SDDRef `json:"sdd,omitempty"`
}

const markerPrefix = "<!-- sdd:v1 "
const markerSuffix = " -->"

// EncodeMarker returns description with m as its last line, replacing any
// marker already there.
func EncodeMarker(description string, m Marker) string {
	desc, _, _ := DecodeMarker(description)
	b, _ := json.Marshal(m)
	desc = strings.TrimRight(desc, " \t\r\n")
	line := markerPrefix + string(b) + markerSuffix
	if desc == "" {
		return line
	}
	return desc + "\n\n" + line
}

// DecodeMarker splits a body into its description and marker. Only a marker
// on the last non-empty line counts. A malformed marker is left in the text
// and ok is false.
func DecodeMarker(body string) (description string, m Marker, ok bool) {
	trimmed := strings.TrimRight(body, " \t\r\n")
	i := strings.LastIndex(trimmed, "\n")
	last := trimmed[i+1:]
	if !strings.HasPrefix(last, markerPrefix) || !strings.HasSuffix(last, markerSuffix) {
		return body, Marker{}, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(last, markerPrefix), markerSuffix)
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return body, Marker{}, false
	}
	if i < 0 {
		return "", m, true
	}
	return strings.TrimRight(trimmed[:i], " \t\r\n"), m, true
}

// Precedence is what a provider keeps for an issue's links and priority when
// its backend has no field for them. It lives apart from Marker so that
// plugins which rewrite the sdd marker whole leave it alone.
type Precedence struct {
	Priority *int        `json:"priority,omitempty"`
	Links    []IssueLink `json:"links,omitempty"`
}

// Empty reports whether p holds nothing worth storing.
func (p Precedence) Empty() bool { return p.Priority == nil && len(p.Links) == 0 }

const precPrefix = "<!-- sdd-prec:v1 "

// EncodePrec returns description with p as its last line, replacing any
// precedence line already there. An empty p removes the line. Put the sdd
// Marker after it with EncodeMarker: the sdd marker must stay the last line.
func EncodePrec(description string, p Precedence) string {
	desc, _, _ := DecodePrec(description)
	if p.Empty() {
		return desc
	}
	b, _ := json.Marshal(p)
	desc = strings.TrimRight(desc, " \t\r\n")
	line := precPrefix + string(b) + markerSuffix
	if desc == "" {
		return line
	}
	return desc + "\n\n" + line
}

// DecodePrec splits a body into its description and precedence. Only a line
// on the last non-empty line counts; a malformed one stays in the text.
func DecodePrec(body string) (description string, p Precedence, ok bool) {
	trimmed := strings.TrimRight(body, " \t\r\n")
	i := strings.LastIndex(trimmed, "\n")
	last := trimmed[i+1:]
	if !strings.HasPrefix(last, precPrefix) || !strings.HasSuffix(last, markerSuffix) {
		return body, Precedence{}, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(last, precPrefix), markerSuffix)
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return body, Precedence{}, false
	}
	if i < 0 {
		return "", p, true
	}
	return strings.TrimRight(trimmed[:i], " \t\r\n"), p, true
}

// Apply edits p the way an UpdateIssueInput asks and reports whether it changed.
func (p *Precedence) Apply(in UpdateIssueInput) bool {
	is := Issue{Links: p.Links, Priority: p.Priority}
	changed := is.AddLinks(in.AddLinks)
	if is.RemoveLinks(in.RemoveLinks) {
		changed = true
	}
	if is.SetPriority(in.Priority) {
		changed = true
	}
	p.Links, p.Priority = is.Links, is.Priority
	return changed
}
