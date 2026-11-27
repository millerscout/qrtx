package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/millerscout/sdd"
)

const trackerUsage = "Usage: %s tracker list [--status <category>] [--change <slug>] [--all] [--json] | get <ref> [--json] | add <title> [--description <text>] | close <ref> [--canceled] | start <ref> <slug> | from-deferred --change <slug> | outbox [--json] [--drop <seq>] | check [--backend <name>] [--allow-writes] [--json] | init | setup <github|jira> [flags] | merge-driver <base> <ours> <theirs>\n"

func runTracker(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintf(stderr, trackerUsage, sdd.CLIName)
		return fmt.Errorf("missing tracker subcommand")
	}
	// merge-driver runs inside git merge; it needs no repository config.
	if args[0] == "merge-driver" {
		return trackerMergeDriver(args[1:])
	}
	root, err := sdd.RepoRoot()
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
	t, err := sdd.OpenTracker(root)
	if err != nil {
		return err
	}
	for _, w := range t.Warnings() {
		fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	drain := func() { reportDrain(sdd.DrainOutbox(root, t), stderr) }
	drain()
	defer drain()
	asJSON := hasFlag(args, "--json")
	switch args[0] {
	case "list":
		f := sdd.IssueFilter{Change: flagValue(args, "--change")}
		if st := flagValue(args, "--status"); st != "" {
			f.Categories = []string{st}
		} else if !hasFlag(args, "--all") {
			f.Categories = []string{sdd.CategoryBacklog, sdd.CategoryUnstarted, sdd.CategoryStarted}
		}
		list, err := t.ListIssues(f)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(stdout, list)
		}
		for _, is := range list {
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", t.Ref("", is.ID), is.Status.Category, is.Title)
		}
		return nil
	case "get":
		ref := firstPositional(args[1:])
		if ref == "" {
			return fmt.Errorf("tracker get: pass an issue ref")
		}
		backend, id := sdd.ParseIssueRef(ref, t.Backend)
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
		for _, r := range is.Refs {
			fmt.Fprintf(stdout, "  sdd: %s %s %s %s\n", r.Repo, r.Project, r.Change, r.Role)
		}
		return nil
	case "add":
		title := firstPositional(args[1:], "--description")
		if title == "" {
			return fmt.Errorf("tracker add: pass a title")
		}
		is, err := t.CreateIssue(sdd.CreateIssueInput{
			Title:       title,
			Description: flagValue(args, "--description"),
			Refs:        []sdd.SDDRef{{Repo: sdd.RepoName(root), Project: t.Project()}},
			ClientRef:   sdd.NewClientRef(sdd.RepoName(root), "", title, sdd.NewNonce()),
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "added %s\n", t.Ref("", is.ID))
		return nil
	case "close":
		ref := firstPositional(args[1:])
		if ref == "" {
			return fmt.Errorf("tracker close: pass an issue ref")
		}
		backend, id := sdd.ParseIssueRef(ref, t.Backend)
		impl, err := t.For(backend)
		if err != nil {
			return err
		}
		cat := sdd.CategoryCompleted
		if hasFlag(args, "--canceled") {
			cat = sdd.CategoryCanceled
		}
		is, err := impl.TransitionIssue(id, cat, t.Native(cat))
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s %s\n", t.Ref(backend, is.ID), is.Status.Category)
		return nil
	case "start":
		pos := positionals(args[1:])
		if len(pos) != 2 {
			return fmt.Errorf("tracker start: pass <ref> <slug>")
		}
		res, err := sdd.StartFromIssue(root, t, pos[0], pos[1])
		if err != nil {
			return err
		}
		if res.Created {
			fmt.Fprintf(stdout, "created sdd/changes/%s/\n", res.Slug)
		}
		fmt.Fprintf(stdout, "linked %s to %s\n", res.Ref, res.Slug)
		return nil
	case "from-deferred":
		slug := flagValue(args, "--change")
		if slug == "" {
			return fmt.Errorf("tracker from-deferred: pass --change <slug>")
		}
		n, err := sdd.QueueDeferredIssues(root, t, slug)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "queued %d deferred issue(s) for %s\n", n, slug)
		return nil
	case "check":
		results, err := sdd.CheckTracker(root, flagValue(args, "--backend"), hasFlag(args, "--allow-writes"))
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
		fmt.Fprintf(stderr, trackerUsage, sdd.CLIName)
		return fmt.Errorf("unknown tracker subcommand: %s", args[0])
	}
}

func reportDrain(rep sdd.DrainReport, stderr io.Writer) {
	for _, w := range rep.Warnings {
		fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	if rep.Pending > 0 {
		fmt.Fprintf(stderr, "tracker: %d pending (%s tracker outbox)\n", rep.Pending, sdd.CLIName)
	}
}

func trackerOutbox(root string, args []string, stdout, stderr io.Writer) error {
	if seq := flagValue(args, "--drop"); seq != "" {
		var n int
		if _, err := fmt.Sscanf(seq, "%d", &n); err != nil {
			return fmt.Errorf("tracker outbox --drop: %q is not a sequence number", seq)
		}
		if err := sdd.DropOutboxEntry(root, n); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "dropped %d\n", n)
		return nil
	}
	entries, err := sdd.ListOutbox(root)
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

func trackerInit(root string, stdout io.Writer) error {
	action, driver, err := registerMergeDriver(root)
	if err != nil {
		return err
	}
	if action == "unchanged" {
		fmt.Fprintf(stdout, "unchanged merge driver sdd-issues: %s\n", driver)
		return nil
	}
	fmt.Fprintf(stdout, "registered merge driver sdd-issues: %s\n", driver)
	return nil
}

// registerMergeDriver writes the .gitattributes line and the git config for
// the issues.jsonl merge driver. It reports "unchanged" when both are in place.
func registerMergeDriver(root string) (action, driver string, err error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	driver = fmt.Sprintf("%q tracker merge-driver %%O %%A %%B", exe)
	attr := filepath.Join(root, ".gitattributes")
	line := sdd.IssuesFile + " merge=sdd-issues"
	raw, err := os.ReadFile(attr)
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	action = "unchanged"
	if !strings.Contains(string(raw), line) {
		text := string(raw)
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		if err := os.WriteFile(attr, []byte(text+line+"\n"), 0o644); err != nil {
			return "", "", err
		}
		action = "created"
	}
	get := exec.Command("git", "config", "--get", "merge.sdd-issues.driver")
	get.Dir = root
	if out, err := get.Output(); err == nil && strings.TrimSpace(string(out)) == driver {
		return action, driver, nil
	}
	for _, kv := range [][2]string{{"merge.sdd-issues.name", "sdd issues.jsonl merge"}, {"merge.sdd-issues.driver", driver}} {
		cmd := exec.Command("git", "config", kv[0], kv[1])
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", "", fmt.Errorf("git config %s: %v: %s", kv[0], err, strings.TrimSpace(string(out)))
		}
	}
	return "created", driver, nil
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
	merged, conflict, err := sdd.MergeIssues(base, ours, theirs)
	if err != nil {
		return err
	}
	if err := os.WriteFile(args[1], merged, 0o644); err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("conflicting edits in %s; resolve the marked issues", sdd.IssuesFile)
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

var valueFlags = map[string]bool{"--status": true, "--change": true, "--description": true, "--backend": true, "--drop": true,
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
