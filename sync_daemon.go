package qrtx

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"
)

const (
	daemonLockName = "daemon.lock"
	daemonInfoFile = "daemon.json"
	daemonStopFile = "stop"
	syncLogFile    = "sync.log"
)

// NoDaemon is set by --no-daemon; with it (or CI, or QRTX_NO_DAEMON=1) no
// command starts the sync daemon.
var NoDaemon bool

func daemonDisabled() bool {
	return NoDaemon || os.Getenv("CI") != "" || os.Getenv("QRTX_NO_DAEMON") == "1"
}

// daemonRunning reports whether a process holds daemon.lock in dir.
func daemonRunning(dir string) bool {
	unlock, ok, err := tryLockFile(filepath.Join(dir, daemonLockName))
	if err != nil {
		return false
	}
	if ok {
		unlock()
		return false
	}
	return true
}

// DaemonCommand returns the program and arguments that start the daemon.
// Hosts that embed qrtx under another binary may replace it.
var DaemonCommand = func() (string, []string, error) {
	exe, err := os.Executable()
	return exe, []string{"sync", "serve"}, err
}

// ensureDaemonLocked starts the daemon when none runs. The caller holds the
// sync lock, which the daemon also holds while it decides to exit, so a
// heartbeat never sees a daemon that is about to leave.
func ensureDaemonLocked(repoRoot, dir string) (bool, error) {
	if daemonDisabled() || daemonRunning(dir) {
		return false, nil
	}
	prog, args, err := DaemonCommand()
	if err != nil {
		return false, err
	}
	logf, err := openSyncLog(dir)
	if err != nil {
		return false, err
	}
	defer logf.Close()
	cmd := exec.Command(prog, args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), EnvRepo+"="+repoRoot)
	cmd.Stdout, cmd.Stderr = logf, logf
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return false, err
	}
	_ = cmd.Process.Release()
	return true, nil
}

func openSyncLog(dir string) (*os.File, error) {
	path := filepath.Join(dir, syncLogFile)
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		_ = os.Rename(path, path+".1")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// DaemonInfo is daemon.json.
type DaemonInfo struct {
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
	Root    string    `json:"root"`
}

// daemonHooks lets the watch and poll phases plug into the tick; tests and
// later code may extend it.
type daemonWork func(root string, logw io.Writer)

var daemonRootWork []daemonWork

// RunDaemon is `qrtx sync serve`. It returns at once when another daemon
// holds the lock. While a lease is live it expires leases every tick and, in
// a goroutine that never blocks expiry, drains each live root's outbox and
// runs the registered per-root work. With no live lease (or a stop request)
// it drains once more and, under the sync lock, releases its lock and exits.
func RunDaemon(repoRoot string, logw io.Writer) error {
	dir, err := ensureTrackerStateDir(repoRoot)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(dir, daemonLockName)
	unlock, ok, err := tryLockFile(lockPath)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	released := false
	defer func() {
		if !released {
			unlock()
		}
	}()
	logf := func(format string, a ...any) {
		fmt.Fprintf(logw, "%s pid %d: %s\n", time.Now().UTC().Format(time.RFC3339), os.Getpid(), fmt.Sprintf(format, a...))
	}
	_ = os.Remove(filepath.Join(dir, daemonStopFile))
	info, _ := json.Marshal(DaemonInfo{PID: os.Getpid(), Started: time.Now().UTC(), Root: canonicalRoot(repoRoot)})
	_ = os.WriteFile(filepath.Join(dir, daemonInfoFile), info, 0o644)
	set := loadSyncSettings(repoRoot)
	logf("started (tick %s, interval %s)", set.Tick, set.Interval)
	_ = withSync(repoRoot, time.Now(), 0, true, func(tx *syncTx) error { return trimEventsLocked(tx.dir, tx.now) })

	var busy int32
	runRoots := func(roots []string) {
		for _, r := range roots {
			drainRoot(r, logf)
			for _, w := range daemonRootWork {
				w(r, logw)
			}
		}
	}
	var known []string
	for {
		refreshLock(lockPath)
		_, stopErr := os.Stat(filepath.Join(dir, daemonStopFile))
		stop := stopErr == nil
		var roots []string
		err := withSync(repoRoot, time.Now(), 0, true, func(tx *syncTx) error {
			roots = liveRoots(tx)
			return nil
		})
		if err != nil {
			logf("tick: %v", err)
		}
		if len(roots) > 0 {
			known = roots
		}
		if stop || len(roots) == 0 {
			for i := 0; i < 600 && atomic.LoadInt32(&busy) == 1; i++ {
				time.Sleep(50 * time.Millisecond)
			}
			for _, r := range known {
				drainRoot(r, logf)
			}
			exit := false
			_ = withSync(repoRoot, time.Now(), 0, true, func(tx *syncTx) error {
				if !stop && len(liveRoots(tx)) > 0 {
					return nil // a heartbeat arrived meanwhile: keep serving
				}
				exit = true
				_ = os.Remove(filepath.Join(dir, daemonInfoFile))
				_ = os.Remove(filepath.Join(dir, daemonStopFile))
				unlock()
				released = true
				return nil
			})
			if exit {
				if stop {
					logf("exiting: stop requested")
				} else {
					logf("exiting: no live sessions")
				}
				return nil
			}
			continue
		}
		if atomic.CompareAndSwapInt32(&busy, 0, 1) {
			go func(rs []string) {
				defer atomic.StoreInt32(&busy, 0)
				runRoots(rs)
			}(roots)
		}
		sleepUnlessStopped(dir, set.Tick)
	}
}

// sleepUnlessStopped waits for d, waking early when a stop is requested.
func sleepUnlessStopped(dir string, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, daemonStopFile)); err == nil {
			return
		}
		step := 200 * time.Millisecond
		if rem := time.Until(deadline); rem < step {
			step = rem
		}
		time.Sleep(step)
	}
}

func liveRoots(tx *syncTx) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range tx.state.Sessions {
		if s.LeaseUntil.After(tx.now) && !seen[s.Root] {
			seen[s.Root] = true
			out = append(out, s.Root)
		}
	}
	return out
}

func drainRoot(root string, logf func(string, ...any)) {
	if _, err := os.Stat(outboxPath(root)); err != nil {
		return
	}
	t, err := OpenTracker(root)
	if err != nil {
		logf("drain %s: %v", root, err)
		return
	}
	rep := DrainOutbox(root, t)
	if rep.Sent > 0 || len(rep.Warnings) > 0 {
		logf("drain %s: sent %d, pending %d %v", root, rep.Sent, rep.Pending, rep.Warnings)
	}
}

// StopDaemon asks a running daemon to exit and waits up to wait for it.
// It returns false when no daemon was running.
func StopDaemon(repoRoot string, wait time.Duration) (bool, error) {
	dir, _ := trackerStateDir(repoRoot)
	if _, err := os.Stat(dir); err != nil || !daemonRunning(dir) {
		return false, nil
	}
	if err := os.WriteFile(filepath.Join(dir, daemonStopFile), nil, 0o644); err != nil {
		return true, err
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if !daemonRunning(dir) {
			return true, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true, fmt.Errorf("sync daemon did not stop within %s", wait)
}

// SyncStatus is `qrtx sync status --json`.
type SyncStatus struct {
	Enabled  bool                `json:"enabled"`
	Daemon   *DaemonInfo         `json:"daemon"`
	Sessions []SyncSessionStatus `json:"sessions"`
	Claims   []syncClaim         `json:"claims"`
	Outbox   int                 `json:"outbox"`
	LastPoll map[string]string   `json:"last_poll"`
}

// SyncSessionStatus is one session in SyncStatus.
type SyncSessionStatus struct {
	ID         string    `json:"id"`
	Live       bool      `json:"live"`
	LeaseUntil time.Time `json:"lease_until"`
	Worktree   string    `json:"worktree,omitempty"`
	Branch     string    `json:"branch,omitempty"`
}

// ReadSyncStatus reads the state without changing it (expired sessions are
// listed with live false) and creates nothing.
func ReadSyncStatus(repoRoot string, now time.Time) (SyncStatus, error) {
	st := SyncStatus{Sessions: []SyncSessionStatus{}, Claims: []syncClaim{}, LastPoll: map[string]string{}}
	dir, _ := trackerStateDir(repoRoot)
	raw, err := os.ReadFile(filepath.Join(dir, syncStateFile))
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	s := newSyncState()
	if err := json.Unmarshal(raw, s); err != nil {
		return st, err
	}
	s.fill()
	st.Enabled = true
	if daemonRunning(dir) {
		var info DaemonInfo
		if b, err := os.ReadFile(filepath.Join(dir, daemonInfoFile)); err == nil && json.Unmarshal(b, &info) == nil {
			st.Daemon = &info
		} else {
			st.Daemon = &DaemonInfo{}
		}
	}
	for id, ss := range s.Sessions {
		st.Sessions = append(st.Sessions, SyncSessionStatus{ID: id, Live: ss.LeaseUntil.After(now), LeaseUntil: ss.LeaseUntil, Worktree: ss.Worktree, Branch: ss.Branch})
	}
	for _, c := range s.Claims {
		st.Claims = append(st.Claims, *c)
	}
	for k, v := range s.LastPoll {
		st.LastPoll[k] = v.UTC().Format(time.RFC3339)
	}
	if list, err := ListOutbox(repoRoot); err == nil {
		st.Outbox = len(list)
	}
	return st, nil
}
