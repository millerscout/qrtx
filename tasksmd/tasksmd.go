// Package tasksmd parses Markdown task checklists ("- [ ] 1.1 ...") and
// "## Task N" prose headings into a progress summary.
package tasksmd

import (
	"os"
	"regexp"
	"strings"
)

// ItemsCap bounds how many items ParseTasksMD returns.
const ItemsCap = 200

var (
	checkboxRE  = regexp.MustCompile(`^(\s*)- \[([ xX])\] (.*)$`)
	proseTaskRE = regexp.MustCompile(`(?i)^#{2,3}\s+(?:Task\s+)?(\d+)[.:)\s]+(.+)$`)
)

// Item is one task line.
type Item struct {
	Text  string
	Done  bool
	Prose bool
	Key   string
}

// Progress summarises the tasks in one file.
type Progress struct {
	Change    string
	Total     int
	Done      int
	NextItems []string
	Items     []Item
	Found     bool
}

// ParseTasksMD reads path and returns its task progress. A missing file
// yields an empty Progress with Found false.
func ParseTasksMD(path string) Progress {
	empty := Progress{NextItems: []string{}, Items: []Item{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		return empty
	}
	text := string(raw)
	total := 0
	done := 0
	nextItems := []string{}
	items := []Item{}
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := checkboxRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		total++
		isDone := strings.EqualFold(m[2], "x")
		if isDone {
			done++
		}
		itemText := strings.TrimSpace(m[3])
		if itemText != "" {
			if !isDone && len(nextItems) < 3 {
				nextItems = append(nextItems, itemText)
			}
			if len(items) < ItemsCap {
				items = append(items, Item{Text: itemText, Done: isDone})
			}
		}
	}
	if total == 0 {
		proseItems := []Item{}
		proseNext := []string{}
		inFence = false
		for _, line := range strings.Split(text, "\n") {
			stripped := strings.TrimSpace(line)
			if strings.HasPrefix(stripped, "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			m := proseTaskRE.FindStringSubmatch(stripped)
			if m == nil {
				continue
			}
			title := strings.TrimSpace(m[2])
			if title == "" {
				continue
			}
			itemText := m[1] + ". " + title
			if len(proseNext) < 3 {
				proseNext = append(proseNext, itemText)
			}
			if len(proseItems) < ItemsCap {
				proseItems = append(proseItems, Item{Text: itemText, Done: false, Prose: true})
			}
		}
		if len(proseItems) > 0 {
			total = len(proseItems)
			nextItems = proseNext
			items = proseItems
		}
	}
	found := total > 0
	if _, err := os.Stat(path); err == nil {
		found = true
	}
	return Progress{
		Total: total, Done: done, NextItems: nextItems, Items: items, Found: found,
	}
}
