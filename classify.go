package qrtx

import (
	"errors"
	"strings"
)

// classifyUncued classifies lines that have no deterministic cue.
// A nil map entry means abstain. Tests set this and do not dial.
var classifyUncued func(kind string, lines []string) map[string]string

// LineClassifier labels each line with one key of criteria ("abstain" when
// the line does not say). It is the first reader; SecondReader is the
// second. Nil means no first reader, so every line goes to SecondReader.
var LineClassifier func(kind string, lines []string, criteria map[string]string) (map[string]string, error)

var errNoLineClassifier = errors.New("qrtx: no line classifier")

// SecondReader completes one LLM prompt. It labels lines the first reader
// missed and settles disagreements. Nil means no LLM is used. A host sets it
// for the duration of a request.
var SecondReader func(prompt string) (string, error)

func classifyParagraphs(lines []string) map[int]string {
	return classifyKind("paragraph", lines)
}

func classifyKind(kind string, lines []string) map[int]string {
	out := map[int]string{}
	var pending []string
	var index []int
	for i, line := range lines {
		if choice, ok := cueChoice(kind, line); ok {
			out[i] = choice
			continue
		}
		pending = append(pending, line)
		index = append(index, i)
	}
	if len(pending) == 0 {
		return out
	}
	choices := map[string]string{}
	if classifyUncued != nil {
		choices = classifyUncued(kind, pending)
	} else {
		choices = liveChoices(kind, pending)
	}
	for n, line := range pending {
		choice := choices[line]
		if choice == "" || choice == "abstain" {
			choice = fallbackChoice(kind)
		}
		out[index[n]] = choice
	}
	return out
}

func cueChoice(kind, line string) (string, bool) {
	switch kind {
	case "paragraph":
		if laterParagraph(line) {
			return "deferred", true
		}
		if sequenceParagraph(line) {
			return "sequence", true
		}
	case "follow":
		if strings.Contains(strings.ToLower(line), "accept") {
			return "accept", true
		}
		if tok := lastAnswerFile(line); tok != "" {
			return tok, true
		}
	}
	return "", false
}

func sequenceParagraph(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "goes first") || strings.Contains(lower, "first thing")
}

func fallbackChoice(kind string) string {
	if kind == "paragraph" {
		return "phase"
	}
	return "abstain"
}

func lastAnswerFile(text string) string {
	var tok string
	for _, field := range strings.Fields(text) {
		field = strings.Trim(field, "`.,;:\"'")
		if strings.Count(field, ".") != 1 || len(field) < 3 || strings.Contains(field, "phases/") {
			continue
		}
		tok = field
	}
	return tok
}

func liveChoices(kind string, lines []string) map[string]string {
	out := map[string]string{}
	first, err := lineChoices(kind, lines)
	var missed, labeled []string
	if err != nil {
		missed = append(missed, lines...)
	} else {
		for _, line := range lines {
			choice := first[line]
			if choice == "" || choice == "abstain" || !validChoice(kind, choice) {
				missed = append(missed, line)
				continue
			}
			labeled = append(labeled, line)
			out[line] = choice
		}
	}
	if len(missed) > 0 && SecondReader != nil {
		for line, choice := range goldfishChoices(kind, missed) {
			if choice != "" && choice != "abstain" && validChoice(kind, choice) {
				out[line] = choice
			}
		}
	}
	if err != nil || len(labeled) == 0 || SecondReader == nil {
		return out
	}
	compared := goldfishCompare(kind, labeled, first)
	var mismatch []string
	for _, line := range labeled {
		other := compared[line]
		if other == "" || other == first[line] || !validChoice(kind, other) {
			continue
		}
		mismatch = append(mismatch, line)
	}
	if len(mismatch) == 0 {
		return out
	}
	decided := goldfishDecide(kind, mismatch, first, compared)
	for _, line := range mismatch {
		choice := decided[line]
		if choice == first[line] || choice == compared[line] {
			out[line] = choice
		}
	}
	return out
}

func validChoice(kind, choice string) bool {
	_, ok := lineCriteria(kind)[choice]
	return ok && choice != "abstain"
}

func lineChoices(kind string, lines []string) (map[string]string, error) {
	if LineClassifier == nil {
		return nil, errNoLineClassifier
	}
	return LineClassifier(kind, lines, lineCriteria(kind))
}

// lineCriteria lists the labels a LineClassifier may return for kind.
func lineCriteria(kind string) map[string]string {
	if kind == "paragraph" {
		return map[string]string{
			"phase":    "a feature the request already names",
			"deferred": "a feature the request says comes later",
			"sequence": "a note about what goes first",
			"abstain":  "the line does not say",
		}
	}
	return map[string]string{
		"file":      "the file the user just named",
		"phasepath": "a phases/ path already on the roadmap",
		"abstain":   "the line does not say",
	}
}

func goldfishChoices(kind string, lines []string) map[string]string {
	if SecondReader == nil {
		return nil
	}
	var b strings.Builder
	b.WriteString("Choose one option per line. Options: ")
	for name := range lineCriteria(kind) {
		b.WriteString(name)
		b.WriteString(" ")
	}
	b.WriteString("\nReturn choice: <option> for each line, in order.\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	text, err := SecondReader(b.String())
	if err != nil {
		return nil
	}
	out := map[string]string{}
	found := goldfishChoiceLines(text)
	for i, line := range lines {
		if i < len(found) {
			out[line] = found[i]
		}
	}
	return out
}

func goldfishCompare(kind string, lines []string, first map[string]string) map[string]string {
	var b strings.Builder
	b.WriteString("A first reader already chose. Return choice: <option> for each line, using the same options.\n")
	for _, line := range lines {
		b.WriteString("First reader chose " + first[line] + " for: " + line + "\n")
	}
	return goldfishPrompt(kind, lines, b.String())
}

func goldfishDecide(kind string, lines []string, first, other map[string]string) map[string]string {
	var b strings.Builder
	b.WriteString("Two readers disagreed. Return choice: one of the two labels, nothing else.\n")
	for _, line := range lines {
		b.WriteString("Reader 1 chose " + first[line] + ". Reader 2 chose " + other[line] + ". Line: " + line + "\n")
	}
	return goldfishPrompt(kind, lines, b.String())
}

func goldfishPrompt(kind string, lines []string, prompt string) map[string]string {
	if SecondReader == nil {
		return nil
	}
	text, err := SecondReader(prompt + "\nOptions include abstain.\n")
	if err != nil {
		return nil
	}
	out := map[string]string{}
	found := goldfishChoiceLines(text)
	for i, line := range lines {
		if i < len(found) {
			out[line] = found[i]
		}
	}
	_ = kind
	return out
}

func goldfishChoiceLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(strings.TrimSpace(line))
		if !strings.HasPrefix(lower, "choice:") {
			continue
		}
		out = append(out, strings.TrimSpace(lower[len("choice:"):]))
	}
	return out
}

func parseBlockLine(line string) (id, status, file string, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "{:") || !strings.HasSuffix(line, "}") {
		return "", "", "", false
	}
	id = blockAttr(line, "id")
	status = blockAttr(line, "status")
	file = blockAttr(line, "file")
	return id, status, file, id != ""
}

func blockAttr(line, name string) string {
	key := name + `="`
	i := strings.Index(line, key)
	if i < 0 {
		return ""
	}
	rest := line[i+len(key):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func dropBlock(tasks, id string) string {
	var out []string
	skip := false
	for _, line := range strings.Split(tasks, "\n") {
		if skip {
			skip = false
			trim := strings.TrimSpace(line)
			if trim != "" && !strings.HasPrefix(trim, "###") && !strings.HasPrefix(trim, "{:") {
				continue
			}
		}
		if pid, _, _, ok := parseBlockLine(line); ok && pid == id {
			skip = true
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
