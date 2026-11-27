package sdd

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Sync coordinates the agent sessions working in one repository: leases
// that heartbeats extend, advisory claims on issues and tasks, and an event
// log every session reads through its inbox. Everything lives in the shared
// tracker state directory (see trackerStateDir), so all worktrees and
// subprojects of one git repository see the same state. Unstable until v1.

const (
	syncStateFile  = "state.json"
	syncEventsFile = "events.jsonl"
	syncLockName   = "sync"
	maxSyncEvents  = 5000
	syncEventAge   = 7 * 24 * time.Hour
)

// Event kinds.
const (
	EventClaim        = "claim"
	EventRelease      = "release"
	EventClaimExpired = "claim_expired"
	EventSessionEnd   = "session_end"
	EventTaskProgress = "task_progress"
	EventRemoteChange = "remote_change"
)

// SyncEvent is one line of events.jsonl.
type SyncEvent struct {
	Seq      int64     `json:"seq"`
	At       time.Time `json:"at"`
	Session  string    `json:"session,omitempty"`
	Kind     string    `json:"kind"`
	Root     string    `json:"root,omitempty"`
	Key      string    `json:"key,omitempty"`
	Change   string    `json:"change,omitempty"`
	Task     string    `json:"task,omitempty"`
	Done     *bool     `json:"done,omitempty"`
	Worktree string    `json:"worktree,omitempty"`
	Branch   string    `json:"branch,omitempty"`
	Detail   string    `json:"detail,omitempty"`
}

type syncSession struct {
	Root       string    `json:"root"`
	Worktree   string    `json:"worktree,omitempty"`
	Branch     string    `json:"branch,omitempty"`
	LeaseUntil time.Time `json:"lease_until"`
	LastSeen   time.Time `json:"last_seen"`
}

type syncClaim struct {
	Session  string    `json:"session"`
	Root     string    `json:"root"`
	Key      string    `json:"key"`
	Worktree string    `json:"worktree,omitempty"`
	Branch   string    `json:"branch,omitempty"`
	At       time.Time `json:"at"`
}

type watchEntry struct {
	Root   string          `json:"root"`
	Mtime  int64           `json:"mtime"`
	Change string          `json:"change"`
	Tasks  map[string]bool `json:"tasks"`
}

type syncState struct {
	Sessions map[string]*syncSession `json:"sessions"`
	// Claims are keyed by syncScope(root) + "|" + claim key.
	Claims  map[string]*syncClaim  `json:"claims"`
	Cursors map[string]int64       `json:"cursors"`
	Remote  map[string]string      `json:"remote"`
	Watch   map[string]*watchEntry `json:"watch"`
	// LastPoll is keyed by syncScope(root).
	LastPoll map[string]time.Time `json:"last_poll"`
}

func newSyncState() *syncState {
	return &syncState{Sessions: map[string]*syncSession{}, Claims: map[string]*syncClaim{}, Cursors: map[string]int64{},
		Remote: map[string]string{}, Watch: map[string]*watchEntry{}, LastPoll: map[string]time.Time{}}
}

func (s *syncState) fill() {
	n := newSyncState()
	if s.Sessions == nil {
		s.Sessions = n.Sessions
	}
	if s.Claims == nil {
		s.Claims = n.Claims
	}
	if s.Cursors == nil {
		s.Cursors = n.Cursors
	}
	if s.Remote == nil {
		s.Remote = n.Remote
	}
	if s.Watch == nil {
		s.Watch = n.Watch
	}
	if s.LastPoll == nil {
		s.LastPoll = n.LastPoll
	}
}

// syncEnabled reports whether sync has ever run here (state.json exists).
func syncEnabled(repoRoot string) bool {
	dir, _ := trackerStateDir(repoRoot)
	_, err := os.Stat(filepath.Join(dir, syncStateFile))
	return err == nil
}

// syncTx is what a withSync callback works with.
type syncTx struct {
	root   string
	dir    string
	now    time.Time
	state  *syncState
	events []SyncEvent
	// lastSeq is the log's last sequence number before this transaction.
	lastSeq int64
}

func (tx *syncTx) emit(e SyncEvent) {
	if e.At.IsZero() {
		e.At = tx.now
	}
	tx.events = append(tx.events, e)
}

// expire drops sessions whose lease ended and releases their claims.
func (tx *syncTx) expire() {
	for id, s := range tx.state.Sessions {
		if s.LeaseUntil.After(tx.now) {
			continue
		}
		delete(tx.state.Sessions, id)
		tx.releaseClaims(id, EventClaimExpired)
	}
}

func (tx *syncTx) releaseClaims(session, kind string) {
	keys := make([]string, 0)
	for k, c := range tx.state.Claims {
		if c.Session == session {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		c := tx.state.Claims[k]
		delete(tx.state.Claims, k)
		tx.emit(SyncEvent{Session: session, Kind: kind, Root: c.Root, Key: c.Key, Worktree: c.Worktree, Branch: c.Branch})
	}
}

func (tx *syncTx) live(session string) bool {
	s, ok := tx.state.Sessions[session]
	return ok && s.LeaseUntil.After(tx.now)
}

// syncLockWait bounds how long hook commands wait for the sync lock.
const syncLockWait = time.Second

// lockWithin takes the named state lock, waiting at most wait (0 = no limit).
func lockWithin(path string, wait time.Duration) (func(), error) {
	if wait <= 0 {
		return lockFile(path)
	}
	deadline := time.Now().Add(wait)
	for {
		unlock, ok, err := tryLockFile(path)
		if err != nil {
			return nil, err
		}
		if ok {
			return unlock, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("sync state is busy (lock %s held for over %s)", path, wait)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// withSync runs fn on the sync state under the sync lock: it loads
// state.json, expires dead leases, lets fn change the state, then writes the
// state and appends emitted events. create=false returns errSyncDisabled,
// without creating anything, when state.json does not exist.
func withSync(repoRoot string, now time.Time, wait time.Duration, create bool, fn func(tx *syncTx) error) error {
	if !create && !syncEnabled(repoRoot) {
		return errSyncDisabled
	}
	dir, err := ensureTrackerStateDir(repoRoot)
	if err != nil {
		return err
	}
	unlock, err := lockWithin(filepath.Join(dir, syncLockName+".lock"), wait)
	if err != nil {
		return err
	}
	defer unlock()
	return syncTxLocked(repoRoot, dir, now, fn)
}

func syncTxLocked(repoRoot, dir string, now time.Time, fn func(tx *syncTx) error) error {
	st := newSyncState()
	raw, err := os.ReadFile(filepath.Join(dir, syncStateFile))
	if err == nil {
		if err := json.Unmarshal(raw, st); err != nil {
			return fmt.Errorf("%s: %v", filepath.Join(dir, syncStateFile), err)
		}
		st.fill()
	} else if !os.IsNotExist(err) {
		return err
	}
	last, err := lastEventSeq(dir)
	if err != nil {
		return err
	}
	tx := &syncTx{root: repoRoot, dir: dir, now: now, state: st, lastSeq: last}
	tx.expire()
	if err := fn(tx); err != nil {
		return err
	}
	if err := appendEventsLocked(dir, last, tx.events); err != nil {
		return err
	}
	out, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, syncStateFile)
	if bytes.Equal(out, raw) {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var errSyncDisabled = errors.New("sync not enabled")

func readEvents(dir string) ([]SyncEvent, error) {
	raw, err := os.ReadFile(filepath.Join(dir, syncEventsFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []SyncEvent
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e SyncEvent
		if json.Unmarshal(line, &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

func lastEventSeq(dir string) (int64, error) {
	evs, err := readEvents(dir)
	if err != nil || len(evs) == 0 {
		return 0, err
	}
	return evs[len(evs)-1].Seq, nil
}

func appendEventsLocked(dir string, last int64, events []SyncEvent) error {
	if len(events) == 0 {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, syncEventsFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	var buf bytes.Buffer
	for _, e := range events {
		last++
		e.Seq = last
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	_, err = f.Write(buf.Bytes())
	return err
}

// trimEventsLocked keeps the newest maxSyncEvents events younger than
// syncEventAge. It rewrites the log atomically; the caller holds the lock.
func trimEventsLocked(dir string, now time.Time) error {
	evs, err := readEvents(dir)
	if err != nil || len(evs) == 0 {
		return err
	}
	keep := evs
	if len(keep) > maxSyncEvents {
		keep = keep[len(keep)-maxSyncEvents:]
	}
	for len(keep) > 1 && now.Sub(keep[0].At) > syncEventAge {
		keep = keep[1:]
	}
	if len(keep) == len(evs) {
		return nil
	}
	var buf bytes.Buffer
	for _, e := range keep {
		b, _ := json.Marshal(e)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	path := filepath.Join(dir, syncEventsFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SyncSettings are the `sync:` keys of .sdd.yaml with defaults applied.
type SyncSettings struct {
	Lease    time.Duration
	Tick     time.Duration
	Interval time.Duration
}

func loadSyncSettings(repoRoot string) SyncSettings {
	s := SyncSettings{Lease: time.Hour, Tick: 5 * time.Second, Interval: 2 * time.Minute}
	cfg, err := LoadRepoConfig(repoRoot)
	if err != nil {
		return s
	}
	set := func(dst *time.Duration, v string) {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
			*dst = d
		}
	}
	set(&s.Lease, cfg.Sync.Lease)
	set(&s.Tick, cfg.Sync.Tick)
	set(&s.Interval, cfg.Sync.Interval)
	return s
}

func (s SyncSettings) debounce() time.Duration {
	d := s.Lease / 4
	if d > time.Minute {
		d = time.Minute
	}
	return d
}

// SessionInfo describes the caller of a sync command.
type SessionInfo struct {
	ID       string
	Worktree string
	Branch   string
}

// ResolveSession picks the session id: explicit flag, then the hook's
// session_id, then $SDD_SESSION, then <worktree>:<parent pid>.
func ResolveSession(repoRoot, flag, hookSession string) SessionInfo {
	info := SessionInfo{Worktree: gitToplevel(repoRoot), Branch: gitBranch(repoRoot)}
	switch {
	case strings.TrimSpace(flag) != "":
		info.ID = strings.TrimSpace(flag)
	case strings.TrimSpace(hookSession) != "":
		info.ID = strings.TrimSpace(hookSession)
	case strings.TrimSpace(os.Getenv("SDD_SESSION")) != "":
		info.ID = strings.TrimSpace(os.Getenv("SDD_SESSION"))
	default:
		info.ID = fmt.Sprintf("%s:%d", info.Worktree, os.Getppid())
	}
	return info
}

// syncScope names the project a root belongs to inside the shared state:
// its path relative to its git worktree's top level ("." for a whole-repo
// project, "services/api" for a subproject). Every worktree of one project
// gets the same scope, so their claims and remote caches are shared, while
// monorepo subprojects stay apart. Outside git it is the root's own path.
func syncScope(repoRoot string) string {
	top := canonicalRoot(gitToplevel(repoRoot))
	rel, err := filepath.Rel(top, canonicalRoot(repoRoot))
	if err != nil || strings.HasPrefix(rel, "..") {
		return "path:" + canonicalRoot(repoRoot)
	}
	return "repo:" + filepath.ToSlash(rel)
}

func gitToplevel(repoRoot string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = repoRoot
	if out, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return canonicalRoot(repoRoot)
}

func markerPath(dir, session string) string {
	sum := sha256.Sum256([]byte(session))
	return filepath.Join(dir, "hb-"+hex.EncodeToString(sum[:])[:16])
}

// touchSession records the caller's lease and, for a new session, starts its
// inbox cursor at the log's end.
func (tx *syncTx) touchSession(who SessionInfo, lease time.Duration) {
	s, ok := tx.state.Sessions[who.ID]
	if !ok {
		s = &syncSession{}
		tx.state.Sessions[who.ID] = s
	}
	s.Root = canonicalRoot(tx.root)
	s.Worktree, s.Branch = who.Worktree, who.Branch
	s.LeaseUntil = tx.now.Add(lease)
	s.LastSeen = tx.now
	if _, ok := tx.state.Cursors[who.ID]; !ok {
		tx.state.Cursors[who.ID] = tx.lastSeq
	}
}

// HeartbeatResult says what a heartbeat did.
type HeartbeatResult struct {
	Debounced     bool
	LeaseUntil    time.Time
	DaemonStarted bool
}

// Heartbeat extends who's lease. A heartbeat within the debounce window of
// the previous one only checks a marker file's mtime. It may start the sync
// daemon (see ensureDaemon).
func Heartbeat(repoRoot string, who SessionInfo, now time.Time, allowDaemon bool) (HeartbeatResult, error) {
	set := loadSyncSettings(repoRoot)
	dir, _ := trackerStateDir(repoRoot)
	marker := markerPath(dir, who.ID)
	if info, err := os.Stat(marker); err == nil && now.Sub(info.ModTime()) < set.debounce() && now.Sub(info.ModTime()) >= 0 {
		return HeartbeatResult{Debounced: true}, nil
	}
	var res HeartbeatResult
	err := withSync(repoRoot, now, syncLockWait, true, func(tx *syncTx) error {
		tx.touchSession(who, set.Lease)
		res.LeaseUntil = tx.state.Sessions[who.ID].LeaseUntil
		if allowDaemon {
			started, err := ensureDaemonLocked(repoRoot, tx.dir)
			res.DaemonStarted = started
			return err
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	_ = os.WriteFile(marker, nil, 0o644)
	_ = os.Chtimes(marker, now, now)
	return res, nil
}

// BeginSession registers who (a SessionStart hook) and appends
// `export SDD_SESSION=<id>` to envFile when it is set.
func BeginSession(repoRoot string, who SessionInfo, now time.Time, envFile string, allowDaemon bool) error {
	dir, _ := trackerStateDir(repoRoot)
	_ = os.Remove(markerPath(dir, who.ID))
	if _, err := Heartbeat(repoRoot, who, now, allowDaemon); err != nil {
		return err
	}
	if envFile == "" {
		return nil
	}
	f, err := os.OpenFile(envFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "export SDD_SESSION=%s\n", shellQuote(who.ID))
	return err
}

func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.:/") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// EndSession drops who's lease and releases its claims. It does nothing
// where sync never ran.
func EndSession(repoRoot string, who SessionInfo, now time.Time) error {
	dir, _ := trackerStateDir(repoRoot)
	err := withSync(repoRoot, now, syncLockWait, false, func(tx *syncTx) error {
		if _, ok := tx.state.Sessions[who.ID]; !ok {
			return nil
		}
		delete(tx.state.Sessions, who.ID)
		tx.releaseClaims(who.ID, EventRelease)
		tx.emit(SyncEvent{Session: who.ID, Kind: EventSessionEnd, Worktree: who.Worktree, Branch: who.Branch})
		return nil
	})
	if errors.Is(err, errSyncDisabled) {
		return nil
	}
	_ = os.Remove(markerPath(dir, who.ID))
	return err
}

// ClaimHeldError says another live session holds a claim.
type ClaimHeldError struct {
	Key      string
	Session  string
	Worktree string
	Branch   string
}

func (e *ClaimHeldError) Error() string {
	return fmt.Sprintf("%s is claimed by session %s (worktree %s, branch %s)", e.Key, e.Session, e.Worktree, e.Branch)
}

// ErrSyncNotEnabled is returned by Claim where sync never ran.
var ErrSyncNotEnabled = errSyncDisabled

// Claim takes (or with release drops) key for who. A claim also extends
// who's lease. It returns ErrSyncNotEnabled, creating nothing, where sync
// never ran, and *ClaimHeldError when another live session holds the key.
func Claim(repoRoot string, who SessionInfo, key string, release bool, now time.Time, allowDaemon bool) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("claim: pass an issue ref or <slug>/<task id>")
	}
	set := loadSyncSettings(repoRoot)
	full := syncScope(repoRoot) + "|" + key
	return withSync(repoRoot, now, 0, false, func(tx *syncTx) error {
		tx.touchSession(who, set.Lease)
		held, ok := tx.state.Claims[full]
		if release {
			if ok && held.Session == who.ID {
				delete(tx.state.Claims, full)
				tx.emit(SyncEvent{Session: who.ID, Kind: EventRelease, Root: canonicalRoot(repoRoot), Key: key, Worktree: who.Worktree, Branch: who.Branch})
			}
			return nil
		}
		if ok && held.Session != who.ID && tx.live(held.Session) {
			return &ClaimHeldError{Key: key, Session: held.Session, Worktree: held.Worktree, Branch: held.Branch}
		}
		if !ok || held.Session != who.ID {
			tx.state.Claims[full] = &syncClaim{Session: who.ID, Root: canonicalRoot(repoRoot), Key: key, Worktree: who.Worktree, Branch: who.Branch, At: tx.now}
			tx.emit(SyncEvent{Session: who.ID, Kind: EventClaim, Root: canonicalRoot(repoRoot), Key: key, Worktree: who.Worktree, Branch: who.Branch})
		}
		if allowDaemon {
			_, err := ensureDaemonLocked(repoRoot, tx.dir)
			return err
		}
		return nil
	})
}

// Inbox returns the events other sessions (or the daemon) caused since
// who's cursor, and advances it. A session with no cursor starts at the end.
func Inbox(repoRoot string, who SessionInfo, now time.Time, wait time.Duration) ([]SyncEvent, error) {
	var out []SyncEvent
	err := withSync(repoRoot, now, wait, false, func(tx *syncTx) error {
		cur, ok := tx.state.Cursors[who.ID]
		if !ok {
			tx.state.Cursors[who.ID] = tx.lastSeq
			return nil
		}
		evs, err := readEvents(tx.dir)
		if err != nil {
			return err
		}
		evs = append(evs, numbered(tx.lastSeq, tx.events)...)
		last := cur
		for _, e := range evs {
			if e.Seq <= cur {
				continue
			}
			last = e.Seq
			if e.Session != who.ID {
				out = append(out, e)
			}
		}
		tx.state.Cursors[who.ID] = last
		return nil
	})
	if errors.Is(err, errSyncDisabled) {
		return nil, nil
	}
	return out, err
}

// numbered gives not-yet-written events the seqs they will get.
func numbered(last int64, evs []SyncEvent) []SyncEvent {
	out := make([]SyncEvent, len(evs))
	for i, e := range evs {
		last++
		e.Seq = last
		out[i] = e
	}
	return out
}

// EventsAfter returns the events with seq > after, for `sync watch`.
func EventsAfter(repoRoot string, after int64) ([]SyncEvent, error) {
	dir, _ := trackerStateDir(repoRoot)
	evs, err := readEvents(dir)
	if err != nil {
		return nil, err
	}
	var out []SyncEvent
	for _, e := range evs {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out, nil
}

// Summary is a one-line description of an event.
func (e SyncEvent) Summary() string {
	who := e.Session
	if len(who) > 12 {
		who = who[:12]
	}
	where := ""
	if e.Worktree != "" {
		where = " in " + filepath.Base(e.Worktree)
		if e.Branch != "" {
			where += " (" + e.Branch + ")"
		}
	}
	switch e.Kind {
	case EventClaim:
		return fmt.Sprintf("%s claimed %s%s", who, e.Key, where)
	case EventRelease:
		return fmt.Sprintf("%s released %s", who, e.Key)
	case EventClaimExpired:
		return fmt.Sprintf("claim_expired %s (%s)", e.Key, who)
	case EventSessionEnd:
		return fmt.Sprintf("session_end %s%s", who, where)
	case EventTaskProgress:
		state := "unchecked"
		if e.Done != nil && *e.Done {
			state = "done"
		}
		return fmt.Sprintf("%s/%s %s%s", e.Change, e.Task, state, where)
	case EventRemoteChange:
		return fmt.Sprintf("remote_change %s: %s", e.Key, e.Detail)
	}
	return e.Kind + " " + e.Key
}
