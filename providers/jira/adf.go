package jira

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Atlassian Document Format: just the parts sdd writes. Paragraphs are split
// on blank lines, single newlines become hard breaks, `code` becomes a code
// mark and [text](url) becomes a link mark.

type adfNode struct {
	Type    string         `json:"type"`
	Version int            `json:"version,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []adfMark      `json:"marks,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []adfNode      `json:"content,omitempty"`
}

type adfMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

var inlineRE = regexp.MustCompile("`([^`]+)`|\\[([^\\]]+)\\]\\(([^)\\s]+)\\)")

func inline(line string) []adfNode {
	var out []adfNode
	last := 0
	for _, m := range inlineRE.FindAllStringSubmatchIndex(line, -1) {
		if m[0] > last {
			out = append(out, adfNode{Type: "text", Text: line[last:m[0]]})
		}
		if m[2] >= 0 {
			out = append(out, adfNode{Type: "text", Text: line[m[2]:m[3]], Marks: []adfMark{{Type: "code"}}})
		} else {
			out = append(out, adfNode{Type: "text", Text: line[m[4]:m[5]], Marks: []adfMark{{Type: "link", Attrs: map[string]any{"href": line[m[6]:m[7]]}}}})
		}
		last = m[1]
	}
	if last < len(line) {
		out = append(out, adfNode{Type: "text", Text: line[last:]})
	}
	return out
}

// MarkdownToADF converts sdd's plain Markdown to an ADF document.
func MarkdownToADF(md string) json.RawMessage {
	doc := adfNode{Type: "doc", Version: 1, Content: []adfNode{}}
	md = strings.ReplaceAll(strings.TrimSpace(md), "\r\n", "\n")
	if md != "" {
		for _, para := range regexp.MustCompile(`\n\s*\n`).Split(md, -1) {
			p := adfNode{Type: "paragraph"}
			for i, line := range strings.Split(para, "\n") {
				if i > 0 {
					p.Content = append(p.Content, adfNode{Type: "hardBreak"})
				}
				p.Content = append(p.Content, inline(line)...)
			}
			doc.Content = append(doc.Content, p)
		}
	}
	b, _ := json.Marshal(doc)
	return b
}

// ADFToMarkdown renders an ADF document back to sdd's Markdown. Unknown node
// types contribute their text content.
func ADFToMarkdown(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s // a plain-string description from an older API
	}
	var doc adfNode
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	var blocks []string
	for _, n := range doc.Content {
		blocks = append(blocks, strings.TrimRight(render(n), "\n"))
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n"))
}

func render(n adfNode) string {
	switch n.Type {
	case "text":
		t := n.Text
		for _, m := range n.Marks {
			switch m.Type {
			case "code":
				t = "`" + t + "`"
			case "link":
				if href, ok := m.Attrs["href"].(string); ok {
					t = "[" + t + "](" + href + ")"
				}
			}
		}
		return t
	case "hardBreak":
		return "\n"
	}
	var b strings.Builder
	for i, c := range n.Content {
		if i > 0 && isBlock(c.Type) {
			b.WriteString("\n\n")
		}
		b.WriteString(render(c))
	}
	return b.String()
}

func isBlock(t string) bool {
	switch t {
	case "paragraph", "heading", "bulletList", "orderedList", "listItem", "codeBlock", "blockquote", "panel":
		return true
	}
	return false
}
