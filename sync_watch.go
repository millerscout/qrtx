package sdd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/millerscout/sdd/tasksmd"
)

func init() {
	daemonRootWork = append(daemonRootWork, watchRoot, pollRoot)
}

type worktree struct {
	Path   string
	Branch string
}

// worktreesOf lists the git worktrees of root's repository; outside git it
// is root alone.
func worktreesOf(root string) []worktree {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return []worktree{{Path: canonicalRoot(root)}}
	}
	var list []worktree
	var cur *worktree
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			list = append(list, worktree{Path: strings.TrimPrefix(line, "worktree ")})
			cur = &list[len(list)-1]
		case strings.HasPrefix(line, "branch ") && cur != nil:
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	return list
}

var watchTaskRE = regexp.MustCompile(`^([A-Z]?\d+(?:\.\d+)*[a-z]?)\b`)

func watchTaskID(text string) string {
	if m := watchTaskRE.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	sum := sha256.Sum256([]byte(text))
	return "h" + hex.EncodeToString(sum[:])[:8]
}

type scannedFile struct {
	path     string
	mtime    int64
	change   string
	worktree worktree
	tasks    map[string]bool
}

func readSyncStateRO(dir string) *syncState {
	st := newSyncState()
	if raw, err := os.ReadFile(filepath.Join(dir, syncStateFile)); err == nil {
		_ = json.Unmarshal(raw, st)
		st.fill()
	}
	return st
}

// watchRoot snapshots the task checkboxes of root's changes in every
// worktree and appends task_progress for each checkbox that changed since
// the stored snapshot. New files are snapshotted silently; vanished files
// are dropped silently.
func watchRoot(root string, logw io.Writer) {
	dir, _ := trackerStateDir(root)
	rel, err := filepath.Rel(gitToplevel(root), canonicalRoot(root))
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = "."
	}
	old := readSyncStateRO(dir)
	var scanned []scannedFile
	var prefixes []string
	for _, wt := range worktreesOf(root) {
		wroot := filepath.Join(wt.Path, rel)
		prefixes = append(prefixes, filepath.Join(wroot, "sdd", "changes")+string(filepath.Separator))
		var files []string
		for _, pat := range []string{"sdd/changes/*/" + changeFileName, "sdd/changes/*/phases/*.md"} {
			m, _ := filepath.Glob(filepath.Join(wroot, filepath.FromSlash(pat)))
			files = append(files, m...)
		}
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			relPath, _ := filepath.Rel(filepath.Join(wroot, "sdd", "changes"), f)
			change := strings.SplitN(filepath.ToSlash(relPath), "/", 2)[0]
			if change == archiveDirName {
				continue
			}
			sf := scannedFile{path: f, mtime: info.ModTime().UnixNano(), change: change, worktree: wt}
			if e, ok := old.Watch[f]; !ok || e.Mtime != sf.mtime {
				sf.tasks = map[string]bool{}
				for _, it := range tasksmd.ParseTasksMD(f).Items {
					sf.tasks[watchTaskID(it.Text)] = it.Done
				}
			}
			scanned = append(scanned, sf)
		}
	}
	err = withSync(root, time.Now(), 0, true, func(tx *syncTx) error {
		seen := map[string]bool{}
		for _, sf := range scanned {
			seen[sf.path] = true
			e, ok := tx.state.Watch[sf.path]
			if !ok {
				tx.state.Watch[sf.path] = &watchEntry{Root: canonicalRoot(root), Mtime: sf.mtime, Change: sf.change, Tasks: sf.tasks}
				continue
			}
			if e.Mtime == sf.mtime || sf.tasks == nil {
				continue
			}
			for id, done := range sf.tasks {
				was, had := e.Tasks[id]
				if had && was != done {
					d := done
					tx.emit(SyncEvent{Kind: EventTaskProgress, Root: canonicalRoot(root), Change: sf.change, Task: id, Done: &d, Worktree: sf.worktree.Path, Branch: sf.worktree.Branch})
				}
			}
			e.Mtime, e.Tasks, e.Change = sf.mtime, sf.tasks, sf.change
		}
		for path := range tx.state.Watch {
			if seen[path] {
				continue
			}
			for _, p := range prefixes {
				if strings.HasPrefix(path, p) {
					delete(tx.state.Watch, path)
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(logw, "watch %s: %v\n", root, err)
	}
}

// pollRoot lists root's remote tracker every sync.interval and appends
// remote_change for each issue whose updated_at changed. The first poll of
// a root only records what it saw. The list runs outside the sync lock.
func pollRoot(root string, logw io.Writer) {
	set := loadSyncSettings(root)
	dir, _ := trackerStateDir(root)
	key := syncScope(root)
	last := readSyncStateRO(dir).LastPoll[key]
	if !last.IsZero() && time.Since(last) < set.Interval {
		return
	}
	t, err := OpenTracker(root)
	if err != nil {
		fmt.Fprintf(logw, "poll %s: %v\n", root, err)
		return
	}
	if t.Backend == fileBackendName && hostTracker == nil {
		return
	}
	list, err := t.ListIssues(IssueFilter{Limit: 200})
	if err != nil {
		fmt.Fprintf(logw, "poll %s: %v\n", root, err)
		return
	}
	err = withSync(root, time.Now(), 0, true, func(tx *syncTx) error {
		first := tx.state.LastPoll[key].IsZero()
		for _, is := range list {
			ref := t.Ref("", is.ID)
			k := key + "|" + ref
			if !first && tx.state.Remote[k] != is.UpdatedAt {
				tx.emit(SyncEvent{Kind: EventRemoteChange, Root: canonicalRoot(root), Key: ref, Detail: is.Title + " [" + is.Status.Category + "]"})
			}
			tx.state.Remote[k] = is.UpdatedAt
		}
		tx.state.LastPoll[key] = tx.now
		return nil
	})
	if err != nil {
		fmt.Fprintf(logw, "poll %s: %v\n", root, err)
	}
}
