package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/millerscout/qrtx"
)

const syncUsage = "Usage: %s sync heartbeat|begin|end [--session id] [--hook] [--quiet] [--no-daemon] | claim <key> [--release] | inbox [--json] [--brief] [--hook] | watch [--json] | status [--json] | stop | serve\n"

// hookStdin is where --hook commands read the Claude Code hook JSON.
var hookStdin io.Reader = os.Stdin

type hookInput struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Event     string `json:"hook_event_name"`
}

func readHook(args []string) hookInput {
	var h hookInput
	if hasFlag(args, "--hook") {
		raw, _ := io.ReadAll(io.LimitReader(hookStdin, 1<<20))
		_ = json.Unmarshal(raw, &h)
	}
	return h
}

func runSync(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintf(stderr, syncUsage, qrtx.CLIName)
		return fmt.Errorf("missing sync subcommand")
	}
	if hasFlag(args, "--no-daemon") {
		qrtx.NoDaemon = true
	}
	hook := readHook(args)
	root, err := qrtx.RepoRoot()
	if err != nil {
		return err
	}
	if hook.Cwd != "" && os.Getenv(qrtx.EnvRepo) == "" {
		if wd, err := os.Getwd(); err == nil && wd != hook.Cwd {
			if err := os.Chdir(hook.Cwd); err == nil {
				if r, err := qrtx.RepoRoot(); err == nil {
					root = r
				}
				_ = os.Chdir(wd)
			}
		}
	}
	who := qrtx.ResolveSession(root, flagValue(args, "--session"), hook.SessionID)
	now := time.Now()
	// Hook-facing commands never fail the caller: a failing UserPromptSubmit
	// hook would block the user's prompt.
	soft := func(err error) error {
		if err != nil {
			fmt.Fprintf(stderr, "qrtx sync %s: warning: %v\n", args[0], err)
		}
		return nil
	}
	switch args[0] {
	case "heartbeat":
		res, err := qrtx.Heartbeat(root, who, now, true)
		if err == nil && !hasFlag(args, "--quiet") {
			switch {
			case res.Debounced:
				fmt.Fprintf(stdout, "%s: lease recent, nothing to do\n", who.ID)
			default:
				fmt.Fprintf(stdout, "%s: lease until %s\n", who.ID, res.LeaseUntil.Format(time.RFC3339))
			}
			if res.DaemonStarted {
				fmt.Fprintln(stdout, "started sync daemon")
			}
		}
		return soft(err)
	case "begin":
		return soft(qrtx.BeginSession(root, who, now, os.Getenv("CLAUDE_ENV_FILE"), true))
	case "end":
		return soft(qrtx.EndSession(root, who, now))
	case "claim":
		pos := positionals(args[1:], "--session")
		if len(pos) != 1 {
			return fmt.Errorf("sync claim: pass one key (an issue ref such as github:12, or <slug>/<task id>)")
		}
		release := hasFlag(args, "--release")
		err := qrtx.Claim(root, who, pos[0], release, now, true)
		var held *qrtx.ClaimHeldError
		switch {
		case errors.Is(err, qrtx.ErrSyncNotEnabled):
			fmt.Fprintln(stdout, "sync not enabled; claim skipped")
			return nil
		case errors.As(err, &held):
			return err
		case err != nil:
			return err
		case release:
			fmt.Fprintf(stdout, "released %s\n", pos[0])
		default:
			fmt.Fprintf(stdout, "claimed %s for %s\n", pos[0], who.ID)
		}
		return nil
	case "inbox":
		wait := time.Duration(0)
		if hasFlag(args, "--hook") {
			wait = time.Second
		}
		evs, err := qrtx.Inbox(root, who, now, wait)
		if err != nil {
			if hasFlag(args, "--hook") {
				return soft(err)
			}
			return err
		}
		if hasFlag(args, "--json") {
			if evs == nil {
				evs = []qrtx.SyncEvent{}
			}
			return writeJSON(stdout, evs)
		}
		limit := len(evs)
		if hasFlag(args, "--brief") && limit > 10 {
			limit = 10
		}
		if len(evs) > 0 && hasFlag(args, "--hook") {
			fmt.Fprintln(stdout, "Other qrtx sessions since your last prompt:")
		}
		for _, e := range evs[:limit] {
			fmt.Fprintf(stdout, "- %s\n", e.Summary())
		}
		if limit < len(evs) {
			fmt.Fprintf(stdout, "…and %d more (%s sync inbox)\n", len(evs)-limit, qrtx.CLIName)
		}
		return nil
	case "watch":
		var last int64 = -1
		for {
			evs, err := qrtx.EventsAfter(root, last)
			if err != nil {
				return err
			}
			if last < 0 {
				if len(evs) > 0 {
					last = evs[len(evs)-1].Seq
				} else {
					last = 0
				}
				evs = nil
			}
			for _, e := range evs {
				if hasFlag(args, "--json") {
					b, _ := json.Marshal(e)
					fmt.Fprintln(stdout, string(b))
				} else {
					fmt.Fprintf(stdout, "%s %s\n", e.At.Local().Format("15:04:05"), e.Summary())
				}
				last = e.Seq
			}
			if f, ok := stdout.(interface{ Sync() error }); ok {
				_ = f.Sync()
			}
			select {
			case <-watchStop:
				return nil
			case <-time.After(watchPoll):
			}
		}
	case "status":
		st, err := qrtx.ReadSyncStatus(root, now)
		if err != nil {
			return err
		}
		if hasFlag(args, "--json") {
			return writeJSON(stdout, st)
		}
		if !st.Enabled {
			fmt.Fprintf(stdout, "sync not enabled here (run %s sync heartbeat, or install hooks with %s skills install --hooks)\n", qrtx.CLIName, qrtx.CLIName)
			return nil
		}
		if st.Daemon != nil {
			fmt.Fprintf(stdout, "daemon: running (pid %d since %s)\n", st.Daemon.PID, st.Daemon.Started.Local().Format(time.RFC3339))
		} else {
			fmt.Fprintln(stdout, "daemon: not running")
		}
		sort.Slice(st.Sessions, func(i, j int) bool { return st.Sessions[i].ID < st.Sessions[j].ID })
		for _, s := range st.Sessions {
			state := "expired"
			if s.Live {
				state = "live until " + s.LeaseUntil.Local().Format("15:04:05")
			}
			fmt.Fprintf(stdout, "session %s  %s  %s %s\n", s.ID, state, s.Worktree, s.Branch)
		}
		for _, c := range st.Claims {
			fmt.Fprintf(stdout, "claim %s  by %s\n", c.Key, c.Session)
		}
		fmt.Fprintf(stdout, "outbox: %d pending\n", st.Outbox)
		return nil
	case "stop":
		was, err := qrtx.StopDaemon(root, 5*time.Second)
		if err != nil {
			return err
		}
		if was {
			fmt.Fprintln(stdout, "sync daemon stopped")
		} else {
			fmt.Fprintln(stdout, "sync daemon not running")
		}
		return nil
	case "serve":
		return qrtx.RunDaemon(root, stdout)
	default:
		fmt.Fprintf(stderr, syncUsage, qrtx.CLIName)
		return fmt.Errorf("unknown sync subcommand: %s", args[0])
	}
}

// watchStop ends `sync watch` (tests); watchPoll is its polling interval.
var (
	watchStop chan struct{}
	watchPoll = 500 * time.Millisecond
)
