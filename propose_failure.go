package sdd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const regressionAcceptance = "- A regression test fails on the reported failure and passes after the fix."

const regressionTask = "Add a regression test that fails on the reported failure and passes after the fix."

const regressionWhy = "The chosen approach is a regression that fails on the reported failure and passes after the fix. The approach set aside is leaving the plan on an open question."

// FailureShapedRequest reports a request the propose writers decide instead of asking.
func FailureShapedRequest(req string) bool {
	return failureShapedRequest(req)
}

// RegressionAcceptance is the decided acceptance bullet for a failure-shaped request.
func RegressionAcceptance() string {
	return regressionAcceptance
}

// RegressionTask is the first task for a failure-shaped request.
func RegressionTask() string {
	return regressionTask
}

// RegressionWhy names the chosen regression and the open question that was set aside.
func RegressionWhy() string {
	return regressionWhy
}

// RegressionValidateCommand is go test for the named file's directory, or the whole module.
func RegressionValidateCommand(request, observed string) string {
	path := FirstRequestPath(request)
	if path == "" {
		path = FirstRequestPath(observed)
	}
	path = strings.TrimPrefix(path, "./")
	if path == "" {
		return "go test ./... -count=1"
	}
	dir := filepath.ToSlash(filepath.Dir(path))
	dir = strings.Trim(dir, "/")
	if dir == "." || dir == "" {
		return "go test ./... -count=1"
	}
	return "go test ./" + dir + "/ -count=1"
}

// SiblingRequestSlug is the other active change with the same first sentence.
// The oldest change file wins. Equal times use the lexicographically smaller slug.
func SiblingRequestSlug(repoRoot, slug, request string) (string, error) {
	want := storedSentence(request)
	if strings.TrimSpace(want) == "" {
		return "", nil
	}
	slugs, err := ListChanges(repoRoot)
	if err != nil {
		return "", err
	}
	type match struct {
		slug string
		mod  time.Time
	}
	var matches []match
	for _, other := range slugs {
		if other == slug {
			continue
		}
		path := changeFilePath(repoRoot, other)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if storedSentence(sectionMarkdown(string(raw), "User request")) != want {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		matches = append(matches, match{slug: other, mod: info.ModTime()})
	}
	if len(matches) == 0 {
		return "", nil
	}
	sort.Slice(matches, func(i, j int) bool {
		if !matches[i].mod.Equal(matches[j].mod) {
			return matches[i].mod.Before(matches[j].mod)
		}
		return matches[i].slug < matches[j].slug
	})
	return matches[0].slug, nil
}

// MarkQuestionOnlyPlanning sets status planning on a phase whose only checkbox is an open question.
func MarkQuestionOnlyPlanning(tasks string) string {
	parts := strings.Split(tasks, "### ")
	if len(parts) < 2 {
		return tasks
	}
	var b strings.Builder
	b.WriteString(parts[0])
	for _, part := range parts[1:] {
		if questionOnlyBlock(part) {
			part = strings.Replace(part, `status="review"`, `status="planning"`, 1)
		}
		b.WriteString("### ")
		b.WriteString(part)
	}
	return b.String()
}

func storedSentence(section string) string {
	var kept []string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "{:") {
			continue
		}
		kept = append(kept, line)
	}
	return firstSentence(strings.Join(kept, "\n"))
}

func questionOnlyBlock(block string) bool {
	n := 0
	needs := false
	for _, line := range strings.Split(block, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "- [") {
			continue
		}
		n++
		if strings.HasPrefix(taskProse(trim), "needs user:") {
			needs = true
		}
	}
	return n == 1 && needs
}

// requestOverlapWarning is the propose-context warning for a duplicate first sentence.
func requestOverlapWarning(slug string) string {
	return fmt.Sprintf("request overlap with %q", slug)
}
