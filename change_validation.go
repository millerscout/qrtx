package qrtx

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var sddCheckboxRE = regexp.MustCompile(`^-\s+\[([ xX])\]\s+(.*)$`)

// ValidateChangeManifest checks intent-first sections and minimal Validate tasks.
func ValidateChangeManifest(repoRoot, slug string) error {
	path := changeFilePath(repoRoot, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(raw)
	userReq := sectionMarkdown(text, "User request")
	if strings.TrimSpace(userReq) == "" {
		return fmt.Errorf("%s: missing User request section", slug)
	}
	if strings.Contains(userReq, "(TBD") {
		return fmt.Errorf("%s: User request still contains TBD", slug)
	}
	acceptance := sectionMarkdown(text, "Acceptance")
	if strings.TrimSpace(acceptance) == "" {
		return fmt.Errorf("%s: missing Acceptance section", slug)
	}
	if strings.Contains(acceptance, "(TBD") {
		return fmt.Errorf("%s: Acceptance still contains TBD", slug)
	}
	validate := sectionMarkdown(text, "Validate")
	if strings.TrimSpace(validate) == "" {
		return fmt.Errorf("%s: missing Validate section", slug)
	}
	validateTasks := checkboxLines(validate)
	if len(validateTasks) == 0 {
		return fmt.Errorf("%s: Validate has no checkbox tasks", slug)
	}
	impl := sectionMarkdown(text, "Tasks")
	implTasks := checkboxLines(impl)
	nonTemplateImpl := 0
	for _, line := range implTasks {
		if !isTemplateImplTask(line) {
			nonTemplateImpl++
		}
	}
	if nonTemplateImpl > 0 {
		realValidate := 0
		for _, line := range validateTasks {
			if !isTemplateValidateTask(line) {
				realValidate++
			}
		}
		if realValidate == 0 {
			return fmt.Errorf("%s: Tasks has implementation work but Validate only has placeholders", slug)
		}
	}
	return validateSpecSections(repoRoot, slug, text)
}

func sectionMarkdown(md, heading string) string {
	want := "## " + heading
	inFence := false
	start := -1
	for i, line := range strings.Split(md, "\n") {
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if stripped == want {
			start = i + 1
			continue
		}
		if start >= 0 && knownSectionLine(stripped) {
			return strings.Join(strings.Split(md, "\n")[start:i], "\n")
		}
	}
	if start < 0 {
		return ""
	}
	lines := strings.Split(md, "\n")
	return strings.Join(lines[start:], "\n")
}

func checkboxLines(section string) []string {
	out := []string{}
	for _, line := range strings.Split(section, "\n") {
		m := sddCheckboxRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out = append(out, strings.TrimSpace(m[2]))
	}
	return out
}

func isTemplateImplTask(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "first implementation task") ||
		strings.Contains(lower, "first task") ||
		strings.Contains(text, "(TBD")
}

func isTemplateValidateTask(text string) bool {
	return strings.Contains(text, "(TBD") ||
		strings.Contains(strings.ToLower(text), "e.g. go test")
}
