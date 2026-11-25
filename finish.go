package sdd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var validateCmdBacktickRE = regexp.MustCompile("`([^`]+)`")

// FinishPropose runs propose-context (warn-only), manifest verify, and score.
func FinishPropose(repoRoot, slug string, stdout, stderr io.Writer) error {
	slug = strings.TrimSpace(slug)
	if !ValidChangeSlug(slug) {
		return fmt.Errorf("invalid change slug %q", slug)
	}
	rep, err := ProposeContext(repoRoot, slug, ProposeContextOpts{})
	if err != nil {
		return err
	}
	for _, w := range rep.Warnings {
		_, _ = fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	if err := ValidateChangeManifest(repoRoot, slug); err != nil {
		return fmt.Errorf("change verify: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "sdd change verify: OK (%s)\n", slug)
	if err := proposePhasesReady(repoRoot, slug, len(rep.Overlaps) > 0); err != nil {
		return err
	}
	raw, err := os.ReadFile(changeFilePath(repoRoot, slug))
	if err != nil {
		return err
	}
	if err := proposeDiagnosticReady(string(raw)); err != nil {
		return err
	}
	sc, err := ScoreChangeManifest(repoRoot, slug)
	if err != nil {
		return err
	}
	out := map[string]any{
		"slug":       sc.Slug,
		"score":      sc.Score,
		"max_score":  sc.MaxScore,
		"verify_ok":  sc.VerifyOK,
		"verify_err": sc.VerifyErr,
		"spec_depth": sc.Depth,
	}
	enc := json.NewEncoder(stdout)
	return enc.Encode(out)
}

func proposePhasesReady(repoRoot, slug string, overlap bool) error {
	raw, err := os.ReadFile(changeFilePath(repoRoot, slug))
	if err != nil {
		return err
	}
	text := string(raw)
	req := sectionMarkdown(text, "User request")
	band := proposeBand(req, groundedAcceptanceBullets(req, sectionMarkdown(text, "Acceptance")), overlap)
	if band != "context" && band != "goldfish" {
		return nil
	}
	tasks := expandPhaseFiles(repoRoot, slug, sectionMarkdown(text, "Tasks"))
	corpus := req + "\n" + sectionMarkdown(text, "Acceptance") + "\n" + sectionMarkdown(text, "Observed")
	fault, bad := findPhaseFault(tasks, corpus)
	if !bad {
		return nil
	}
	return fmt.Errorf("phases: %s plan is not ready: %s %s task %s token %s", band, fault.check, fault.heading, fault.task, fault.token)
}

var failureRequestRE = regexp.MustCompile(`(?i)\b(drops?|dropped|empty|old script|fails?|failed|failure|broken|not working)\b`)

func failureShapedRequest(req string) bool {
	return failureRequestRE.MatchString(req)
}

func observedHasCommandResult(observed string) bool {
	if strings.Contains(observed, "`") || strings.Contains(observed, "/") {
		return true
	}
	lower := strings.ToLower(observed)
	return strings.Contains(lower, "http") || strings.Contains(lower, "exit")
}

func proposeDiagnosticReady(text string) error {
	if !failureShapedRequest(sectionMarkdown(text, "User request")) {
		return nil
	}
	if observedHasCommandResult(sectionMarkdown(text, "Observed")) {
		return nil
	}
	return fmt.Errorf("diagnostic: failure request has no command result in Observed")
}

// FinishApply runs verify, scenario coverage, verify-ast when packages are listed, then run-validate.
func FinishApply(repoRoot, slug string, stdout, stderr io.Writer) error {
	slug = strings.TrimSpace(slug)
	if err := ValidateChangeManifest(repoRoot, slug); err != nil {
		return fmt.Errorf("change verify: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "sdd change verify: OK (%s)\n", slug)
	raw, err := os.ReadFile(changeFilePath(repoRoot, slug))
	if err != nil {
		return err
	}
	if err := checkScenarioCoverage(repoRoot, slug, string(raw)); err != nil {
		return fmt.Errorf("scenario coverage: %w", err)
	}
	man, err := ParseChangeFile(repoRoot, slug)
	if err != nil {
		return err
	}
	if len(man.Packages) > 0 {
		if err := VerifyAST(stderr, repoRoot, man.Packages); err != nil {
			return err
		}
	}
	return RunValidateTasks(repoRoot, slug, stdout, stderr)
}

// RunValidateTasks executes unchecked Validate harness lines (allowlisted).
func RunValidateTasks(repoRoot, slug string, stdout, stderr io.Writer) error {
	path := changeFilePath(repoRoot, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	boxes := parseValidateCheckboxes(string(raw))
	escape := firstEnv(RunValidateEnv) == "1"
	cfg, err := LoadRepoConfig(repoRoot)
	if err != nil {
		return err
	}
	ran, skipped := 0, 0
	for _, box := range boxes {
		if box.done {
			continue
		}
		cmd, skip, err := extractValidateCommand(box.text)
		if err != nil {
			return err
		}
		if skip {
			skipped++
			_, _ = fmt.Fprintf(stderr, "run-validate: skip (manual): %s\n", truncateRunes(box.text, 80))
			continue
		}
		if cmd == "" {
			return fmt.Errorf("run-validate: no command in validate task: %s", truncateRunes(box.text, 120))
		}
		if !allowedValidateCommand(cmd, escape, cfg.ValidateAllow) {
			return fmt.Errorf("run-validate: command not allowlisted (set "+EnvRunValidate+"=1 to override): %s", cmd)
		}
		_, _ = fmt.Fprintf(stderr, "run-validate: %s\n", cmd)
		c := exec.Command("bash", "-lc", cmd)
		c.Dir = repoRoot
		c.Stdout = stdout
		c.Stderr = stderr
		c.Env = os.Environ()
		if err := c.Run(); err != nil {
			return fmt.Errorf("run-validate: %w", err)
		}
		ran++
	}
	_, _ = fmt.Fprintf(stdout, "run-validate: OK (%s) ran=%d skipped=%d\n", slug, ran, skipped)
	return nil
}

type validateCheckbox struct {
	done bool
	text string
}

func parseValidateCheckboxes(md string) []validateCheckbox {
	section := sectionMarkdown(md, "Validate")
	out := []validateCheckbox{}
	for _, line := range strings.Split(section, "\n") {
		m := sddCheckboxRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		done := strings.EqualFold(m[1], "x")
		out = append(out, validateCheckbox{done: done, text: strings.TrimSpace(m[2])})
	}
	return out
}

func extractValidateCommand(taskText string) (cmd string, skip bool, err error) {
	lower := strings.ToLower(taskText)
	if strings.Contains(lower, "manual:") {
		return "", true, nil
	}
	if isTemplateValidateTask(taskText) {
		return "", false, fmt.Errorf("validate task still template/TBD")
	}
	taskText = stripScenarioCoverage(taskText)
	if m := validateCmdBacktickRE.FindStringSubmatch(taskText); len(m) >= 2 {
		return strings.TrimSpace(m[1]), false, nil
	}
	// Bare command after V1.x prefix (no backticks).
	trim := strings.TrimSpace(taskText)
	if idx := strings.Index(trim, " "); idx > 0 && strings.HasPrefix(trim, "V") {
		rest := strings.TrimSpace(trim[idx+1:])
		if rest != "" && !strings.HasPrefix(strings.ToLower(rest), "optional") {
			return rest, false, nil
		}
	}
	return "", false, nil
}

var scenarioCoverageRE = regexp.MustCompile(`(?i)\s*[(\[—–:;,-]*\s*covers\s+S\d+(?:\s*,\s*S\d+)*\s*[)\]]?\s*$`)

// stripScenarioCoverage drops a trailing `covers S1,S2` clause so it is never run as part of a bare command.
func stripScenarioCoverage(taskText string) string {
	return strings.TrimSpace(scenarioCoverageRE.ReplaceAllString(taskText, ""))
}

// allowedValidateCommand reports whether run-validate may execute cmd.
// extra holds the repo's validate_allow prefixes.
func allowedValidateCommand(cmd string, escape bool, extra []string) bool {
	if escape {
		return true
	}
	cmd = strings.TrimSpace(cmd)
	// Strip leading env assignments for prefix check.
	check := cmd
	for {
		eq := strings.Index(check, "=")
		if eq < 0 {
			break
		}
		sp := strings.Index(check, " ")
		if sp < 0 || sp < eq {
			break
		}
		check = strings.TrimSpace(check[sp+1:])
	}
	allowed := []string{
		"go test ",
		"go run ",
		"python3 -m unittest ",
		"python3 scripts/",
		CLIName + " ",
		"./" + CLIName + " ",
	}
	allowed = append(allowed, extra...)
	for _, p := range allowed {
		if strings.HasPrefix(check, p) {
			return true
		}
	}
	for _, name := range RepoEnv {
		if strings.HasPrefix(cmd, name+"=") {
			return allowedValidateCommand(check, false, extra)
		}
	}
	if strings.HasPrefix(cmd, "PYTHONPATH=") {
		return allowedValidateCommand(check, false, extra)
	}
	return false
}
