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
