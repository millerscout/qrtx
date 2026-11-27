package jira

import (
	"encoding/json"
	"testing"
)

func FuzzADF(f *testing.F) {
	f.Add("First `code`\nsecond [a](https://x.example)\n\nPara.", []byte(`{"type":"doc","version":1,"content":[]}`))
	f.Add("", []byte(`null`))
	f.Fuzz(func(t *testing.T, md string, raw []byte) {
		var doc adfNode
		if err := json.Unmarshal(MarkdownToADF(md), &doc); err != nil {
			t.Fatalf("not JSON: %v", err)
		}
		if doc.Type != "doc" || doc.Version != 1 {
			t.Fatalf("doc %+v", doc)
		}
		var walk func(n adfNode)
		walk = func(n adfNode) {
			if n.Type == "text" && n.Text == "" {
				t.Fatalf("empty text node from %q", md)
			}
			for _, c := range n.Content {
				walk(c)
			}
		}
		walk(doc)
		_ = ADFToMarkdown(raw) // must not panic
	})
}
