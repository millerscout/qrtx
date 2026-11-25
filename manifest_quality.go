package sdd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"
)

var observedFileRE = regexp.MustCompile(`(?:internal|scripts|docs|homepage)/[A-Za-z0-9_./-]+`)

var validateHarnessRE = regexp.MustCompile(`(?i)(go test|unittest|golden|manual:)`)

// ManifestScore is a weighted rubric result for change.sdd.md intent-first quality.
type ManifestScore struct {
	Slug             string
	Score            int
	MaxScore         int
	VerifyOK         bool
	VerifyErr        string
	ObservedGrounded bool
	// Depth counts Scenarios, Decisions and Risks; it is reported, never added to Score.
	Depth SpecDepth
}

// ScoreChangeManifest scores manifest quality; ValidateChangeManifest rules define verify pass.
func ScoreChangeManifest(repoRoot, slug string) (ManifestScore, error) {
	path := changeFilePath(repoRoot, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		return ManifestScore{}, err
	}
	text := string(raw)
	out := ManifestScore{Slug: slug, MaxScore: 100}
	out.Depth = computeSpecDepth(repoRoot, slug, text)

	verifyErr := ValidateChangeManifest(repoRoot, slug)
	if verifyErr != nil {
		out.VerifyOK = false
		out.VerifyErr = verifyErr.Error()
	} else {
		out.VerifyOK = true
	}

	score := 0
	userReq := strings.TrimSpace(sectionMarkdown(text, "User request"))
	if userReq != "" && !strings.Contains(userReq, "(TBD") {
		score += 25
	}
	failure := failureShapedRequest(userReq)
	sentence := firstSentence(userReq)
	acceptance := sectionMarkdown(text, "Acceptance")
	acceptBullets := 0
	copiedAcceptance := 0
	for _, line := range strings.Split(acceptance, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "- ") || strings.Contains(trim, "(TBD") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
		if failure && body == sentence {
			copiedAcceptance++
			continue
		}
		if !failure && AcceptanceEcho(body, userReq) {
			copiedAcceptance++
			continue
		}
		acceptBullets++
	}
	if acceptBullets >= 1 {
		score += 25
	} else if copiedAcceptance > 0 || (strings.TrimSpace(acceptance) != "" && !strings.Contains(acceptance, "(TBD")) {
		score += 10
	}
	validate := sectionMarkdown(text, "Validate")
	realValidate := 0
	for _, line := range checkboxLines(validate) {
		if failure && manualConfirmOfRequest(line, sentence) {
			continue
		}
		if !failure && manualRequestEcho(line, userReq) {
			continue
		}
		if !isTemplateValidateTask(line) && validateHarnessRE.MatchString(line) {
			realValidate++
		}
	}
	if realValidate >= 1 {
		score += 25
	} else if len(checkboxLines(validate)) > 0 {
		score += 10
	}
	observed := sectionMarkdown(text, "Observed")
	if observedPathExists(repoRoot, observed) {
		out.ObservedGrounded = true
		score += 15
	} else if strings.TrimSpace(observed) != "" && !strings.Contains(observed, "(TBD") {
		score += 5
	}
	impl := sectionMarkdown(text, "Tasks")
	nonTemplateImpl := 0
	for _, line := range checkboxLines(impl) {
		if strings.HasPrefix(taskProse(line), "needs user:") {
			continue
		}
		if !isTemplateImplTask(line) {
			nonTemplateImpl++
		}
	}
	if nonTemplateImpl > 0 && out.VerifyOK {
		score += 10
	}
	if score > out.MaxScore {
		score = out.MaxScore
	}
	out.Score = score
	return out, nil
}

func manualConfirmOfRequest(line, sentence string) bool {
	lower := strings.ToLower(line)
	if sentence == "" || !strings.Contains(lower, "manual:") {
		return false
	}
	return strings.Contains(lower, strings.ToLower(sentence))
}

func observedPathExists(repoRoot, observed string) bool {
	for _, match := range observedFileRE.FindAllString(observed, -1) {
		rel := strings.TrimRight(match, ".,);")
		if rel == "" {
			continue
		}
		info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// LintActiveChanges prints verify and score for each active change; always succeeds.
func LintActiveChanges(repoRoot string, w io.Writer) error {
	slugs, err := ListChanges(repoRoot)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "slug\tverify\tscore\tmax")
	for _, slug := range slugs {
		sc, err := ScoreChangeManifest(repoRoot, slug)
		if err != nil {
			_, _ = fmt.Fprintf(tw, "%s\terror\t0\t100\n", slug)
			continue
		}
		verify := "ok"
		if !sc.VerifyOK {
			verify = "fail"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\n", slug, verify, sc.Score, sc.MaxScore)
	}
	return tw.Flush()
}
