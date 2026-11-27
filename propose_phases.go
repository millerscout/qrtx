package qrtx

import (
	"strings"
	"unicode"
)

type phaseBlock struct {
	heading string
	body    string
}

func splitPhases(tasks string) []phaseBlock {
	var out []phaseBlock
	var cur *phaseBlock
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
		}
	}
	for _, line := range strings.Split(tasks, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "### ") {
			flush()
			cur = &phaseBlock{heading: trim}
			continue
		}
		if cur != nil {
			cur.body += line + "\n"
		}
	}
	flush()
	return out
}

func phaseCheckboxes(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		m := sddCheckboxRE.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		out = append(out, strings.TrimSpace(line))
	}
	return out
}

func phasesReady(tasks string) bool {
	phases := splitPhases(tasks)
	if len(phases) == 0 {
		return false
	}
	for _, p := range phases {
		boxes := phaseCheckboxes(p.body)
		if len(boxes) < 1 {
			return false
		}
		for _, box := range boxes {
			if strings.HasPrefix(taskProse(box), "needs user:") {
				return false
			}
		}
	}
	return true
}

func openPhase(tasks string) string {
	for _, p := range splitPhases(tasks) {
		for _, box := range phaseCheckboxes(p.body) {
			if strings.Contains(box, "- [ ]") {
				return p.heading
			}
		}
	}
	return ""
}

func checkboxBody(line string) string {
	body := strings.TrimSpace(line)
	for _, prefix := range []string{"- [ ]", "- [x]", "- [X]"} {
		if strings.HasPrefix(body, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(body, prefix))
		}
	}
	return body
}

func taskProse(line string) string {
	body := checkboxBody(line)
	fields := strings.Fields(body)
	if len(fields) > 0 && isTaskNumber(fields[0]) {
		return strings.TrimSpace(strings.TrimPrefix(body, fields[0]))
	}
	return body
}

func isTaskNumber(s string) bool {
	rest := strings.TrimPrefix(s, "V")
	i := strings.Index(rest, ".")
	if i <= 0 || i >= len(rest)-1 {
		return false
	}
	for _, part := range []string{rest[:i], rest[i+1:]} {
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func decisionGrounded(corpus, task string) bool {
	return decisionGroundedWith(corpus, "", task)
}

func decisionGroundedWith(corpus, earlier, task string) bool {
	body := taskProse(task)
	if strings.HasPrefix(body, "needs user:") {
		return true
	}
	rest := body
	for {
		start := strings.Index(rest, "`")
		if start < 0 {
			return true
		}
		rest = rest[start+1:]
		end := strings.Index(rest, "`")
		if end < 0 {
			return true
		}
		span := rest[:end]
		rest = rest[end+1:]
		if !spanGrounded(corpus, earlier, task, span) {
			return false
		}
	}
}

func hasLetterOrDigit(word string) bool {
	for _, r := range word {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func spanWords(span string) []string {
	span = strings.NewReplacer("(", " ", ")", " ").Replace(span)
	var out []string
	for _, word := range strings.Fields(span) {
		word = strings.TrimPrefix(word, "./")
		word = strings.Trim(word, ".,;:()[]")
		if word == "" || word == "go" || word == "test" || !hasLetterOrDigit(word) {
			continue
		}
		out = append(out, word)
	}
	return out
}

func spanGrounded(corpus, earlier, task, span string) bool {
	var named []string
	for _, word := range spanWords(span) {
		if isPathToken(word) || backtickIdent(word) {
			named = append(named, word)
		}
	}
	if len(named) == 0 {
		return true
	}
	firstOK := wordGrounded(corpus, earlier, task, named[0])
	for i, word := range named {
		if i > 0 && firstOK {
			continue
		}
		if !wordGrounded(corpus, earlier, task, word) {
			return false
		}
	}
	return true
}

func wordGrounded(corpus, earlier, task, word string) bool {
	if isPathToken(word) {
		return pathGrounded(corpus, task, word)
	}
	if backtickIdent(word) {
		return identGrounded(corpus, earlier, task, word)
	}
	return true
}

func backtickIdent(word string) bool {
	if strings.Contains(word, "_") || isDottedName(word) {
		return true
	}
	for i, r := range word {
		if unicode.IsUpper(r) && i > 0 {
			return true
		}
		if unicode.IsUpper(r) && i == 0 {
			return true
		}
	}
	return false
}

func pathGrounded(corpus, task, path string) bool {
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimRight(path, "/")
	if path == "" || path == "change.sdd.md" {
		return path == "change.sdd.md"
	}
	if strings.Contains(corpus, path) || addedIn(task, path) {
		return true
	}
	prose := taskProse(task)
	if strings.Contains(prose, "not in domain") || strings.Contains(prose, "false for") {
		return true
	}
	return dirMentioned(corpus, path)
}

func dirMentioned(corpus, path string) bool {
	dir := filepathDir(path)
	if directoryInCorpus(corpus, dir) {
		return true
	}
	parent := filepathDir(dir)
	if parent == dir {
		return false
	}
	return directoryInCorpus(corpus, parent)
}

func directoryInCorpus(corpus, dir string) bool {
	if dir == "" || dir == "." || !strings.Contains(dir, "/") {
		return false
	}
	return strings.Contains(corpus, dir+"/")
}

func filepathDir(path string) string {
	path = strings.TrimPrefix(path, "./")
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "."
	}
	return path[:i]
}

func identGrounded(corpus, earlier, task, word string) bool {
	if strings.Contains(strings.ToLower(corpus), strings.ToLower(word)) {
		return true
	}
	return addedIn(task, word) || addedIn(earlier, word)
}

func addedIn(text, word string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "Add") && strings.Contains(line, "`"+word) {
			return true
		}
	}
	return false
}

func tasksGrounded(corpus, tasks string) bool {
	var earlier []string
	for _, line := range strings.Split(tasks, "\n") {
		if sddCheckboxRE.FindStringSubmatch(strings.TrimSpace(line)) == nil {
			continue
		}
		if !decisionGroundedWith(corpus, strings.Join(earlier, "\n"), line) {
			return false
		}
		earlier = append(earlier, line)
	}
	return true
}

type phaseFault struct {
	check   string
	heading string
	task    string
	token   string
}

func findPhaseFault(tasks, corpus string) (phaseFault, bool) {
	phases := splitPhases(tasks)
	if len(phases) == 0 {
		return phaseFault{check: "phases", token: "heading"}, true
	}
	for _, p := range phases {
		boxes := phaseCheckboxes(p.body)
		if len(boxes) < 1 {
			return phaseFault{check: "phases", heading: p.heading, task: "", token: "count"}, true
		}
		for _, box := range boxes {
			if strings.HasPrefix(taskProse(box), "needs user:") {
				return phaseFault{check: "phases", heading: p.heading, task: taskID(box), token: "needs user:"}, true
			}
		}
	}
	heading := ""
	var earlier []string
	for _, line := range strings.Split(tasks, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "### ") {
			heading = trim
			continue
		}
		if sddCheckboxRE.FindStringSubmatch(trim) == nil {
			continue
		}
		if tok, ok := ungroundedToken(corpus, strings.Join(earlier, "\n"), line); ok {
			return phaseFault{check: "grounded", heading: heading, task: taskID(line), token: tok}, true
		}
		earlier = append(earlier, line)
	}
	return phaseFault{}, false
}

func taskID(line string) string {
	fields := strings.Fields(checkboxBody(line))
	if len(fields) == 0 || !isTaskNumber(fields[0]) {
		return ""
	}
	return fields[0]
}

func ungroundedToken(corpus, earlier, task string) (string, bool) {
	body := taskProse(task)
	if strings.HasPrefix(body, "needs user:") {
		return "", false
	}
	rest := body
	for {
		start := strings.Index(rest, "`")
		if start < 0 {
			return "", false
		}
		rest = rest[start+1:]
		end := strings.Index(rest, "`")
		if end < 0 {
			return "", false
		}
		span := rest[:end]
		rest = rest[end+1:]
		if tok, ok := ungroundedSpan(corpus, earlier, task, span); ok {
			return tok, true
		}
	}
}

func ungroundedSpan(corpus, earlier, task, span string) (string, bool) {
	var named []string
	for _, word := range spanWords(span) {
		if isPathToken(word) || backtickIdent(word) {
			named = append(named, word)
		}
	}
	if len(named) == 0 {
		return "", false
	}
	firstOK := wordGrounded(corpus, earlier, task, named[0])
	for i, word := range named {
		if i > 0 && firstOK {
			continue
		}
		if !wordGrounded(corpus, earlier, task, word) {
			return word, true
		}
	}
	return "", false
}

func goldfishPassed(goal, testGap, ready string, phasesOK bool) bool {
	if strings.TrimSpace(goal) == "" || !phasesOK {
		return false
	}
	ready = strings.TrimSpace(ready)
	ready = strings.TrimPrefix(strings.ToLower(ready), "ready:")
	ready = strings.TrimSpace(ready)
	if strings.EqualFold(ready, "no") {
		return false
	}
	gap := strings.TrimSpace(testGap)
	if gap != "" && !strings.Contains(gap, "### Phase") {
		return false
	}
	return true
}
