package cli

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/millerscout/qrtx"
	"github.com/millerscout/qrtx/providers/kit"
	"github.com/millerscout/qrtx/skills"
)

// Prompting reads from promptIn, and only when isTerminal reports a terminal.
var (
	promptIn   io.Reader = os.Stdin
	isTerminal           = func() bool {
		fi, err := os.Stdin.Stat()
		return err == nil && fi.Mode()&os.ModeCharDevice != 0
	}
	promptReader *bufio.Reader
)

func ask(stdout io.Writer, question string) string {
	if promptReader == nil {
		promptReader = bufio.NewReader(promptIn)
	}
	fmt.Fprint(stdout, question)
	line, _ := promptReader.ReadString('\n')
	return strings.TrimSpace(line)
}

// need returns the flag's value, prompting for it on a terminal; otherwise a
// missing value is an error naming the flag.
func need(args []string, flag, question string, stdout io.Writer) (string, error) {
	if v := flagValue(args, flag); v != "" {
		return v, nil
	}
	if isTerminal() {
		if v := ask(stdout, question+": "); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("missing %s", flag)
}

type step struct{ action, what string }

func printSteps(w io.Writer, steps []step) {
	for _, s := range steps {
		fmt.Fprintf(w, "%-10s %s\n", s.action, s.what)
	}
}

func hostOf(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%q is not a URL", raw)
	}
	return strings.ToLower(u.Host), nil
}

// trackerSetup writes the tracker block, binds credentials and runs a
// read-only check. It prints its steps and returns them for `qrtx init`.
func trackerSetup(root string, args []string, stdout io.Writer) ([]step, error) {
	pos := positionals(args)
	backend := ""
	if len(pos) > 0 {
		backend = pos[0]
	}
	var cfg map[string]any
	var host, scope string
	bind := kit.Binding{TokenEnv: flagValue(args, "--token-env"), EmailEnv: flagValue(args, "--email-env"), TokenCommand: flagValue(args, "--token-command")}
	switch backend {
	case "github":
		repo, err := need(args, "--repo", "GitHub repository (owner/name)", stdout)
		if err != nil {
			return nil, err
		}
		cfg = map[string]any{"repo": repo}
		api := flagValue(args, "--api-url")
		if api != "" {
			cfg["api_url"] = api
		} else {
			api = "https://api.github.com"
		}
		if host, err = hostOf(api); err != nil {
			return nil, err
		}
		scope = repo
		if bind.TokenEnv == "" && bind.TokenCommand == "" {
			v, err := need(args, "--token-command", "Command that prints a GitHub token (for example: gh auth token)", stdout)
			if err != nil {
				return nil, fmt.Errorf("missing --token-env or --token-command")
			}
			bind.TokenCommand = v
		}
	case "jira":
		site, err := need(args, "--url", "Jira site URL (https://<site>.atlassian.net)", stdout)
		if err != nil {
			return nil, err
		}
		project, err := need(args, "--project", "Jira project key", stdout)
		if err != nil {
			return nil, err
		}
		cfg = map[string]any{"url": site, "project": project}
		if it := flagValue(args, "--issue-type"); it != "" {
			cfg["issue_type"] = it
		}
		if host, err = hostOf(site); err != nil {
			return nil, err
		}
		scope = project
		if bind.EmailEnv == "" {
			if bind.EmailEnv, err = need(args, "--email-env", "Environment variable holding your Atlassian email", stdout); err != nil {
				return nil, err
			}
		}
		if bind.TokenEnv == "" && bind.TokenCommand == "" {
			if bind.TokenEnv, err = need(args, "--token-env", "Environment variable holding your Atlassian API token", stdout); err != nil {
				return nil, fmt.Errorf("missing --token-env or --token-command")
			}
		}
	case "":
		return nil, fmt.Errorf("tracker setup: name a backend (github or jira)")
	default:
		return nil, fmt.Errorf("tracker setup: %q is not supported here; set tracker.backend in %s by hand (see docs/providers.md)", backend, qrtx.RepoConfigFile)
	}
	bind.Scopes = []string{scope}
	var steps []step

	changed, err := qrtx.SetTrackerConfig(root, qrtx.TrackerConfig{Backend: backend, Config: cfg})
	if err != nil {
		return steps, err
	}
	steps = append(steps, step{actionOf(changed), qrtx.RepoConfigFile + " tracker block (" + backend + ")"})

	path, err := kit.CredentialsPath()
	if err != nil {
		return steps, err
	}
	binds, err := kit.LoadBindings(path)
	if err != nil {
		return steps, err
	}
	if have, ok := binds[host]; ok && sameSources(have, bind) && containsFold(have.Scopes, scope) {
		steps = append(steps, step{"unchanged", fmt.Sprintf("%s binding for %s (%s)", path, host, scope)})
	} else {
		fmt.Fprintf(stdout, "credentials file %s will bind host %s:\n  %s\n", path, host, describeBinding(bind))
		ok := hasFlag(args, "--yes")
		if !ok && isTerminal() {
			ok = strings.HasPrefix(strings.ToLower(ask(stdout, "Write this binding? [y/N] ")), "y")
		} else if !ok {
			return steps, fmt.Errorf("pass --yes to write the credentials binding (no terminal to ask)")
		}
		if !ok {
			fmt.Fprintf(stdout, "not written. To bind it by hand, add under hosts: in %s:\n  %s: {%s}\n", path, host, describeBinding(bind))
			steps = append(steps, step{"skipped", "credentials binding (declined)"})
			printSteps(stdout, steps)
			return steps, nil
		}
		if _, err := kit.BindHost(path, host, bind); err != nil {
			return steps, err
		}
		steps = append(steps, step{"updated", fmt.Sprintf("%s binding for %s (%s)", path, host, scope)})
	}

	bin := "qrtx-tracker-" + backend
	if _, err := exec.LookPath(bin); err != nil {
		steps = append(steps, step{"skipped", "check: " + bin + " is not on PATH; install it with: go install github.com/millerscout/qrtx/cmd/" + bin + "@latest"})
		printSteps(stdout, steps)
		return steps, nil
	}
	results, err := qrtx.CheckTracker(root, "", false)
	if err != nil {
		printSteps(stdout, steps)
		return steps, err
	}
	failed := 0
	for _, r := range results {
		mark := "pass"
		if !r.OK {
			mark, failed = "FAIL", failed+1
		}
		steps = append(steps, step{"check", mark + " " + r.Op + " " + r.Detail})
	}
	printSteps(stdout, steps)
	if failed > 0 {
		return steps, fmt.Errorf("tracker setup: %d check(s) failed", failed)
	}
	return steps, nil
}

func actionOf(changed bool) string {
	if changed {
		return "updated"
	}
	return "unchanged"
}

func sameSources(a, b kit.Binding) bool {
	return a.TokenEnv == b.TokenEnv && a.EmailEnv == b.EmailEnv && a.TokenCommand == b.TokenCommand
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if v == "*" || strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

func describeBinding(b kit.Binding) string {
	var parts []string
	if b.TokenEnv != "" {
		parts = append(parts, "token_env: "+b.TokenEnv)
	}
	if b.EmailEnv != "" {
		parts = append(parts, "email_env: "+b.EmailEnv)
	}
	if b.TokenCommand != "" {
		parts = append(parts, fmt.Sprintf("token_command: %q", b.TokenCommand))
	}
	parts = append(parts, "scopes: ["+strings.Join(b.Scopes, ", ")+"]")
	return strings.Join(parts, ", ")
}

func inGit(root string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = root
	return cmd.Run() == nil
}

// runInit sets up a repository or folder: skills, constitution, the
// issues.jsonl merge driver and, with --tracker, the tracker.
func runInit(args []string, stdout, stderr io.Writer) error {
	root, err := qrtx.RepoRoot()
	if err != nil {
		return err
	}
	var steps []step
	if !hasFlag(args, "--no-skills") {
		res, err := skills.InstallKeep(filepath.Join(root, ".claude", "skills"))
		if err != nil {
			return err
		}
		for _, r := range res {
			what := "skill " + r.Name
			if r.Action == "skipped" {
				what += " (local edits; use " + qrtx.CLIName + " skills install --force)"
			}
			steps = append(steps, step{r.Action, what})
		}
	}
	if !hasFlag(args, "--no-constitution") {
		files, err := qrtx.ConstitutionFiles(root)
		if err != nil {
			return err
		}
		if len(files) > 0 {
			steps = append(steps, step{"unchanged", "constitution (" + strings.Join(files, ", ") + ")"})
		} else {
			path, err := qrtx.InitConstitution(root, false)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			steps = append(steps, step{"created", "constitution " + filepath.ToSlash(rel)})
		}
	}
	if inGit(root) {
		action, _, err := registerMergeDriver(root)
		if err != nil {
			return err
		}
		steps = append(steps, step{action, "issues.jsonl merge driver"})
	} else {
		steps = append(steps, step{"skipped", "issues.jsonl merge driver (not a git repository)"})
	}
	printSteps(stdout, steps)
	if backend := flagValue(args, "--tracker"); backend != "" {
		setupArgs := append([]string{backend}, args...)
		if _, err := trackerSetup(root, setupArgs, stdout); err != nil {
			return err
		}
	}
	return nil
}
