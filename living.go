package qrtx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Living specs: opsx merges each change's scenarios into openspec/specs/ at
// archive, so the next planner reads what the system already promises. SDD
// scenarios used to stay in sdd/changes/archive/. ArchiveChange now appends a
// change's Requirements and Scenarios to the `## Behaviour` section of each
// touched package's contract.sdd.md, and propose-context shows those blocks.

const (
	behaviourHeading  = "## Behaviour"
	behaviourMaxRunes = 4000
)

// ExistingBehaviour is the `## Behaviour` block of one package contract.
type ExistingBehaviour struct {
	Package string `json:"package"`
	Text    string `json:"text"`
}

// behaviourPackages returns the package dirs a change touches that have a
// contract: its `packages:` list plus dirs of cited files.
func behaviourPackages(repoRoot string, packages []string, text string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(dir string) {
		dir = strings.Trim(filepath.ToSlash(dir), "/")
		if dir == "" || seen[dir] {
			return
		}
		if st, err := os.Stat(filepath.Join(repoRoot, dir, contractFileName)); err != nil || st.IsDir() {
			return
		}
		seen[dir] = true
		out = append(out, dir)
	}
	for _, p := range packages {
		add(p)
	}
	for _, c := range citedPaths(repoRoot, text) {
		add(pathDir(c.path))
	}
	sort.Strings(out)
	return out
}

// behaviourBlock renders a change's requirements and scenarios, ids prefixed with the slug.
func behaviourBlock(slug, date, text string) string {
	reqs, _ := parseRequirements(text)
	scenarios, _ := parseScenarios(text)
	if len(reqs) == 0 && len(scenarios) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### %s %s\n\n", date, slug)
	for _, r := range reqs {
		fmt.Fprintf(&b, "- %s/%s\n", slug, r.Text)
	}
	for _, sc := range scenarios {
		fmt.Fprintf(&b, "- %s/%s\n", slug, sc.Text)
	}
	return b.String()
}

// hasBehaviourBlock reports whether a contract already holds the block for slug (any date).
func hasBehaviourBlock(contract, slug string) bool {
	for _, line := range strings.Split(contract, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "### ") && strings.HasSuffix(trim, " "+slug) {
			return true
		}
	}
	return false
}

// promoteBehaviour appends the change's requirements and scenarios to each
// touched package contract. It is idempotent and returns the contracts written.
func promoteBehaviour(repoRoot, slug, date string) ([]string, error) {
	raw, err := os.ReadFile(changeFilePath(repoRoot, slug))
	if err != nil {
		return nil, err
	}
	text := string(raw)
	block := behaviourBlock(slug, date, text)
	if block == "" {
		return nil, nil
	}
	man, err := ParseChangeMarkdown(text, changeFilePath(repoRoot, slug))
	if err != nil {
		return nil, err
	}
	var written []string
	for _, dir := range behaviourPackages(repoRoot, man.Packages, text) {
		path := filepath.Join(repoRoot, dir, contractFileName)
		b, err := os.ReadFile(path)
		if err != nil {
			return written, err
		}
		contract := string(b)
		if hasBehaviourBlock(contract, slug) {
			continue
		}
		contract = strings.TrimRight(contract, "\n") + "\n\n"
		if !strings.Contains(contract, "\n"+behaviourHeading+"\n") {
			contract += behaviourHeading + "\n\nWhat archived changes promised for this package (`qrtx change archive` appends; newest last).\n\n"
		}
		contract += block
		if err := os.WriteFile(path, []byte(contract), 0o644); err != nil {
			return written, err
		}
		written = append(written, filepath.ToSlash(filepath.Join(dir, contractFileName)))
	}
	return written, nil
}

// existingBehaviour reads the `## Behaviour` section of each package contract.
func existingBehaviour(repoRoot string, dirs []string) []ExistingBehaviour {
	var out []ExistingBehaviour
	for _, dir := range dirs {
		b, err := os.ReadFile(filepath.Join(repoRoot, dir, contractFileName))
		if err != nil {
			continue
		}
		section := strings.TrimSpace(sectionAfterHeading(string(b), behaviourHeading))
		if section == "" {
			continue
		}
		out = append(out, ExistingBehaviour{Package: dir, Text: truncateRunes(section, behaviourMaxRunes)})
	}
	return out
}

// sectionAfterHeading returns the text after an exact `## ` heading up to the next `## ` heading.
func sectionAfterHeading(text, heading string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != heading {
			continue
		}
		var body []string
		for _, l := range lines[i+1:] {
			if strings.HasPrefix(l, "## ") {
				break
			}
			body = append(body, l)
		}
		return strings.Join(body, "\n")
	}
	return ""
}
