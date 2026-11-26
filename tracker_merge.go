package sdd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

type mergeIssue struct {
	fields map[string]json.RawMessage
	raw    []byte
}

func (m mergeIssue) clientRef() string {
	var s string
	_ = json.Unmarshal(m.fields["client_ref"], &s)
	return s
}

func parseMergeSide(name string, data []byte) (map[string]mergeIssue, error) {
	out := map[string]mergeIssue{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	n := 0
	for sc.Scan() {
		n++
		text := bytes.TrimSpace(sc.Bytes())
		if len(text) == 0 {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(text, &fields); err != nil {
			return nil, fmt.Errorf("%s line %d: %v", name, n, err)
		}
		for k, v := range fields {
			c, err := canonicalJSON(v)
			if err != nil {
				return nil, fmt.Errorf("%s line %d: %v", name, n, err)
			}
			fields[k] = c
		}
		var id string
		if err := json.Unmarshal(fields["id"], &id); err != nil || id == "" {
			return nil, fmt.Errorf("%s line %d: missing id", name, n)
		}
		out[id] = mergeIssue{fields: fields, raw: append([]byte(nil), text...)}
	}
	return out, sc.Err()
}

func encodeFields(fields map[string]json.RawMessage) []byte {
	b, _ := json.Marshal(fields)
	c, err := canonicalJSON(b)
	if err != nil {
		return b
	}
	return c
}

func sameIssue(a, b mergeIssue) bool {
	return bytes.Equal(encodeFields(a.fields), encodeFields(b.fields))
}

// mergeFields does a three-way merge of one issue's top-level fields.
// updated_at never conflicts: the later value wins.
func mergeFields(base, ours, theirs map[string]json.RawMessage) (map[string]json.RawMessage, bool) {
	keys := map[string]bool{}
	for _, m := range []map[string]json.RawMessage{base, ours, theirs} {
		for k := range m {
			keys[k] = true
		}
	}
	out := map[string]json.RawMessage{}
	conflict := false
	for k := range keys {
		b, o, t := base[k], ours[k], theirs[k]
		switch {
		case bytes.Equal(o, t):
			if o != nil {
				out[k] = o
			}
		case bytes.Equal(o, b):
			if t != nil {
				out[k] = t
			}
		case bytes.Equal(t, b):
			if o != nil {
				out[k] = o
			}
		case k == "updated_at":
			if string(o) > string(t) {
				out[k] = o
			} else {
				out[k] = t
			}
		default:
			conflict = true
		}
	}
	return out, conflict
}

// MergeIssues merges three versions of sdd/issues.jsonl per issue id, for
// `sdd tracker merge-driver`. Issues added on both sides are kept; when one
// id holds two different client_refs, the incoming (theirs) issue gets a
// longer id. conflict is true when both sides changed one field of one issue
// differently, or one side deleted an issue the other edited; those issues
// are written between conflict markers.
func MergeIssues(base, ours, theirs []byte) (merged []byte, conflict bool, err error) {
	b, err := parseMergeSide("base", base)
	if err != nil {
		return nil, false, err
	}
	o, err := parseMergeSide("ours", ours)
	if err != nil {
		return nil, false, err
	}
	t, err := parseMergeSide("theirs", theirs)
	if err != nil {
		return nil, false, err
	}
	type entry struct {
		id   string
		line []byte
	}
	var out []entry
	taken := map[string]string{}
	for id, m := range o {
		taken[id] = m.clientRef()
	}
	for id, m := range t {
		if _, ok := taken[id]; !ok {
			taken[id] = m.clientRef()
		}
	}
	ids := map[string]bool{}
	for _, m := range []map[string]mergeIssue{b, o, t} {
		for id := range m {
			ids[id] = true
		}
	}
	conflictLines := func(id string, ours, theirs []byte) {
		conflict = true
		var buf bytes.Buffer
		buf.WriteString("<<<<<<< ours\n")
		if ours != nil {
			buf.Write(ours)
			buf.WriteByte('\n')
		}
		buf.WriteString("=======\n")
		if theirs != nil {
			buf.Write(theirs)
			buf.WriteByte('\n')
		}
		buf.WriteString(">>>>>>> theirs")
		out = append(out, entry{id, buf.Bytes()})
	}
	for id := range ids {
		bi, inB := b[id]
		oi, inO := o[id]
		ti, inT := t[id]
		switch {
		case !inB && inO && !inT:
			out = append(out, entry{id, encodeFields(oi.fields)})
		case !inB && !inO && inT:
			out = append(out, entry{id, encodeFields(ti.fields)})
		case !inB && inO && inT:
			if sameIssue(oi, ti) {
				out = append(out, entry{id, encodeFields(oi.fields)})
				continue
			}
			if oi.clientRef() != "" && ti.clientRef() != "" && oi.clientRef() != ti.clientRef() {
				out = append(out, entry{id, encodeFields(oi.fields)})
				newID := issueIDFrom(ti.clientRef(), taken, 8)
				taken[newID] = ti.clientRef()
				fields := map[string]json.RawMessage{}
				for k, v := range ti.fields {
					fields[k] = v
				}
				fields["id"], _ = json.Marshal(newID)
				out = append(out, entry{newID, encodeFields(fields)})
				continue
			}
			m, c := mergeFields(nil, oi.fields, ti.fields)
			if c {
				conflictLines(id, encodeFields(oi.fields), encodeFields(ti.fields))
				continue
			}
			out = append(out, entry{id, encodeFields(m)})
		case inB && !inO && !inT:
			// deleted on both sides
		case inB && !inO && inT:
			if !sameIssue(bi, ti) {
				conflictLines(id, nil, encodeFields(ti.fields))
			}
		case inB && inO && !inT:
			if !sameIssue(bi, oi) {
				conflictLines(id, encodeFields(oi.fields), nil)
			}
		case inB && inO && inT:
			m, c := mergeFields(bi.fields, oi.fields, ti.fields)
			if c {
				conflictLines(id, encodeFields(oi.fields), encodeFields(ti.fields))
				continue
			}
			out = append(out, entry{id, encodeFields(m)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].id < out[j].id })
	var buf bytes.Buffer
	for _, e := range out {
		buf.Write(e.line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), conflict, nil
}
