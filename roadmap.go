package sdd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

type roadmapPhase struct {
	ID        string
	Heading   string
	Status    string
	File      string
	Paragraph string
	Note      string
}

func roadmapMarked(tasks string) bool {
	return strings.Contains(tasks, `status="review"`) || strings.Contains(tasks, `status="accepted"`) || strings.Contains(tasks, "\nreview\n") || strings.Contains(tasks, "\naccepted\n")
}

func requestParagraphs(request string) []string {
	var out []string
	for _, line := range strings.Split(request, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func MultiParagraphRequest(request string) bool {
	return len(requestParagraphs(request)) >= 3
}

// PlanRoadmap copies one phase per paragraph the request already names.
// A paragraph that says "later" is deferred. A request with no path gets one question.
func PlanRoadmap(request string) (phases []roadmapPhase, deferred []string, question string) {
	paragraphs := requestParagraphs(request)
	choices := classifyParagraphs(paragraphs)
	n := 0
	for i, paragraph := range paragraphs {
		if choices[i] == "sequence" || sequenceParagraph(paragraph) {
			if n > 0 {
				phases[n-1].Note = paragraph
			}
			continue
		}
		if choices[i] == "deferred" || laterParagraph(paragraph) {
			deferred = append(deferred, paragraph)
			continue
		}
		n++
		title := phaseTitle(paragraph)
		file := fmt.Sprintf("phases/%02d-%s.md", n, phaseSlug(title))
		phases = append(phases, roadmapPhase{
			ID:        fmt.Sprintf("phase-%02d", n),
			Heading:   "### Phase " + title,
			Status:    "review",
			File:      file,
			Paragraph: paragraph,
		})
	}
	if !hasPath(request) {
		question = OpenQuestion(request)
	}
	return phases, deferred, question
}

// OpenQuestion is the one checkbox that blocks finish until the user answers.
// A failure asks for the check output. Two checks ask which one belongs.
// Any other open request asks for the outcome.
func OpenQuestion(request string) string {
	if failureShapedRequest(request) {
		return "needs user: what did the failing check print?"
	}
	if acceptanceBulletCount(request) >= 2 {
		return "needs user: which check belongs in this change?"
	}
	return fmt.Sprintf("needs user: what outcome should %q deliver?", firstSentence(request))
}

func laterParagraph(paragraph string) bool {
	return regexp.MustCompile(`(?i)\blater\b`).MatchString(paragraph)
}

func phaseTitle(paragraph string) string {
	fields := strings.Fields(paragraph)
	if len(fields) > 6 {
		fields = fields[:6]
	}
	title := strings.Trim(strings.Join(fields, " "), ".,;:")
	if title == "" {
		return "Feature"
	}
	return title
}

func phaseSlug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if s == "" {
		return "phase"
	}
	return s
}

func renderRoadmap(phases []roadmapPhase, deferred []string, question string) (tasks, deferredSection, reply string) {
	var taskB, replyB strings.Builder
	for _, phase := range phases {
		id := phase.ID
		if id == "" {
			id = phaseSlug(phase.Heading)
		}
		block := phase.Heading + "\n" + fmt.Sprintf("{: id=%q status=%q file=%q}", id, phase.Status, phase.File) + "\n"
		if phase.Note != "" {
			block += fmt.Sprintf("{: id=%q}\n%s\n", id+"-note", phase.Note)
		}
		taskB.WriteString(block + "\n")
		replyB.WriteString(block + "\n")
	}
	if question != "" {
		taskB.WriteString("{: id=\"needs-user\"}\n" + question + "\n")
		replyB.WriteString("\n" + question + "\n")
	}
	if len(deferred) > 0 {
		replyB.WriteString("\n## Deferred\n")
		for _, line := range deferred {
			replyB.WriteString("- " + line + "\n")
		}
	}
	var def strings.Builder
	if len(deferred) == 0 {
		def.WriteString("(Optional — other slugs that own split acceptance.)\n")
	} else {
		for _, line := range deferred {
			def.WriteString("- " + line + "\n")
		}
	}
	return strings.TrimSpace(taskB.String()), strings.TrimSpace(def.String()), strings.TrimSpace(replyB.String())
}

// WriteRoadmap stores the roadmap in change.sdd.md and an empty phase file per feature.
func WriteRoadmap(root, slug, request, observed string) (string, error) {
	phases, deferred, question := PlanRoadmap(request)
	if len(phases) == 0 {
		return "", fmt.Errorf("roadmap: no phases")
	}
	failure := failureShapedRequest(request)
	sibling, err := SiblingRequestSlug(root, slug, request)
	if err != nil {
		return "", err
	}
	if failure || sibling != "" {
		question = ""
	}
	if sibling != "" {
		deferred = append(deferred, sibling)
	}
	for i, phase := range phases {
		path := filepath.Join(root, "sdd", "changes", slug, filepath.FromSlash(phase.File))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		steps := groundedPhaseSteps(phase.Paragraph, nil)
		if i == 0 && failure {
			steps = append([]string{RegressionTask()}, steps...)
		} else if i == 0 && question != "" {
			steps = append([]string{question}, steps...)
		}
		if len(steps) == 1 && strings.HasPrefix(steps[0], "needs user:") {
			phase.Status = "planning"
			phases[i].Status = "planning"
		}
		body := phaseFileBody(phase, i, steps)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return "", err
		}
	}
	tasks, deferredSection, reply := renderRoadmap(phases, deferred, question)
	tasks = MarkQuestionOnlyPlanning(tasks)
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(raw)
	text = replaceSection(text, "Tasks", tasks)
	text = replaceSection(text, "User request", strings.TrimSpace(request))
	text = replaceSection(text, "Deferred", deferredSection)
	if strings.TrimSpace(observed) != "" {
		text = replaceSection(text, "Observed", strings.TrimSpace(observed))
	}
	why := sectionMarkdown(text, "Why")
	if failure {
		text = replaceSection(text, "Why", RegressionWhy())
	} else if strings.Contains(why, "TBD") && len(phases) > 0 {
		text = replaceSection(text, "Why", phases[0].Paragraph)
	}
	acceptance := "- " + firstSentence(request)
	if failure {
		acceptance = RegressionAcceptance()
	}
	text = replaceSection(text, "Acceptance", acceptance)
	if failure {
		text = replaceSection(text, "Validate", "- [ ] V1.1 "+RegressionValidateCommand(request, sectionMarkdown(text, "Observed")))
	}
	text = RemovePlaceholderSpecSections(text)
	text = stampSectionIDs(text)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return reply, nil
}

func firstSentence(request string) string {
	paragraphs := requestParagraphs(request)
	if len(paragraphs) == 0 {
		return request
	}
	sentence := paragraphs[0]
	if i := strings.IndexAny(sentence, ".?"); i > 0 {
		sentence = sentence[:i]
	}
	return strings.TrimSpace(sentence)
}

// ProposeAcceptanceLine is one verify-clean acceptance bullet from a short request.
func ProposeAcceptanceLine(request string) string {
	s := firstSentence(request)
	if s == "" {
		return ""
	}
	return "- " + s
}

// WritePhaseTasks fills each phase file from its paragraph. The named file from
// the answer is added once. A task that names some other file is not written.
func WritePhaseTasks(root, slug, answer string) (string, error) {
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(raw)
	req := sectionMarkdown(text, "User request")
	phases := parseRoadmap(sectionMarkdown(text, "Tasks"))
	allowed := fileTokens(req + "\n" + answer)
	named := firstFileToken(answer)
	var reply strings.Builder
	taskN := 0
	observed := sectionMarkdown(text, "Observed")
	for i, phase := range phases {
		paragraph := phaseParagraph(root, slug, phase, req)
		var lines []string
		taskNo := 0
		add := func(text string) {
			if taskNamesForeignFile(text, allowed) || namesOtherPhase(text, paragraph, phases) {
				return
			}
			taskNo++
			taskN++
			lines = append(lines, "- [ ] "+fmt.Sprintf("%d.%d %s", i+1, taskNo, text))
			lines = append(lines, fmt.Sprintf("{: id=%q}", phase.ID+"-task-"+fmt.Sprint(taskNo)))
		}
		file := filepath.Join(root, "sdd", "changes", slug, filepath.FromSlash(phase.File))
		existing, _ := os.ReadFile(file)
		kept, grounded := groundedCheckboxLines(withoutNeedsUserCheckboxes(string(existing)), paragraph)
		var steps []string
		if grounded {
			steps = kept
		} else {
			steps = groundedPhaseSteps(paragraph, goldfishPhaseTasks(paragraph, answer, observed, phases, allowed))
		}
		if i == 0 && failureShapedRequest(req) {
			hasRegression := false
			for _, line := range steps {
				if strings.Contains(line, "regression test") {
					hasRegression = true
					break
				}
			}
			if !hasRegression {
				steps = append([]string{RegressionTask()}, steps...)
			}
		}
		hasFile := false
		for _, line := range steps {
			if named != "" && strings.Contains(line, named) {
				hasFile = true
			}
			add(line)
		}
		if i == 0 && named != "" && tokenAllowed(named, allowed) && !hasFile {
			add("drawn in " + named)
		}
		body := "# " + strings.TrimPrefix(phase.Heading, "### ") + "\n\n" + strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			return "", err
		}
		reply.WriteString(phase.Heading + "\n" + strings.Join(lines, "\n") + "\n\n")
	}
	_ = taskN
	tasks := sectionMarkdown(text, "Tasks")
	tasks = dropBlock(tasks, "needs-user")
	text = replaceSection(text, "Tasks", strings.TrimSpace(tasks))
	if failureShapedRequest(req) && answerRecordsCommandResult(answer) {
		text = replaceSection(text, "Observed", strings.TrimSpace(answer))
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return strings.TrimSpace(reply.String()), nil
}

func goldfishPhaseTasks(paragraph, answer, observed string, phases []roadmapPhase, allowed map[string]bool) []string {
	if SecondReader == nil || strings.TrimSpace(paragraph) == "" {
		return nil
	}
	prompt := "Phase: " + paragraph + "\nAnswer: " + answer + "\nObserved: " + strings.TrimSpace(observed) + "\nWrite checkbox tasks that complete only this phase. Include the outcome in the words of the phase, the steps that produce it, and one check that fails when the phase is unmet. Start the check line with check:. Name a file only when the phase or the answer already names it. One task per line, starting with - ."
	text, err := SecondReader(prompt)
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- [ ]")
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimSpace(line)
		if line == "" || taskNamesForeignFile(line, allowed) || namesOtherPhase(line, paragraph, phases) {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func namesOtherPhase(line, paragraph string, phases []roadmapPhase) bool {
	lower := strings.ToLower(line)
	own := strings.ToLower(paragraph)
	for _, phase := range phases {
		title := strings.ToLower(strings.TrimPrefix(phase.Heading, "### Phase "))
		fields := strings.Fields(title)
		if len(fields) > 3 {
			fields = fields[:3]
		}
		needle := strings.Join(fields, " ")
		if len(needle) < 8 || strings.Contains(own, needle) || !strings.Contains(lower, needle) {
			continue
		}
		return true
	}
	return false
}

func taskNamesForeignFile(text string, allowed map[string]bool) bool {
	for _, tok := range fileTokenRE.FindAllString(text, -1) {
		if !tokenAllowed(tok, allowed) {
			return true
		}
	}
	return false
}

func phaseParagraph(root, slug string, phase roadmapPhase, request string) string {
	raw, err := os.ReadFile(filepath.Join(root, "sdd", "changes", slug, filepath.FromSlash(phase.File)))
	if err == nil {
		var lines []string
		for _, line := range strings.Split(string(raw), "\n") {
			trim := strings.TrimSpace(line)
			if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "- [") {
				continue
			}
			if _, _, _, ok := parseBlockLine(trim); ok {
				continue
			}
			lines = append(lines, trim)
		}
		if len(lines) > 0 {
			return strings.Join(lines, " ")
		}
	}
	title := strings.TrimPrefix(phase.Heading, "### Phase ")
	for _, paragraph := range requestParagraphs(request) {
		if phaseTitle(paragraph) == title {
			return paragraph
		}
	}
	return ""
}

func phaseFileBody(phase roadmapPhase, index int, steps []string) string {
	var b strings.Builder
	b.WriteString("# " + strings.TrimPrefix(phase.Heading, "### ") + "\n\n")
	for n, step := range steps {
		fmt.Fprintf(&b, "- [ ] %d.%d %s\n", index+1, n+1, step)
		fmt.Fprintf(&b, "{: id=%q}\n", phase.ID+"-task-"+fmt.Sprint(n+1))
	}
	return b.String()
}

// groundedPhaseSteps keeps a model line that shares a content word with the paragraph
// or adds a content word the paragraph does not. No such line becomes the paragraph's own clauses.
func groundedPhaseSteps(paragraph string, planned []string) []string {
	var kept []string
	for _, line := range planned {
		line = strings.TrimSpace(line)
		if !keepsPhaseLine(line, paragraph) {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == 0 {
		return clauseSteps(paragraph)
	}
	for _, line := range kept {
		if strings.Contains(strings.ToLower(line), "check:") {
			return kept
		}
	}
	return append(kept, "check: "+strings.TrimSpace(paragraph))
}

func keepsPhaseLine(line, paragraph string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if sharesContentWord(line, paragraph) {
		return true
	}
	have := map[string]bool{}
	for _, word := range contentWords(paragraph) {
		have[word] = true
	}
	for _, word := range contentWords(line) {
		if !have[word] {
			return true
		}
	}
	return false
}

func clauseSteps(paragraph string) []string {
	paragraph = strings.TrimSpace(paragraph)
	var steps []string
	parts := regexp.MustCompile(`[.;]`).Split(paragraph, -1)
	if len(parts) > 0 {
		if first := strings.TrimSpace(parts[0]); first != "" {
			steps = append(steps, first)
		}
	}
	steps = append(steps, extraSentences(paragraph)...)
	if len(steps) == 0 && paragraph != "" {
		steps = append(steps, paragraph)
	}
	if paragraph != "" {
		steps = append(steps, "check: "+paragraph)
	}
	return steps
}

func withoutNeedsUserCheckboxes(raw string) string {
	var b strings.Builder
	lines := strings.Split(raw, "\n")
	for i := 0; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "- [ ]") || strings.HasPrefix(trim, "- [x]") || strings.HasPrefix(trim, "- [X]") {
			if strings.HasPrefix(taskProse(trim), "needs user:") {
				if i+1 < len(lines) && strings.Contains(lines[i+1], "{: id=") {
					i++
				}
				continue
			}
		}
		b.WriteString(lines[i])
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func answerRecordsCommandResult(answer string) bool {
	if strings.Contains(answer, "`") {
		return true
	}
	lower := strings.ToLower(answer)
	return strings.Contains(lower, "http") || strings.Contains(lower, "exit")
}

// groundedCheckboxLines reports the checkbox text when every task already uses a word from the paragraph.
func groundedCheckboxLines(raw, paragraph string) ([]string, bool) {
	var steps []string
	for _, line := range strings.Split(raw, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "- [ ]") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(trim, "- [ ]"))
		fields := strings.Fields(body)
		if len(fields) > 0 && strings.Contains(fields[0], ".") {
			fields = fields[1:]
		}
		text := strings.Join(fields, " ")
		if text == "" || strings.Contains(text, "{: id=") || !keepsPhaseLine(text, paragraph) {
			continue
		}
		steps = append(steps, text)
	}
	return steps, len(steps) > 0
}

func sharesContentWord(line, paragraph string) bool {
	lower := strings.ToLower(line)
	for _, word := range contentWords(paragraph) {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func extraSentences(paragraph string) []string {
	parts := regexp.MustCompile(`[.;]`).Split(paragraph, -1)
	var out []string
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if len(contentWords(part)) >= 2 {
			out = append(out, part)
		}
	}
	return out
}

func fileTokens(text string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range fileTokenRE.FindAllString(text, -1) {
		out[strings.ToLower(tok)] = true
	}
	return out
}

func firstFileToken(text string) string {
	toks := fileTokenRE.FindAllString(text, -1)
	if len(toks) == 0 {
		return ""
	}
	return toks[0]
}

func tokenAllowed(tok string, allowed map[string]bool) bool {
	return allowed[strings.ToLower(tok)]
}

var fileTokenRE = regexp.MustCompile(`\b[\w./-]+\.[A-Za-z0-9]{1,4}\b`)

func parseRoadmap(tasks string) []roadmapPhase {
	var out []roadmapPhase
	var cur *roadmapPhase
	for _, line := range strings.Split(tasks, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "### ") {
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &roadmapPhase{Heading: trim}
			continue
		}
		if cur == nil {
			continue
		}
		if id, status, file, ok := parseBlockLine(trim); ok {
			if strings.HasSuffix(id, "-note") {
				continue
			}
			if id != "" {
				cur.ID = id
			}
			if status != "" {
				cur.Status = status
			}
			if file != "" {
				cur.File = file
			}
			continue
		}
		switch trim {
		case "review", "accepted":
			cur.Status = trim
		default:
			if strings.HasPrefix(trim, "phases/") && strings.HasSuffix(trim, ".md") {
				cur.File = trim
			}
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

func openRoadmapPhase(root, slug, tasks string) string {
	for _, phase := range parseRoadmap(tasks) {
		if phase.Status != "accepted" || phase.File == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, "sdd", "changes", slug, filepath.FromSlash(phase.File)))
		if err != nil {
			continue
		}
		if strings.Contains(string(raw), "- [ ]") {
			return phase.Heading
		}
	}
	return ""
}

func expandPhaseFiles(root, slug, tasks string) string {
	if !roadmapMarked(tasks) {
		return tasks
	}
	var b strings.Builder
	for _, phase := range parseRoadmap(tasks) {
		b.WriteString(phase.Heading + "\n")
		raw, err := os.ReadFile(filepath.Join(root, "sdd", "changes", slug, filepath.FromSlash(phase.File)))
		if err != nil {
			continue
		}
		b.WriteString(string(raw))
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return tasks
	}
	return b.String()
}

// AcceptFirstReviewPhase marks the first review phase accepted and leaves the rest.
func AcceptFirstReviewPhase(root, slug string) (string, error) {
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(raw)
	tasks := sectionMarkdown(text, "Tasks")
	replaced := false
	var b strings.Builder
	for _, line := range strings.Split(tasks, "\n") {
		trim := strings.TrimSpace(line)
		if !replaced && strings.Contains(trim, `status="review"`) {
			b.WriteString(strings.Replace(line, `status="review"`, `status="accepted"`, 1) + "\n")
			replaced = true
			continue
		}
		if !replaced && trim == "review" {
			b.WriteString("accepted\n")
			replaced = true
			continue
		}
		b.WriteString(line + "\n")
	}
	if !replaced {
		return "", fmt.Errorf("roadmap: no review phase")
	}
	text = replaceSection(text, "Tasks", strings.TrimSpace(b.String()))
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return openPhaseOf(root, slug), nil
}

func replaceSection(text, heading, body string) string {
	sectionBody := strings.TrimSpace(body)
	if id := sectionID(heading); id != "" && !strings.Contains(sectionBody, `id="`+id+`"`) {
		sectionBody = "{: id=\"" + id + "\"}\n" + sectionBody
	}
	section := "## " + heading + "\n\n" + sectionBody + "\n"
	start := strings.Index(text, "## "+heading)
	if start < 0 {
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return text + "\n" + section
	}
	rest := text[start+len("## "+heading):]
	next := nextKnownSection(rest)
	if next < 0 {
		return text[:start] + section
	}
	return text[:start] + section + rest[next+1:]
}

func stampSectionIDs(text string) string {
	for _, heading := range []string{"Why", "User request", "Acceptance", "Requirements", "Scenarios", "Decisions", "Risks", "Observed", "Assumptions", "Tasks", "Validate", "Non-goals", "Deferred", "Review"} {
		if !strings.Contains(text, "## "+heading) {
			continue
		}
		if strings.Contains(sectionMarkdown(text, heading), `id="`+sectionID(heading)+`"`) {
			continue
		}
		text = replaceSection(text, heading, strings.TrimSpace(sectionMarkdown(text, heading)))
	}
	return text
}

func sectionID(heading string) string {
	switch heading {
	case "Why":
		return "why"
	case "User request":
		return "user-request"
	case "Acceptance":
		return "acceptance"
	case "Requirements":
		return "requirements"
	case "Scenarios":
		return "scenarios"
	case "Decisions":
		return "decisions"
	case "Risks":
		return "risks"
	case "Review":
		return "review"
	case "Observed":
		return "observed"
	case "Assumptions":
		return "assumptions"
	case "Tasks":
		return "tasks"
	case "Validate":
		return "validate"
	case "Non-goals":
		return "non-goals"
	case "Deferred":
		return "deferred"
	default:
		return ""
	}
}

func knownSectionLine(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "## ") {
		return false
	}
	return sectionID(strings.TrimPrefix(line, "## ")) != ""
}

func nextKnownSection(rest string) int {
	offset := 0
	for _, line := range strings.Split(rest, "\n") {
		if offset > 0 && knownSectionLine(line) {
			return offset - 1
		}
		offset += len(line) + 1
	}
	return -1
}
