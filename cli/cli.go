// Package cli is the command-line front end shared by the sdd binary and
// hosts that embed sdd under their own command name.
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
	"github.com/millerscout/sdd/skills"
)

// Run executes one sdd subcommand: verify-ast, graph, sync-from-code, mutate,
// change, skills or constitution.
func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Usage: %s verify-ast|graph|sync-from-code|mutate|change|skills|constitution|tracker|init|sync\n", sdd.CLIName)
		return fmt.Errorf("missing subcommand")
	}
	if args[0] == "change" {
		return runChange(args[1:], stdout, stderr)
	}
	if args[0] == "skills" {
		return runSkills(args[1:], stdout, stderr)
	}
	if args[0] == "constitution" {
		return runConstitution(args[1:], stdout, stderr)
	}
	if args[0] == "tracker" {
		return runTracker(args[1:], stdout, stderr)
	}
	if args[0] == "init" {
		return runInit(args[1:], stdout, stderr)
	}
	if args[0] == "sync" {
		return runSync(args[1:], stdout, stderr)
	}
	switch args[0] {
	case "graph":
		root, err := sdd.RepoRoot()
		if err != nil {
			return err
		}
		path, err := sdd.WriteGraph(root)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "wrote %s\n", path)
		return nil
	case "sync-from-code":
		root, err := sdd.RepoRoot()
		if err != nil {
			return err
		}
		var pkgs []string
		driftedOnly := false
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--package":
				i++
				if i < len(args) {
					pkgs = append(pkgs, args[i])
				}
			case "--drifted-only":
				driftedOnly = true
			}
		}
		if driftedOnly {
			return sdd.SyncDriftedPackages(root, pkgs)
		}
		for _, pkg := range pkgs {
			if err := sdd.SyncPackageFromCode(root, pkg); err != nil {
				return err
			}
		}
		if len(pkgs) == 0 {
			return sdd.SyncDriftedPackages(root, nil)
		}
		return nil
	case "verify-ast":
		root, err := sdd.RepoRoot()
		if err != nil {
			return err
		}
		var pkgs []string
		for i := 1; i < len(args); i++ {
			if args[i] == "--package" && i+1 < len(args) {
				pkgs = append(pkgs, args[i+1])
				i++
			}
		}
		return sdd.VerifyAST(stdout, root, pkgs)
	case "mutate":
		root, err := sdd.RepoRoot()
		if err != nil {
			return err
		}
		var pkgs []string
		for i := 1; i < len(args); i++ {
			if args[i] == "--package" && i+1 < len(args) {
				pkgs = append(pkgs, args[i+1])
				i++
			}
		}
		if len(pkgs) == 0 {
			return fmt.Errorf("sdd mutate: pass --package <path> (repeatable)")
		}
		for _, pkg := range pkgs {
			if err := sdd.MutatePackage(root, pkg); err != nil {
				return err
			}
		}
		return nil
	default:
		fmt.Fprintf(stderr, "Usage: %s verify-ast|graph|sync-from-code|mutate|change|skills|constitution|tracker|init|sync\n", sdd.CLIName)
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func runChange(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Usage: %s change new|list|status|verify|score|review-packet|lint-active|propose-context|finish-propose|finish-apply|run-validate|archive\n", sdd.CLIName)
		return fmt.Errorf("missing change subcommand")
	}
	root, err := sdd.RepoRoot()
	if err != nil {
		return err
	}
	switch args[0] {
	case "new":
		if len(args) < 2 {
			return fmt.Errorf("sdd change new: pass change slug")
		}
		if err := sdd.NewChange(root, args[1]); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "created sdd/changes/%s/\n", strings.TrimSpace(args[1]))
		return nil
	case "list":
		slugs, err := sdd.ListChanges(root)
		if err != nil {
			return err
		}
		for _, slug := range slugs {
			_, _ = fmt.Fprintln(stdout, slug)
		}
		return nil
	case "status":
		slug := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "--change" && i+1 < len(args) {
				slug = args[i+1]
				i++
			}
		}
		if slug == "" {
			slugs, err := sdd.ListChanges(root)
			if err != nil {
				return err
			}
			if len(slugs) == 0 {
				return fmt.Errorf("no active SDD changes")
			}
			slug = slugs[0]
		}
		sum, err := sdd.SummarizeChange(root, slug)
		if err != nil {
			return err
		}
		out := map[string]any{
			"slug":        sum.Slug,
			"status":      sum.Status,
			"tasks_total": sum.TasksTotal,
			"tasks_done":  sum.TasksDone,
			"open_phase":  sum.OpenPhase,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	case "verify":
		slug := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "--change" && i+1 < len(args) {
				slug = args[i+1]
				i++
			}
		}
		if slug == "" && len(args) >= 2 && args[1] != "--change" {
			slug = args[1]
		}
		if slug == "" {
			return fmt.Errorf("sdd change verify: pass --change <slug>")
		}
		if err := sdd.ValidateChangeManifest(root, slug); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "sdd change verify: OK (%s)\n", strings.TrimSpace(slug))
		return nil
	case "score":
		slug := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "--change" && i+1 < len(args) {
				slug = args[i+1]
				i++
			}
		}
		if slug == "" {
			return fmt.Errorf("sdd change score: pass --change <slug>")
		}
		sc, err := sdd.ScoreChangeManifest(root, slug)
		if err != nil {
			return err
		}
		out := sdd.ScoreJSON(sc)
		enc := json.NewEncoder(stdout)
		return enc.Encode(out)
	case "review-packet":
		slug := ""
		asJSON := false
		for i := 1; i < len(args); i++ {
			switch {
			case args[i] == "--change" && i+1 < len(args):
				slug = args[i+1]
				i++
			case args[i] == "--json":
				asJSON = true
			}
		}
		if slug == "" {
			return fmt.Errorf("sdd change review-packet: pass --change <slug>")
		}
		pkt, err := sdd.BuildReviewPacket(root, slug)
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(stdout).Encode(pkt)
		}
		_, _ = fmt.Fprint(stdout, sdd.FormatReviewPacket(pkt))
		return nil
	case "lint-active":
		if err := sdd.LintActiveChanges(root, stdout); err != nil {
			return err
		}
		return nil
	case "propose-context":
		slug := ""
		jsonOut := false
		refresh := false
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--change":
				i++
				if i < len(args) {
					slug = args[i]
				}
			case "--json":
				jsonOut = true
			case "--refresh-graph":
				refresh = true
			}
		}
		if slug == "" {
			return fmt.Errorf("sdd change propose-context: pass --change <slug>")
		}
		rep, err := sdd.ProposeContext(root, slug, sdd.ProposeContextOpts{RefreshGraph: refresh})
		if err != nil {
			return err
		}
		for _, w := range rep.Warnings {
			_, _ = fmt.Fprintf(stderr, "warning: %s\n", w)
		}
		if jsonOut {
			return sdd.WriteProposeContextJSON(stdout, rep)
		}
		return sdd.WriteProposeContextHuman(stdout, rep)
	case "finish-propose":
		slug := slugFromArgs(args[1:])
		if slug == "" {
			return fmt.Errorf("sdd change finish-propose: pass --change <slug>")
		}
		return sdd.FinishPropose(root, slug, stdout, stderr)
	case "finish-apply":
		slug := slugFromArgs(args[1:])
		if slug == "" {
			return fmt.Errorf("sdd change finish-apply: pass --change <slug>")
		}
		return sdd.FinishApply(root, slug, stdout, stderr)
	case "run-validate":
		slug := slugFromArgs(args[1:])
		if slug == "" {
			return fmt.Errorf("sdd change run-validate: pass --change <slug>")
		}
		return sdd.RunValidateTasks(root, slug, stdout, stderr)
	case "archive":
		slug := ""
		runValidate := false
		quickCheck := false
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--run-validate":
				runValidate = true
			case "--quick-check", "--quick-golden":
				quickCheck = true
			case "--change":
				i++
				if i < len(args) {
					slug = args[i]
				}
			default:
				if slug == "" && !strings.HasPrefix(args[i], "--") {
					slug = args[i]
				}
			}
		}
		if slug == "" {
			return fmt.Errorf("sdd change archive: pass change slug")
		}
		if runValidate {
			if err := sdd.RunValidateTasks(root, slug, stdout, stderr); err != nil {
				return err
			}
		}
		if quickCheck {
			if err := sdd.RunQuickCheck(root, stdout, stderr); err != nil {
				return err
			}
		}
		linked := false
		if man, err := sdd.ParseChangeFile(root, strings.TrimSpace(slug)); err == nil {
			linked = len(man.Issues) > 0
		}
		if err := sdd.ArchiveChange(root, slug); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "archived sdd/changes/%s\n", strings.TrimSpace(slug))
		if linked {
			t, err := sdd.OpenTracker(root)
			if err != nil {
				fmt.Fprintf(stderr, "warning: tracker: %v; the writes stay queued (%s tracker outbox)\n", err, sdd.CLIName)
				return nil
			}
			reportDrain(sdd.DrainOutbox(root, t), stderr)
		}
		return nil
	default:
		fmt.Fprintf(stderr, "Usage: %s change new|list|status|verify|score|review-packet|lint-active|propose-context|finish-propose|finish-apply|run-validate|archive\n", sdd.CLIName)
		return fmt.Errorf("unknown change subcommand: %s", args[0])
	}
}

func slugFromArgs(args []string) string {
	slug := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--change" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
		if slug == "" && args[i] != "" && !strings.HasPrefix(args[i], "--") {
			slug = args[i]
		}
	}
	return strings.TrimSpace(slug)
}

// runSkills lists or installs the bundled agent skills.
func runSkills(args []string, stdout, stderr io.Writer) error {
	usage := func() {
		fmt.Fprintf(stderr, "Usage: %s skills list | install [--dir <dir>] [--force] [--hooks [--shared]]\n", sdd.CLIName)
	}
	if len(args) == 0 {
		usage()
		return fmt.Errorf("missing skills subcommand")
	}
	switch args[0] {
	case "list":
		for _, name := range skills.Names {
			_, _ = fmt.Fprintln(stdout, name)
		}
		return nil
	case "install":
		dir, force, hooks, shared := "", false, false, false
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--force":
				force = true
			case "--hooks":
				hooks = true
			case "--shared":
				shared = true
			case "--dir":
				i++
				if i >= len(args) {
					return fmt.Errorf("skills install: --dir needs a value")
				}
				dir = args[i]
			default:
				usage()
				return fmt.Errorf("unknown flag: %s", args[i])
			}
		}
		if dir == "" {
			root, err := sdd.RepoRoot()
			if err != nil {
				return err
			}
			dir = filepath.Join(root, ".claude", "skills")
		}
		results, err := skills.Install(dir, force)
		for _, r := range results {
			_, _ = fmt.Fprintf(stdout, "%s %s\n", r.Action, r.Path)
		}
		if err != nil || !hooks {
			return err
		}
		root, err := sdd.RepoRoot()
		if err != nil {
			return err
		}
		name := "settings.local.json"
		if shared {
			name = "settings.json"
		}
		path := filepath.Join(root, ".claude", name)
		changed, err := skills.InstallHooks(path, hookCommand())
		if err != nil {
			return err
		}
		action := "unchanged"
		if changed {
			action = "updated"
		}
		_, _ = fmt.Fprintf(stdout, "%s %s (sdd sync hooks)\n", action, path)
		if !shared {
			if added, err := ensureGitignored(root, ".claude/settings.local.json"); err != nil {
				return err
			} else if added {
				_, _ = fmt.Fprintf(stdout, "updated %s (ignores .claude/settings.local.json)\n", filepath.Join(root, ".gitignore"))
			}
			_, _ = fmt.Fprintf(stdout, "note: these hooks apply to this clone only; run with --shared to commit them in .claude/settings.json\n")
		}
		return nil
	default:
		usage()
		return fmt.Errorf("unknown skills subcommand: %s", args[0])
	}
}

// runConstitution shows the project's constitution or scaffolds one.
func runConstitution(args []string, stdout, stderr io.Writer) error {
	usage := func() { fmt.Fprintf(stderr, "Usage: %s constitution show [--json] | init [--force]\n", sdd.CLIName) }
	if len(args) == 0 {
		usage()
		return fmt.Errorf("missing constitution subcommand")
	}
	root, err := sdd.RepoRoot()
	if err != nil {
		return err
	}
	switch args[0] {
	case "show":
		asJSON := len(args) > 1 && args[1] == "--json"
		docs, err := sdd.LoadConstitution(root)
		if err != nil {
			return err
		}
		if asJSON {
			if docs == nil {
				docs = []sdd.ConstitutionDoc{}
			}
			return json.NewEncoder(stdout).Encode(docs)
		}
		if len(docs) == 0 {
			_, _ = fmt.Fprintf(stdout, "no constitution (add %s or list files under `constitution:` in %s; `%s constitution init` writes a template)\n", sdd.DefaultConstitutionFile, sdd.RepoConfigFile, sdd.CLIName)
			return nil
		}
		for _, d := range docs {
			_, _ = fmt.Fprintln(stdout, d.Path)
			for _, r := range d.Rules {
				_, _ = fmt.Fprintf(stdout, "  - %s\n", r)
			}
		}
		return nil
	case "init":
		force := len(args) > 1 && args[1] == "--force"
		path, err := sdd.InitConstitution(root, force)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "wrote %s\n", path)
		return nil
	default:
		usage()
		return fmt.Errorf("unknown constitution subcommand: %s", args[0])
	}
}

// hookCommand is how installed hooks call sdd: by name when it is on PATH,
// else by this binary's absolute path.
func hookCommand() string {
	if p, err := exec.LookPath(sdd.CLIName); err == nil && p != "" {
		return sdd.CLIName
	}
	if exe, err := os.Executable(); err == nil {
		return fmt.Sprintf("%q", exe)
	}
	return sdd.CLIName
}

// ensureGitignored appends entry to root/.gitignore unless git already
// ignores it. It reports whether it changed the file.
func ensureGitignored(root, entry string) (bool, error) {
	check := exec.Command("git", "check-ignore", "-q", entry)
	check.Dir = root
	if err := check.Run(); err == nil {
		return false, nil
	} else if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
		return false, nil // not a git repository, or no git
	}
	path := filepath.Join(root, ".gitignore")
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	b = append(b, entry+"\n"...)
	return true, os.WriteFile(path, b, 0o644)
}
