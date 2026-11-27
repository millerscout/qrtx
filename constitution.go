package qrtx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultConstitutionFile is used when .sdd.yaml lists no constitution.
const DefaultConstitutionFile = "sdd/constitution.md"

// ConstitutionDoc is one constitution file and the rules it defines.
type ConstitutionDoc struct {
	Path  string   `json:"path"`
	Rules []string `json:"rules"`
}

var (
	constitutionBulletIDRE = regexp.MustCompile(`^C(\d+)\b`)
	ruleHeadingRE          = regexp.MustCompile(`^(#{2,3})\s+(.+?)\s*#*\s*$`)
)

// ConstitutionFiles lists the repository's constitution files: the
// `constitution` entries of .sdd.yaml, or else sdd/constitution.md when it
// exists. A listed file that does not exist is an error.
func ConstitutionFiles(repoRoot string) ([]string, error) {
	cfg, err := LoadRepoConfig(repoRoot)
	if err != nil {
		return nil, err
	}
	if len(cfg.Constitution) > 0 {
		for _, rel := range cfg.Constitution {
			if _, err := os.Stat(filepath.Join(repoRoot, rel)); err != nil {
				return nil, fmt.Errorf("%s: constitution file %s: %w", RepoConfigFile, rel, err)
			}
		}
		return cfg.Constitution, nil
	}
	if _, err := os.Stat(filepath.Join(repoRoot, DefaultConstitutionFile)); err == nil {
		return []string{DefaultConstitutionFile}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return nil, nil
}

// LoadConstitution reads every constitution file and its rules. A repository
// without a constitution returns nil.
func LoadConstitution(repoRoot string) ([]ConstitutionDoc, error) {
	files, err := ConstitutionFiles(repoRoot)
	if err != nil {
		return nil, err
	}
	var out []ConstitutionDoc
	for _, rel := range files {
		raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			return nil, err
		}
		out = append(out, ConstitutionDoc{Path: filepath.ToSlash(rel), Rules: constitutionRules(string(raw))})
	}
	return out, nil
}

// constitutionRules returns a file's rule titles. Rules are the `##` or `###`
// headings, whichever level is more common outside code fences (ties go to
// `##`). This fits both one-heading-per-rule files and files that group
// rules under a section such as `## Rules`.
func constitutionRules(text string) []string {
	byLevel := map[int][]string{}
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := ruleHeadingRE.FindStringSubmatch(trim); m != nil {
			byLevel[len(m[1])] = append(byLevel[len(m[1])], m[2])
		}
	}
	if len(byLevel[3]) > len(byLevel[2]) {
		return byLevel[3]
	}
	return byLevel[2]
}

// ruleKey is the short name a plan may cite a rule by: the text before the
// first ":", " — " or " - " ("Rule 1", "Article IX"), or the whole title.
func ruleKey(title string) string {
	key := title
	for _, sep := range []string{":", " — ", " - "} {
		if i := strings.Index(key, sep); i > 0 {
			key = key[:i]
		}
	}
	return strings.TrimSpace(key)
}

// citedRule returns the rule a Constitution bullet cites, or "" when it
// names none. A bullet cites a rule by its full title or its ruleKey.
func citedRule(bullet string, docs []ConstitutionDoc) string {
	lower := strings.ToLower(bullet)
	best := ""
	for _, d := range docs {
		for _, r := range d.Rules {
			for _, name := range []string{r, ruleKey(r)} {
				n := strings.ToLower(name)
				if n != "" && containsWord(lower, n) && len(name) > len(best) {
					best = r
				}
			}
		}
	}
	return best
}

// containsWord reports whether s contains sub with no letter or digit
// directly around it, so "Rule 1" does not match inside "Rule 12".
func containsWord(s, sub string) bool {
	for from := 0; ; {
		i := strings.Index(s[from:], sub)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(sub)
		if (start == 0 || !isWordByte(s[start-1])) && (end == len(s) || !isWordByte(s[end])) {
			return true
		}
		from = start + 1
	}
}

func isWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// validateConstitutionSection checks the plan's `## Constitution` section
// against the repository's constitution. It is a finish-propose gate: a
// repository with a constitution needs the section, and each bullet is
// `C<n> <rule> → <how the plan complies>` naming a real rule, or a single
// `None: <why no rule applies>`.
func validateConstitutionSection(repoRoot, slug, text string) error {
	docs, err := LoadConstitution(repoRoot)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}
	if !hasSection(text, "Constitution") {
		return fmt.Errorf("%s: this repository has a constitution (%s); add `## Constitution` with `- C1 <rule> → <how the plan complies>` per rule the change touches, or `- None: <why no rule applies>` (`%s constitution show` lists the rules)", slug, constitutionPaths(docs), CLIName)
	}
	section := sectionMarkdown(text, "Constitution")
	if strings.Contains(section, "(TBD") {
		return fmt.Errorf("%s: Constitution still contains TBD", slug)
	}
	bullets := sectionBullets(section)
	if len(bullets) == 0 {
		return fmt.Errorf("%s: Constitution has no bullet (`- C1 <rule> → <how the plan complies>` or `- None: <why no rule applies>`)", slug)
	}
	for _, b := range bullets {
		if strings.HasPrefix(strings.ToLower(b), "none") {
			if len(bullets) > 1 {
				return fmt.Errorf("%s: Constitution mixes `None:` with C lines", slug)
			}
			if len(strings.TrimSpace(strings.TrimLeft(b[len("none"):], ":"))) == 0 {
				return fmt.Errorf("%s: Constitution `None:` needs a reason", slug)
			}
			continue
		}
		if !constitutionBulletIDRE.MatchString(b) {
			return fmt.Errorf("%s: constitution line must start with C<n>: %s", slug, truncateRunes(b, 100))
		}
		if !strings.Contains(b, "→") && !strings.Contains(b, "->") {
			return fmt.Errorf("%s: constitution line needs `→ <how the plan complies>`: %s", slug, truncateRunes(b, 100))
		}
		if citedRule(b, docs) == "" {
			return fmt.Errorf("%s: constitution line names no rule from %s: %s", slug, constitutionPaths(docs), truncateRunes(b, 100))
		}
	}
	return nil
}

func constitutionPaths(docs []ConstitutionDoc) string {
	var paths []string
	for _, d := range docs {
		paths = append(paths, d.Path)
	}
	return strings.Join(paths, ", ")
}

// writeConstitution prints the constitution block of the human report.
func writeConstitution(w io.Writer, docs []ConstitutionDoc) {
	if len(docs) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, "\nconstitution:")
	for _, d := range docs {
		_, _ = fmt.Fprintf(w, "  %s\n", d.Path)
		for _, r := range d.Rules {
			_, _ = fmt.Fprintf(w, "    - %s\n", r)
		}
	}
}

// constitutionTemplate is what `constitution init` writes. Each `##` heading
// is one rule.
const constitutionTemplate = `# Constitution

The rules every change in this project follows. ` + "`qrtx change finish-propose`" + ` requires each
plan to say, under ` + "`## Constitution`" + `, how it complies with each rule it touches,
citing the rule by its heading (or the part before a colon, such as "Rule 1").

Each ` + "`##`" + ` heading below is one rule. Keep headings short and stable: plans cite them.
These are examples. Replace them with your project's own rules.

## Rule 1: Tests before code

A behaviour change adds or updates a test that fails before the change and passes after it.

## Rule 2: No silent data loss

A change that writes, migrates or deletes stored data says how existing data survives,
and how to roll back if the change fails midway.

## Rule 3: Public contracts stay compatible

Exported APIs, CLI flags, file formats and JSON fields keep working for existing users.
A breaking change is a decision the user approves, with a migration path.
`

// InitConstitution writes sdd/constitution.md from the template. It refuses
// when .sdd.yaml already lists constitution files, and it overwrites an
// existing file only when force is set. It returns the path written.
func InitConstitution(repoRoot string, force bool) (string, error) {
	cfg, err := LoadRepoConfig(repoRoot)
	if err != nil {
		return "", err
	}
	if len(cfg.Constitution) > 0 {
		return "", fmt.Errorf("%s already lists a constitution: %s", RepoConfigFile, strings.Join(cfg.Constitution, ", "))
	}
	path := filepath.Join(repoRoot, DefaultConstitutionFile)
	if _, err := os.Stat(path); err == nil && !force {
		return "", fmt.Errorf("%s exists; pass --force to replace it", DefaultConstitutionFile)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(constitutionTemplate), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
