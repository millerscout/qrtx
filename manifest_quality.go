package qrtx

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"
)

// pathTokenRE finds path-shaped tokens in plan prose: a slash, or a name with a letter-bearing extension.
var pathTokenRE = regexp.MustCompile(`[A-Za-z0-9_./~-]+`)

var fileExtRE = regexp.MustCompile(`^[^/]{2,}\.[A-Za-z0-9]*[A-Za-z][A-Za-z0-9]*$`)

// dependencyManifests never ground Observed: every repository has them, so citing one shows no reading.
var dependencyManifests = map[string]bool{
	"go.mod": true, "go.sum": true, "package.json": true, "package-lock.json": true,
	"yarn.lock": true, "pnpm-lock.yaml": true, "Cargo.toml": true, "Cargo.lock": true,
	"pyproject.toml": true, "poetry.lock": true, "requirements.txt": true, "Gemfile": true, "Gemfile.lock": true,
}

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
	// TasksPathsTotal counts distinct file paths named in task prose (before `verify:`),
	// phase files included; TasksPathsObserved counts those also cited in Observed.
	// Both are reported, never added to Score.
	TasksPathsTotal    int
	TasksPathsObserved int
}

// ScoreJSON is the map `change score` and `finish-propose` print, so both keep the same keys.
func ScoreJSON(sc ManifestScore) map[string]any {
	return map[string]any{
		"slug":                 sc.Slug,
		"score":                sc.Score,
		"max_score":            sc.MaxScore,
		"verify_ok":            sc.VerifyOK,
		"verify_err":           sc.VerifyErr,
		"spec_depth":           sc.Depth,
		"tasks_paths_total":    sc.TasksPathsTotal,
		"tasks_paths_observed": sc.TasksPathsObserved,
	}
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
	out.TasksPathsTotal, out.TasksPathsObserved = taskPathCounts(expandPhaseFiles(repoRoot, slug, impl), observed)
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

// taskPathCounts counts distinct paths named in checkbox task prose before `verify:`,
// leaving out roadmap phase-file links, and how many of them Observed also cites.
func taskPathCounts(tasks, observed string) (int, int) {
	cited := map[string]bool{}
	for _, tok := range planPathTokens(observed) {
		cited[tok] = true
	}
	seen := map[string]bool{}
	total, hit := 0, 0
	for _, line := range checkboxLines(tasks) {
		prose := taskProse(line)
		if i := strings.Index(strings.ToLower(prose), "verify:"); i >= 0 {
			prose = prose[:i]
		}
		for _, tok := range planPathTokens(prose) {
			if seen[tok] || strings.HasPrefix(tok, "phases/") {
				continue
			}
			seen[tok] = true
			total++
			if cited[tok] {
				hit++
			}
		}
	}
	return total, hit
}

func observedPathExists(repoRoot, observed string) bool {
	for _, tok := range planPathTokens(observed) {
		if groundedFile(repoRoot, tok) {
			return true
		}
	}
	return false
}

// planPathTokens returns path-shaped tokens from prose, cleaned to slash form, in order and without repeats.
// Absolute paths, home paths, URLs and dot-only tokens are kept out.
func planPathTokens(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range pathTokenRE.FindAllString(text, -1) {
		tok := strings.Trim(raw, ".,;:")
		if tok == "" || strings.HasPrefix(raw, "/") || strings.HasPrefix(tok, "~") || strings.Contains(tok, "...") {
			continue
		}
		if !strings.Contains(tok, "/") && !fileExtRE.MatchString(tok) {
			continue
		}
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(tok)))
		if clean == "." || clean == ".." {
			continue
		}
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	return out
}

// groundedFile reports whether rel names a regular source file inside repoRoot, after symlinks,
// that is neither a plan file under sdd/ nor a dependency manifest.
func groundedFile(repoRoot, rel string) bool {
	if filepath.IsAbs(rel) || excludedFromGrounding(rel) {
		return false
	}
	target := filepath.Join(repoRoot, filepath.FromSlash(rel))
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return false
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return false
	}
	inside, err := filepath.Rel(realRoot, realTarget)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return false
	}
	return !excludedFromGrounding(filepath.ToSlash(inside))
}

func excludedFromGrounding(rel string) bool {
	if rel == "sdd" || strings.HasPrefix(rel, "sdd/") || strings.HasPrefix(rel, "../") || rel == ".." {
		return true
	}
	return dependencyManifests[pathBase(rel)]
}

func pathBase(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[i+1:]
	}
	return rel
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
