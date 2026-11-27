package qrtx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGroundedAcceptanceBand(t *testing.T) {
	vague := "a one room game"
	invented := "- The player wins the match\n- The theme is forest\n- The room has a door\n- (TBD — checks.)\n"
	if n := groundedAcceptanceBullets(vague, invented); n != 0 {
		t.Fatalf("invented bullets counted %d", n)
	}
	if proposeBand(vague, groundedAcceptanceBullets(vague, invented), false) != "context" {
		t.Fatal("invented bullets changed the band")
	}
	stated := "related queries return nothing and search returns the same row twice"
	checks := "- related queries return nothing\n- search returns the same row twice\n"
	if proposeBand(stated, groundedAcceptanceBullets(stated, checks), false) != "goldfish" {
		t.Fatal("stated checks should be goldfish")
	}
	if proposeBand("Fix the comment in internal/codegraph/db.go", groundedAcceptanceBullets("Fix the comment in internal/codegraph/db.go", "- Fix the comment in internal/codegraph/db.go"), false) != "simple" {
		t.Fatal("named file and one check should be simple")
	}
}

func TestGoldfishPhaseSteps(t *testing.T) {
	stubDeterministicRoadmapClassify(t)
	prev := SecondReader
	SecondReader = func(string) (string, error) {
		return "- keep the lights on\n- wire the switch\n- edit engine.go\n- stone age threats\n", nil
	}
	t.Cleanup(func() { SecondReader = prev })
	root := t.TempDir()
	slug := "lights"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	request := "keep the lights on\nstone age threats stay out\na third feature for the map\n"
	reply, err := WriteRoadmap(root, slug, request, "score: context\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reply, "- [ ]") {
		t.Fatalf("roadmap listed tasks %s", reply)
	}
	if _, err := WritePhaseTasks(root, slug, "drawn in room.txt"); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(root, "sdd", "changes", slug, "phases")
	files, err := filepath.Glob(filepath.Join(first, "01-*.md"))
	if err != nil || len(files) != 1 {
		t.Fatal(files)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	phase := string(raw)
	if strings.Count(phase, "- [ ]") < 2 || !strings.Contains(phase, "check:") || !strings.Contains(phase, "keep the lights on") || !strings.Contains(phase, "room.txt") || strings.Contains(phase, "wire the switch") {
		t.Fatalf("phase %s", phase)
	}
	if strings.Contains(phase, "engine.go") || strings.Contains(phase, "stone age threats") {
		t.Fatalf("foreign task kept %s", phase)
	}
	if strings.TrimSpace(phase) == "keep the lights on" {
		t.Fatal("phase copied the sentence")
	}
	assertPhasesKeepSentences(t, root, slug, []string{
		"keep the lights on",
		"stone age threats stay out",
		"a third feature for the map",
	})

	SecondReader = func(string) (string, error) {
		return "- build an event bus\n- diagnostic scripts\n- Outcome: the droids needs maintenance\n", nil
	}
	cityRoot := t.TempDir()
	citySlug := "city-game"
	if err := NewChange(cityRoot, citySlug); err != nil {
		t.Fatal(err)
	}
	city := "the game is a city management game\nlow poly top-down survival\ndroids need maintenance\nstone age threats\na treadmill for explorers\ntimed events raise difficulty\n"
	if _, err := WriteRoadmap(cityRoot, citySlug, city, "score: context\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePhaseTasks(cityRoot, citySlug, "drawn in room.txt"); err != nil {
		t.Fatal(err)
	}
	cityFiles, err := filepath.Glob(filepath.Join(cityRoot, "sdd", "changes", citySlug, "phases", "*.md"))
	if err != nil || len(cityFiles) < 3 {
		t.Fatal(cityFiles)
	}
	for _, file := range cityFiles {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		body := string(raw)
		if strings.Count(body, "- [ ]") < 2 || !strings.Contains(body, "check:") || strings.Contains(body, "event bus") || strings.Contains(body, "diagnostic scripts") {
			t.Fatalf("shallow phase %s", body)
		}
	}
	assertPhasesKeepSentences(t, cityRoot, citySlug, []string{
		"the game is a city management game",
		"low poly top-down survival",
		"droids need maintenance",
		"stone age threats",
		"a treadmill for explorers",
		"timed events raise difficulty",
	})
	busFiles, err := filepath.Glob(filepath.Join(cityRoot, "sdd", "changes", citySlug, "phases", "03-*.md"))
	if err != nil || len(busFiles) != 1 {
		t.Fatal(busFiles)
	}
	bus := "# Phase droids need maintenance\n\n- [ ] 3.1 Define event types and configure the bus\n{: id=\"phase-03-task-1\"}\n- [ ] 3.2 diagnostic scripts\n{: id=\"phase-03-task-2\"}\n"
	if err := os.WriteFile(busFiles[0], []byte(bus), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePhaseTasks(cityRoot, citySlug, "drawn in room.txt"); err != nil {
		t.Fatal(err)
	}
	rewritten, err := os.ReadFile(busFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rewritten), "Define event types") || !strings.Contains(string(rewritten), "diagnostic scripts") {
		t.Fatalf("outcome lines dropped %s", rewritten)
	}
	left, err := filepath.Glob(filepath.Join(cityRoot, "sdd", "changes", citySlug, "phases", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range left {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "needs user:") {
			t.Fatalf("question kept %s", raw)
		}
	}
}

func assertPhasesKeepSentences(t *testing.T, root, slug string, paragraphs []string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "sdd", "changes", slug, "phases", "*.md"))
	if err != nil || len(files) != len(paragraphs) {
		t.Fatalf("phases %v", files)
	}
	for i, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		body := string(raw)
		var boxes []string
		for _, line := range strings.Split(body, "\n") {
			trim := strings.TrimSpace(line)
			if !strings.HasPrefix(trim, "- [ ]") {
				continue
			}
			if strings.Contains(trim, "{: id=") {
				t.Fatalf("task line is an id %s", body)
			}
			boxes = append(boxes, strings.ToLower(trim))
		}
		if len(boxes) < 2 || !strings.Contains(body, "check:") {
			t.Fatalf("shallow phase %s", body)
		}
		joined := strings.Join(boxes, " ")
		hit := false
		for _, word := range contentWords(paragraphs[i]) {
			if strings.Contains(joined, word) {
				hit = true
				break
			}
		}
		if !hit {
			t.Fatalf("task lost %q\n%s", paragraphs[i], body)
		}
	}
}

func TestOutcomePhaseLineKept(t *testing.T) {
	root := t.TempDir()
	slug := "tic-tac-toe"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sdd", "changes", slug, "change.sdd.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := replaceSection(string(raw), "User request", "we need a tic tac toe game")
	text = replaceSection(text, "Tasks", "### Phase board\n{: id=\"phase-01\" status=\"review\" file=\"phases/01-board.md\"}")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "sdd", "changes", slug, "phases")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := "Show a draw when the board is full"
	phasePath := filepath.Join(dir, "01-board.md")
	if err := os.WriteFile(phasePath, []byte("# Phase board\n\n- [ ] 1.1 "+line+"\n{: id=\"phase-01-task-1\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePhaseTasks(root, slug, ""); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(phasePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), line) {
		t.Fatalf("dropped %s", got)
	}
}

func TestOpenQuestion(t *testing.T) {
	if got := OpenQuestion("the search drops results"); got != "needs user: what did the failing check print?" {
		t.Fatalf("failure %q", got)
	}
	two := "- keep the header\n- show one switch\n"
	if got := OpenQuestion(two); got != "needs user: which check belongs in this change?" {
		t.Fatalf("checks %q", got)
	}
	if got := OpenQuestion("tic tac toe"); got != `needs user: what outcome should "tic tac toe" deliver?` {
		t.Fatalf("outcome %q", got)
	}
	if strings.Contains(OpenQuestion("ship the buffer"), "where the game is drawn") {
		t.Fatal("canned game question")
	}
}

func TestRoadmapOpenQuestion(t *testing.T) {
	stubDeterministicRoadmapClassify(t)
	root := t.TempDir()
	slug := "ship-buffer"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	request := "ship the buffer\nkeep the lights on\nsave the map\n"
	if _, err := WriteRoadmap(root, slug, request, ""); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, "sdd", "changes", slug, "phases", "01-*.md"))
	if err != nil || len(files) != 1 {
		t.Fatal(files)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	want := `needs user: what outcome should "ship the buffer" deliver?`
	first := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- [ ]") {
			first = line
			break
		}
	}
	if !strings.Contains(first, want) {
		t.Fatalf("first task %q\n%s", first, body)
	}
	all, err := os.ReadFile(changeFilePath(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(all), "where the game is drawn") || strings.Contains(body, "where the game is drawn") {
		t.Fatal("canned game question")
	}
	var out, errOut strings.Builder
	err = FinishPropose(root, slug, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("finish %v out=%s", err, out.String())
	}
}

func TestRoadmapFailureAnswer(t *testing.T) {
	stubDeterministicRoadmapClassify(t)
	root := t.TempDir()
	slug := "search-drops"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	request := "the search drops results\nkeep the log\nnote the time\n"
	if _, err := WriteRoadmap(root, slug, request, ""); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(changeFilePath(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sectionMarkdown(string(manifest), "Acceptance"), "A regression test fails on the reported failure and passes after the fix.") {
		t.Fatalf("acceptance %s", manifest)
	}
	if !strings.Contains(sectionMarkdown(string(manifest), "Validate"), "go test") {
		t.Fatalf("validate %s", manifest)
	}
	files, err := filepath.Glob(filepath.Join(root, "sdd", "changes", slug, "phases", "*.md"))
	if err != nil || len(files) < 2 {
		t.Fatal(files)
	}
	joined := ""
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "needs user:") {
			t.Fatalf("question before answer %s", raw)
		}
		joined += string(raw)
	}
	if !strings.Contains(joined, "regression test") || !strings.Contains(joined, "keep the log") || !strings.Contains(joined, "note the time") {
		t.Fatalf("phases %s", joined)
	}
	var earlyOut, earlyErr strings.Builder
	err = FinishPropose(root, slug, &earlyOut, &earlyErr)
	if err == nil || !strings.Contains(err.Error(), "command result") || strings.Contains(err.Error(), "needs user:") {
		t.Fatalf("early finish %v", err)
	}
	answer := "`go test` exited 1"
	if _, err := WritePhaseTasks(root, slug, answer); err != nil {
		t.Fatal(err)
	}
	files, err = filepath.Glob(filepath.Join(root, "sdd", "changes", slug, "phases", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "needs user:") {
			t.Fatalf("question kept %s", raw)
		}
	}
	manifest, err = os.ReadFile(changeFilePath(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sectionMarkdown(string(manifest), "Observed"), answer) {
		t.Fatalf("observed %s", manifest)
	}
	var doneOut, doneErr strings.Builder
	if err := FinishPropose(root, slug, &doneOut, &doneErr); err != nil {
		t.Fatalf("finish %v %s", err, doneErr.String())
	}
}

func TestRoadmapPhaseFiles(t *testing.T) {
	stubDeterministicRoadmapClassify(t)
	root := t.TempDir()
	slug := "city-game"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	request := "the game is a city management game\nlow poly top-down survival\ndroids recharge in sunlight unless a sandstorm blocks it\ndroids need maintenance\nstone age threats\nthe humans arrive later\na treadmill for explorers\ntimed events raise difficulty\nsave the map in mystery.bin\n"
	reply, err := WriteRoadmap(root, slug, request, "score: context\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reply, "- [ ]") || strings.Contains(reply, "finish-propose") {
		t.Fatalf("first pass listed tasks %s", reply)
	}
	if !strings.Contains(reply, "### Phase") || !strings.Contains(reply, "review") || !strings.Contains(reply, "needs user:") {
		t.Fatalf("roadmap %s", reply)
	}
	if !strings.Contains(reply, "humans arrive later") {
		t.Fatalf("humans not deferred %s", reply)
	}
	manifest, err := os.ReadFile(changeFilePath(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "### Phase") && strings.Contains(string(manifest), "humans arrive later") && strings.Contains(sectionMarkdown(string(manifest), "Tasks"), "humans") {
		t.Fatal("humans stayed in the task roadmap")
	}
	if !strings.Contains(string(manifest), "needs user:") || !strings.Contains(sectionMarkdown(string(manifest), "Deferred"), "humans arrive later") {
		t.Fatal("deferred section")
	}
	if strings.Count(sectionMarkdown(string(manifest), "Tasks"), "### Phase") < 3 {
		t.Fatalf("phases %s", manifest)
	}
	if openPhaseOf(root, slug) != "" {
		t.Fatal("review phase became the open phase")
	}

	detail, err := WritePhaseTasks(root, slug, "drawn in room.txt and engine.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, "room.txt") || strings.Contains(detail, "engine.go") || !strings.Contains(detail, "mystery.bin") {
		t.Fatalf("detail %s", detail)
	}
	prog := ChangeProgress(root, slug)
	if prog.Total <= 3 {
		t.Fatalf("progress %d", prog.Total)
	}
	phases, err := filepath.Glob(filepath.Join(root, "sdd", "changes", slug, "phases", "*.md"))
	if err != nil || len(phases) < 2 {
		t.Fatal(phases)
	}
	before := map[string]string{}
	for _, file := range phases {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		before[file] = string(raw)
	}
	if openPhaseOf(root, slug) != "" {
		t.Fatal("detail marked a phase accepted")
	}
	heading, err := AcceptFirstReviewPhase(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	if heading == "" || openPhaseOf(root, slug) != heading {
		t.Fatalf("open %q heading %q", openPhaseOf(root, slug), heading)
	}
	changed := 0
	for file, raw := range before {
		now, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(now) != raw {
			changed++
		}
	}
	if changed != 0 {
		t.Fatal("accept rewrote a phase file")
	}
	later := phases[len(phases)-1]
	if err := os.Remove(later); err != nil {
		t.Fatal(err)
	}
	if openPhaseOf(root, slug) != heading {
		t.Fatal("open phase required a later phase file")
	}
}
