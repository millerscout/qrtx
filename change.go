package sdd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/millerscout/sdd/tasksmd"
	"gopkg.in/yaml.v3"
)

const (
	changesDirName = "sdd/changes"
	changeFileName = "change.sdd.md"
	archiveDirName = "archive"
)

var changeSlugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ChangeManifest is the machine block in change.sdd.md.
type ChangeManifest struct {
	Name     string   `yaml:"name"`
	Status   string   `yaml:"status"`
	Packages []string `yaml:"packages"`
}

// ChangeSummary is one active SDD change for UI/CLI.
type ChangeSummary struct {
	Slug       string
	Status     string
	TasksTotal int
	TasksDone  int
	OpenPhase  string
}

// ValidChangeSlug reports kebab-case slug rules for SDD changes.
func ValidChangeSlug(slug string) bool {
	s := strings.TrimSpace(slug)
	return s != "" && changeSlugRE.MatchString(s)
}

func changesRoot(repoRoot string) string {
	return filepath.Join(repoRoot, changesDirName)
}

func changeDir(repoRoot, slug string) string {
	return filepath.Join(changesRoot(repoRoot), slug)
}

func changeFilePath(repoRoot, slug string) string {
	return filepath.Join(changeDir(repoRoot, slug), changeFileName)
}

// NewChange scaffolds sdd/changes/<slug>/change.sdd.md.
func NewChange(repoRoot, slug string) error {
	slug = strings.TrimSpace(slug)
	if !ValidChangeSlug(slug) {
		return fmt.Errorf("invalid change slug %q: use lowercase kebab-case", slug)
	}
	dir := changeDir(repoRoot, slug)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("change %q already exists", slug)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("```change\nname: %s\nstatus: planning\npackages: []\n```\n\n## Why\n\n(TBD — user pain and product sense, and the chosen approach. Alternatives go in Decisions.)\n\n## User request\n\n(TBD — what the user wanted to accomplish or experience, from the conversation arc.)\n\n## Acceptance\n\n- (TBD — user-visible checks that satisfy the requester.)\n\n## Requirements\n\n- (TBD — `Q1 <subject> SHALL <behaviour>.` One per concern the request touches: the normal path that must keep working, the new behaviour, failure and recovery, what happens afterwards, what an operator sees.)\n\n## Scenarios\n\n- (TBD — `S1 (Q1) WHEN <condition> THEN <observable result>`. One per failure, boundary, empty or partial-failure case this request can hit. Cite each id in Validate or a task with `covers S1`.)\n\n## Decisions\n\n- (TBD — `D1 <choice>. Rejected: <alternative> because <reason>.` Check each against Acceptance, Scenarios and the cited code. `None: <reason>` when there is no choice.)\n\n## Risks\n\n- (TBD — `<risk> → <mitigation>`.)\n\nDeploy: (TBD — when the change persists data or spans processes: the order binaries, scripts and data move in, and which old version must keep working meanwhile. `n/a (<reason>)` otherwise.)\n\nRollback: (TBD — how to undo if this fails midway, or `n/a (<reason>)` when nothing persists.)\n\n## Observed\n\n(TBD — cite a path that exists in the repo. Separate from assumptions.)\n\n## Assumptions\n\n(TBD)\n\n## Non-goals\n\n(TBD — a boundary the request already has.)\n\n## Deferred\n\n(Optional — other slugs that own split acceptance.)\n\n## Tasks\n\n- [ ] 1.1 First implementation task\n\n## Validate\n\n(Minimal regression — one focused task per acceptance bullet; end each line with the scenarios it covers, `covers S1,S2`. Name harness: go test, unittest, golden JSONL row, or manual:.)\n\n- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)\n- [ ] V1.2 (optional) golden row in scripts/fixtures/ if behavior is user-visible\n\n## Review\n\n- (TBD — last step: run `%[2]s change review-packet --change %[1]s`, give it to a second reader with fresh context, and record each finding as `R<n> <claim ids> <conflict> → <fix>`, or `None: <what was checked>`.)\n", slug, CLIName)
	docs, err := LoadConstitution(repoRoot)
	if err != nil {
		return err
	}
	if len(docs) > 0 {
		section := fmt.Sprintf("## Constitution\n\n- (TBD — one `C1 <rule> → <how this plan complies>` line per rule from %s that this change touches (`%s constitution show` lists them), or `None: <why no rule applies>`.)\n\n## Review\n", constitutionPaths(docs), CLIName)
		body = strings.Replace(body, "## Review\n", section, 1)
	}
	path := filepath.Join(dir, changeFileName)
	return os.WriteFile(path, []byte(body), 0o644)
}

// ListChanges returns active slugs under sdd/changes (not archive).
func ListChanges(repoRoot string) ([]string, error) {
	root := changesRoot(repoRoot)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := []string{}
	for _, ent := range entries {
		if !ent.IsDir() || ent.Name() == archiveDirName || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		if !ValidChangeSlug(ent.Name()) {
			continue
		}
		if _, err := os.Stat(changeFilePath(repoRoot, ent.Name())); err != nil {
			continue
		}
		out = append(out, ent.Name())
	}
	sort.Strings(out)
	return out, nil
}

// ParseChangeFile reads change.sdd.md at repoRoot/slug.
func ParseChangeFile(repoRoot, slug string) (*ChangeManifest, error) {
	path := changeFilePath(repoRoot, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseChangeMarkdown(string(raw), path)
}

// ParseChangeMarkdown extracts the ```change block.
func ParseChangeMarkdown(markdown, pathForErr string) (*ChangeManifest, error) {
	block, err := extractFencedBlock(markdown, "change")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pathForErr, err)
	}
	var m ChangeManifest
	if err := yaml.Unmarshal([]byte(block), &m); err != nil {
		return nil, fmt.Errorf("%s: invalid change yaml: %w", pathForErr, err)
	}
	if strings.TrimSpace(m.Name) == "" {
		return nil, fmt.Errorf("%s: change missing name", pathForErr)
	}
	st := strings.TrimSpace(m.Status)
	if st == "" {
		m.Status = "planning"
	} else if st != "planning" && st != "applying" && st != "complete" {
		return nil, fmt.Errorf("%s: invalid status %q", pathForErr, st)
	}
	return &m, nil
}

func extractFencedBlock(markdown, lang string) (string, error) {
	open := "```" + lang
	start := strings.Index(markdown, open)
	if start < 0 {
		return "", fmt.Errorf("missing %s block", open)
	}
	rest := markdown[start+len(open):]
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		return "", fmt.Errorf("unclosed %s block", open)
	}
	return strings.TrimSpace(rest[:end]), nil
}

// ChangeProgress parses task checkboxes in change.sdd.md.
func ChangeProgress(repoRoot, slug string) tasksmd.Progress {
	dir := filepath.Join(repoRoot, "sdd", "changes", slug, "phases")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return tasksmd.ParseTasksMD(changeFilePath(repoRoot, slug))
	}
	var prog tasksmd.Progress
	prog.Found = true
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		one := tasksmd.ParseTasksMD(filepath.Join(dir, entry.Name()))
		prog.Total += one.Total
		prog.Done += one.Done
		for _, item := range one.NextItems {
			if len(prog.NextItems) < 3 {
				prog.NextItems = append(prog.NextItems, item)
			}
		}
		prog.Items = append(prog.Items, one.Items...)
	}
	if prog.NextItems == nil {
		prog.NextItems = []string{}
	}
	if prog.Items == nil {
		prog.Items = []tasksmd.Item{}
	}
	return prog
}

// SummarizeChange loads manifest + task counts.
func SummarizeChange(repoRoot, slug string) (ChangeSummary, error) {
	man, err := ParseChangeFile(repoRoot, slug)
	if err != nil {
		return ChangeSummary{}, err
	}
	prog := ChangeProgress(repoRoot, slug)
	return ChangeSummary{
		Slug:       slug,
		Status:     man.Status,
		TasksTotal: prog.Total,
		TasksDone:  prog.Done,
		OpenPhase:  openPhaseOf(repoRoot, slug),
	}, nil
}

func openPhaseOf(repoRoot, slug string) string {
	raw, err := os.ReadFile(changeFilePath(repoRoot, slug))
	if err != nil {
		return ""
	}
	tasks := sectionMarkdown(string(raw), "Tasks")
	if roadmapMarked(tasks) {
		return openRoadmapPhase(repoRoot, slug, tasks)
	}
	return openPhase(tasks)
}

// ResolveActiveChangeSlug maps session memory to an effective active slug.
// When memSlug is not an active change directory, effective is empty and stale is true.
func ResolveActiveChangeSlug(repoRoot, memSlug string) (effective string, stale bool) {
	memSlug = strings.TrimSpace(memSlug)
	if memSlug == "" {
		return "", false
	}
	slugs, err := ListChanges(repoRoot)
	if err != nil {
		return "", true
	}
	for _, slug := range slugs {
		if slug == memSlug {
			return memSlug, false
		}
	}
	return "", true
}

// ActiveChangeListed reports whether slug is an active change under sdd/changes/.
func ActiveChangeListed(repoRoot, slug string) bool {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return false
	}
	slugs, err := ListChanges(repoRoot)
	if err != nil {
		return false
	}
	for _, s := range slugs {
		if s == slug {
			return true
		}
	}
	return false
}

// ChangesMap builds context sdd.changes payload.
func ChangesMap(repoRoot, memSlug string) map[string]any {
	memSlug = strings.TrimSpace(memSlug)
	effective, stale := ResolveActiveChangeSlug(repoRoot, memSlug)
	slugs, err := ListChanges(repoRoot)
	if err != nil {
		out := map[string]any{
			"active_slug":   effective,
			"active_status": "",
			"active":        []any{},
		}
		if stale && memSlug != "" {
			out["stale_pin"] = memSlug
		}
		return out
	}
	active := []map[string]any{}
	activeStatus := ""
	for _, slug := range slugs {
		sum, err := SummarizeChange(repoRoot, slug)
		if err != nil {
			continue
		}
		row := map[string]any{
			"slug":        sum.Slug,
			"status":      sum.Status,
			"tasks_total": sum.TasksTotal,
			"tasks_done":  sum.TasksDone,
		}
		active = append(active, row)
		if slug == effective {
			activeStatus = sum.Status
		}
	}
	out := []any{}
	for _, row := range active {
		out = append(out, row)
	}
	result := map[string]any{
		"active_slug":   effective,
		"active_status": activeStatus,
		"active":        out,
	}
	if stale && memSlug != "" {
		result["stale_pin"] = memSlug
	}
	return result
}

// ArchiveChange moves a complete change into sdd/changes/archive/.
func ArchiveChange(repoRoot, slug string) error {
	slug = strings.TrimSpace(slug)
	if !ValidChangeSlug(slug) {
		return fmt.Errorf("invalid change slug %q", slug)
	}
	if err := ValidateChangeManifest(repoRoot, slug); err != nil {
		return fmt.Errorf("change verify: %w", err)
	}
	man, err := ParseChangeFile(repoRoot, slug)
	if err != nil {
		return err
	}
	if strings.TrimSpace(man.Status) != "complete" {
		return fmt.Errorf("change %q status is %q; set status to complete before archive", slug, man.Status)
	}
	prog := ChangeProgress(repoRoot, slug)
	if prog.Total > 0 && prog.Done < prog.Total {
		return fmt.Errorf("change %q has unchecked tasks (%d/%d)", slug, prog.Done, prog.Total)
	}
	src := changeDir(repoRoot, slug)
	if _, err := os.Stat(src); err != nil {
		return err
	}
	date := time.Now().UTC().Format("2006-01-02")
	dest := filepath.Join(changesRoot(repoRoot), archiveDirName, date+"-"+slug)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("archive destination already exists: %s", dest)
	}
	// Keep what the change promised in the touched package contracts.
	if _, err := promoteBehaviour(repoRoot, slug, date); err != nil {
		return fmt.Errorf("promote behaviour: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dest)
}

// ensureChangesStore creates sdd/changes if missing.
func ensureChangesStore(repoRoot string) error {
	return os.MkdirAll(changesRoot(repoRoot), 0o755)
}

// MergeChangesIntoStatus adds sdd.changes to an existing sdd status map.
func MergeChangesIntoStatus(m map[string]any, repoRoot, activeSlug string) {
	if m == nil {
		return
	}
	_ = ensureChangesStore(repoRoot)
	m["changes"] = ChangesMap(repoRoot, activeSlug)
}
