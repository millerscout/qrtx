package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/millerscout/qrtx"
)

const trackerUsage = "Usage: %s tracker list [--status <category>] [--change <slug>] [--all] [--json] | get <ref> [--json] | add <title> [--description <text>] [--priority <0-4>] [--client-ref <ref>] | close <ref> [--canceled] | link <ref> <blocks|relates|duplicates> <ref> | unlink <ref> <type> <ref> | priority <ref> <0-4> | start <ref> <slug> [--force] | from-deferred --change <slug> | outbox [--json] [--drop <seq>] | check [--backend <name>] [--allow-writes] [--json] | init | setup <github|jira> [flags] | merge-driver <base> <ours> <theirs>\n"

func runTracker(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintf(stderr, trackerUsage, qrtx.CLIName)
		return fmt.Errorf("missing tracker subcommand")
	}
	// merge-driver runs inside git merge; it needs no repository config.
	if args[0] == "merge-driver" {
		return trackerMergeDriver(args[1:])
	}
	root, err := qrtx.RepoRoot()
	if err != nil {
		return err
	}
	if args[0] == "outbox" {
		return trackerOutbox(root, args[1:], stdout, stderr)
	}
	if args[0] == "init" {
		return trackerInit(root, stdout)
	}
	if args[0] == "setup" {
		_, err := trackerSetup(root, args[1:], stdout)
		return err
	}
	t, err := qrtx.OpenTracker(root)
	if err != nil {
		return err
	}
	for _, w := range t.Warnings() {
		fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	drain := func() { reportDrain(qrtx.DrainOutbox(root, t), stderr) }
	drain()
	defer drain()
	asJSON := hasFlag(args, "--json")
	switch args[0] {
	case "list":
		f := qrtx.IssueFilter{Change: flagValue(args, "--change")}
		if st := flagValue(args, "--status"); st != "" {
			f.Categories = []string{st}
		} else if !hasFlag(args, "--all") {
			f.Categories = []string{qrtx.CategoryBacklog, qrtx.CategoryUnstarted, qrtx.CategoryStarted}
		}
		list, err := t.ListIssues(f)
		if err != nil {
			return err
		}
		list = qrtx.WithRecentCreates(root, t.Backend, f, list, t.GetIssue)
		qrtx.SortByPriority(list)
		if asJSON {
			return writeJSON(stdout, list)
		}
		blockers := listBlockers(t, f, list, stderr)
		for _, is := range list {
			fmt.Fprintf(stdout, "%s\t%s\t%s", t.Ref("", is.ID), is.Status.Category, is.Title)
			if is.Priority != nil && *is.Priority > 0 {
				fmt.Fprintf(stdout, "\tP%d", *is.Priority)
			}
			if by := blockers[t.Ref("", is.ID)]; len(by) > 0 && qrtx.OpenCategory(is.Status.Category) {
				fmt.Fprintf(stdout, "\tblocked by %s", strings.Join(by, ", "))
			}
			fmt.Fprintln(stdout)
		}
		return nil
	case "get":
		ref := firstPositional(args[1:])
		if ref == "" {
			return fmt.Errorf("tracker get: pass an issue ref")
		}
		backend, id := qrtx.ParseIssueRef(ref, t.Backend)
		impl, err := t.For(backend)
		if err != nil {
			return err
		}
		is, err := impl.GetIssue(id)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(stdout, is)
		}
		fmt.Fprintf(stdout, "%s  [%s] %s\n", t.Ref(backend, is.ID), is.Status.Category, is.Title)
		if is.Description != "" {
			fmt.Fprintf(stdout, "\n%s\n", is.Description)
		}
		if is.Priority != nil && *is.Priority > 0 {
			fmt.Fprintf(stdout, "  priority: %d\n", *is.Priority)
		}
		for _, l := range is.Links {
			fmt.Fprintf(stdout, "  %s %s\n", l.Type, t.Ref(backend, l.ID))
		}
		for _, r := range displayRefs(is.Refs) {
			fields := []string{}
			for _, f := range []string{r.Repo, r.Project, r.Change, r.Role} {
				if f != "" {
					fields = append(fields, f)
				}
			}
			fmt.Fprintf(stdout, "  sdd: %s\n", strings.Join(fields, " "))
		}
		return nil
	case "add":
		title := firstPositional(args[1:], "--description", "--client-ref")
		if title == "" {
			return fmt.Errorf("tracker add: pass a title")
		}
		var priority *int
		if v := flagValue(args, "--priority"); v != "" {
			n, err := parsePriority(v)
			if err != nil {
				return fmt.Errorf("tracker add: %w", err)
			}
			priority = &n
		}
		// A new client_ref has never been sent, so the provider can skip its
		// duplicate lookup; --client-ref retries an earlier add safely.
		cref := flagValue(args, "--client-ref")
		fresh := cref == ""
		if fresh {
			cref = qrtx.NewClientRef(qrtx.RepoName(root), "", title, qrtx.NewNonce())
		}
		is, err := t.CreateIssue(qrtx.CreateIssueInput{
			Title:       title,
			Description: flagValue(args, "--description"),
			Refs:        []qrtx.SDDRef{{Repo: qrtx.RepoName(root), Project: t.Project()}},
			Priority:    priority,
			ClientRef:   cref,
			Fresh:       fresh,
		})
		if err != nil {
			if qrtx.IsUnavailable(err) {
				fmt.Fprintf(stderr, "the issue may or may not have been created; to retry without a duplicate run:\n  %s tracker add %s --description %s --client-ref %s\n", qrtx.CLIName, shQuote(title), shQuote(flagValue(args, "--description")), shQuote(cref))
			}
			return err
		}
		qrtx.RecordCreated(root, t.Backend, is)
		fmt.Fprintf(stdout, "added %s\n", t.Ref("", is.ID))
		return nil
	case "close":
		ref := firstPositional(args[1:])
		if ref == "" {
			return fmt.Errorf("tracker close: pass an issue ref")
		}
		backend, id := qrtx.ParseIssueRef(ref, t.Backend)
		impl, err := t.For(backend)
		if err != nil {
			return err
		}
		cat := qrtx.CategoryCompleted
		if hasFlag(args, "--canceled") {
			cat = qrtx.CategoryCanceled
		}
		is, err := impl.TransitionIssue(id, cat, t.Native(cat))
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s %s\n", t.Ref(backend, is.ID), is.Status.Category)
		return nil
	case "link", "unlink":
		pos := positionals(args[1:])
		if len(pos) != 3 {
			return fmt.Errorf("tracker %s: pass <ref> <%s> <ref>", args[0], strings.Join(qrtx.LinkTypes, "|"))
		}
		remove := args[0] == "unlink"
		is, err := qrtx.SetLink(t, pos[0], pos[1], pos[2], remove)
		if err != nil {
			return err
		}
		backend, _ := qrtx.ParseIssueRef(pos[0], t.Backend)
		verb := "linked"
		if remove {
			verb = "unlinked"
		}
		_, bID := qrtx.ParseIssueRef(pos[2], t.Backend)
		fmt.Fprintf(stdout, "%s %s %s %s\n", verb, t.Ref(backend, is.ID), pos[1], t.Ref(backend, bID))
		return nil
	case "priority":
		pos := positionals(args[1:])
		if len(pos) != 2 {
			return fmt.Errorf("tracker priority: pass <ref> <0-4>")
		}
		n, err := parsePriority(pos[1])
		if err != nil {
			return fmt.Errorf("tracker priority: %w", err)
		}
		is, err := qrtx.SetPriority(t, pos[0], n)
		if err != nil {
			return err
		}
		backend, _ := qrtx.ParseIssueRef(pos[0], t.Backend)
		fmt.Fprintf(stdout, "%s priority %d\n", t.Ref(backend, is.ID), n)
		return nil
	case "start":
		pos := positionals(args[1:])
		if len(pos) != 2 {
			return fmt.Errorf("tracker start: pass <ref> <slug>")
		}
		res, err := qrtx.StartFromIssueWith(root, t, pos[0], pos[1], qrtx.StartOptions{Force: hasFlag(args, "--force")})
		if err != nil {
			return err
		}
		for _, w := range res.Warnings {
			fmt.Fprintf(stderr, "warning: %s\n", w)
		}
		if res.Created {
			fmt.Fprintf(stdout, "created sdd/changes/%s/\n", res.Slug)
		}
		if res.Forced {
			if len(res.Blockers) > 0 {
				fmt.Fprintf(stdout, "forced: %s is blocked by %s\n", res.Ref, strings.Join(res.Blockers, ", "))
			} else {
				fmt.Fprintf(stdout, "forced: could not check whether %s is blocked\n", res.Ref)
			}
		}
		fmt.Fprintf(stdout, "linked %s to %s\n", res.Ref, res.Slug)
		for _, a := range res.AlsoActive {
			where := ""
			if a.Worktree != "" {
				where = " (worktree " + a.Worktree + ")"
			}
			fmt.Fprintf(stderr, "warning: %s is also linked to active change %s%s\n", res.Ref, a.Slug, where)
		}
		return nil
	case "from-deferred":
		slug := flagValue(args, "--change")
		if slug == "" {
			return fmt.Errorf("tracker from-deferred: pass --change <slug>")
		}
		n, err := qrtx.QueueDeferredIssues(root, t, slug)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "queued %d deferred issue(s) for %s\n", n, slug)
		return nil
	case "check":
		results, err := qrtx.CheckTracker(root, flagValue(args, "--backend"), hasFlag(args, "--allow-writes"))
		if err != nil {
			return err
		}
		if asJSON {
			if err := writeJSON(stdout, results); err != nil {
				return err
			}
		}
		failed := 0
		for _, r := range results {
			mark := "pass"
			if !r.OK {
				mark = "FAIL"
				failed++
			}
			if !asJSON {
				fmt.Fprintf(stdout, "%s  %s  %s\n", mark, r.Op, r.Detail)
			}
		}
		if failed > 0 {
			return fmt.Errorf("tracker check: %d operation(s) failed", failed)
		}
		return nil
	default:
		fmt.Fprintf(stderr, trackerUsage, qrtx.CLIName)
		return fmt.Errorf("unknown tracker subcommand: %s", args[0])
	}
}

// listBlockers maps each open issue of the configured backend to the open
// issues that block it. The listing itself is used when it holds every open
// issue; a filter (--status, --change) gets a second listing of open issues,
// so a blocker the filter hides still shows. A failed or cut-off second
// listing warns and shows what it found.
func listBlockers(t *qrtx.Tracker, f qrtx.IssueFilter, list []qrtx.Issue, stderr io.Writer) map[string][]string {
	open := list
	if !coversOpen(f) {
		all, complete, err := qrtx.ListOpenIssues(t.IssueTracker)
		if err != nil {
			fmt.Fprintf(stderr, "warning: blockers not shown: %v\n", err)
			return nil
		}
		if !complete {
			fmt.Fprintf(stderr, "warning: the open-issue listing was incomplete; some blockers may not show\n")
		}
		open = all
	}
	return qrtx.BlockerIndex(t.Backend, open)
}

// coversOpen reports whether a listing with filter f holds every open issue.
func coversOpen(f qrtx.IssueFilter) bool {
	if f.Change != "" {
		return false
	}
	if len(f.Categories) == 0 {
		return true
	}
	for _, c := range []string{qrtx.CategoryBacklog, qrtx.CategoryUnstarted, qrtx.CategoryStarted} {
		if !containsArg(f.Categories, c) {
			return false
		}
	}
	return true
}

func reportDrain(rep qrtx.DrainReport, stderr io.Writer) {
	for _, w := range rep.Warnings {
		fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	if rep.Pending > 0 {
		fmt.Fprintf(stderr, "tracker: %d pending (%s tracker outbox)\n", rep.Pending, qrtx.CLIName)
	}
}

func trackerOutbox(root string, args []string, stdout, stderr io.Writer) error {
	if seq := flagValue(args, "--drop"); seq != "" {
		var n int
		if _, err := fmt.Sscanf(seq, "%d", &n); err != nil {
			return fmt.Errorf("tracker outbox --drop: %q is not a sequence number", seq)
		}
		if err := qrtx.DropOutboxEntry(root, n); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "dropped %d\n", n)
		return nil
	}
	entries, err := qrtx.ListOutbox(root)
	if err != nil {
		return err
	}
	if hasFlag(args, "--json") {
		return writeJSON(stdout, entries)
	}
	if len(entries) == 0 {
		fmt.Fprintln(stdout, "outbox empty")
		return nil
	}
	for _, e := range entries {
		fmt.Fprintf(stdout, "%d\t%s\t%s:%s\tattempts=%d\t%s\n", e.Seq, e.Op, e.Backend, e.IssueID, e.Attempts, e.LastError)
	}
	return nil
}

// displayRefs drops a ref that names no change when another ref for the
// same repo and project names one: `add` records the first, `start` the
// second, and both stay in the backend. JSON output keeps every ref.
func displayRefs(refs []qrtx.SDDRef) []qrtx.SDDRef {
	hasChange := map[[2]string]bool{}
	for _, r := range refs {
		if r.Change != "" {
			hasChange[[2]string{r.Repo, r.Project}] = true
		}
	}
	out := make([]qrtx.SDDRef, 0, len(refs))
	for _, r := range refs {
		if r.Change == "" && hasChange[[2]string{r.Repo, r.Project}] {
			continue
		}
		out = append(out, r)
	}
	return out
}

func trackerInit(root string, stdout io.Writer) error {
	action, driver, warn, err := registerMergeDriver(root)
	if err != nil {
		return err
	}
	if action == "unchanged" {
		fmt.Fprintf(stdout, "unchanged merge driver sdd-issues: %s\n", driver)
	} else {
		fmt.Fprintf(stdout, "registered merge driver sdd-issues: %s\n", driver)
	}
	if warn != "" {
		fmt.Fprintf(stdout, "warning: %s\n", warn)
	}
	return nil
}

// mergeDriverArgs is what follows the command in the issues merge driver.
const mergeDriverArgs = " tracker merge-driver %O %A %B"

// pathMergeDriver is the driver that runs the CLI by name, through PATH.
func pathMergeDriver() string { return qrtx.CLIName + mergeDriverArgs }

// mergeDriverFor picks the merge driver this binary should register: the
// CLI name when PATH resolves it to this very binary (so upgrades behind that
// name keep working), else this binary's absolute path. Embedded hosts set
// CLIName to "host qrtx"; the first word is looked up and the rest is kept.
func mergeDriverFor() (driver string, onPath bool, exe string, err error) {
	exe, err = os.Executable()
	if err != nil {
		return "", false, "", err
	}
	words := strings.Fields(qrtx.CLIName)
	if len(words) > 0 {
		if found, lerr := exec.LookPath(words[0]); lerr == nil && sameFile(found, exe) {
			return pathMergeDriver(), true, exe, nil
		}
	}
	rest := ""
	if len(words) > 1 {
		rest = " " + strings.Join(words[1:], " ")
	}
	return fmt.Sprintf("%q", exe) + rest + mergeDriverArgs, false, exe, nil
}

// sameFile reports whether a and b are the same file after resolving links.
func sameFile(a, b string) bool {
	sa, err := os.Stat(a)
	if err != nil {
		return false
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(sa, sb)
}

// registerMergeDriver writes the .gitattributes line and the git config for
// the issues.jsonl merge driver. It reports "unchanged" when both are in
// place. It never replaces a PATH-form driver with an absolute path: a
// go run or temporary build would otherwise break merges once it is gone.
// warn is non-empty when the driver depends on a binary not found on PATH.
func registerMergeDriver(root string) (action, driver, warn string, err error) {
	want, onPath, exe, err := mergeDriverFor()
	if err != nil {
		return "", "", "", err
	}
	attr := filepath.Join(root, ".gitattributes")
	line := qrtx.IssuesFile + " merge=sdd-issues"
	raw, err := os.ReadFile(attr)
	if err != nil && !os.IsNotExist(err) {
		return "", "", "", err
	}
	action = "unchanged"
	if !strings.Contains(string(raw), line) {
		text := string(raw)
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		if err := os.WriteFile(attr, []byte(text+line+"\n"), 0o644); err != nil {
			return "", "", "", err
		}
		action = "created"
	}
	get := exec.Command("git", "config", "--get", "merge.sdd-issues.driver")
	get.Dir = root
	current := ""
	if out, gerr := get.Output(); gerr == nil {
		current = strings.TrimSpace(string(out))
	}
	if !onPath {
		if current == pathMergeDriver() {
			return action, current, fmt.Sprintf("this %s (%s) is not the one on PATH; kept the merge driver that runs %s from PATH", qrtx.CLIName, exe, qrtx.CLIName), nil
		}
		warn = fmt.Sprintf("the merge driver runs %s, which is not on PATH; if that file goes away, merges of %s fail. Install %s on PATH and re-run `%s tracker init`", exe, qrtx.IssuesFile, qrtx.CLIName, qrtx.CLIName)
	}
	if current == want {
		return action, want, warn, nil
	}
	for _, kv := range [][2]string{{"merge.sdd-issues.name", "sdd issues.jsonl merge"}, {"merge.sdd-issues.driver", want}} {
		cmd := exec.Command("git", "config", kv[0], kv[1])
		cmd.Dir = root
		if out, cerr := cmd.CombinedOutput(); cerr != nil {
			return "", "", "", fmt.Errorf("git config %s: %v: %s", kv[0], cerr, strings.TrimSpace(string(out)))
		}
	}
	return "created", want, warn, nil
}

// trackerMergeDriver is git's merge driver: it merges into ours (%A) and
// exits non-zero when conflict markers were written.
func trackerMergeDriver(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("tracker merge-driver: pass <base> <ours> <theirs>")
	}
	read := func(p string) ([]byte, error) {
		b, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			return nil, nil
		}
		return b, err
	}
	base, err := read(args[0])
	if err != nil {
		return err
	}
	ours, err := read(args[1])
	if err != nil {
		return err
	}
	theirs, err := read(args[2])
	if err != nil {
		return err
	}
	merged, conflict, err := qrtx.MergeIssues(base, ours, theirs)
	if err != nil {
		return err
	}
	if err := os.WriteFile(args[1], merged, 0o644); err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("conflicting edits in %s; resolve the marked issues", qrtx.IssuesFile)
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func flagValue(args []string, flag string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

// parsePriority reads a priority: 0 (none) to 4 (low), 1 being urgent.
func parsePriority(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || !qrtx.ValidPriority(n) {
		return 0, fmt.Errorf("priority %q is not a number from 0 (none) to 4 (low); 1 is urgent", s)
	}
	return n, nil
}

var valueFlags = map[string]bool{"--priority": true, "--status": true, "--change": true, "--description": true, "--backend": true, "--drop": true,
	"--repo": true, "--api-url": true, "--url": true, "--project": true, "--issue-type": true, "--token-env": true, "--email-env": true, "--token-command": true, "--tracker": true, "--session": true}

// positionals returns the arguments that are neither flags nor flag values.
func positionals(args []string, extraValueFlags ...string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			if valueFlags[a] || containsArg(extraValueFlags, a) {
				i++
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func firstPositional(args []string, extraValueFlags ...string) string {
	p := positionals(args, extraValueFlags...)
	if len(p) == 0 {
		return ""
	}
	return p[0]
}

func containsArg(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// shQuote quotes s for a POSIX shell.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
