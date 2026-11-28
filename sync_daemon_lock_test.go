package qrtx

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A status probe holds the guard while it takes daemon.lock; a daemon starting
// right then must wait for it, not leave.
func TestRunDaemonOutlastsLockProbe(t *testing.T) {
	root := t.TempDir()
	dir, err := ensureTrackerStateDir(root)
	if err != nil {
		t.Fatal(err)
	}
	release, err := lockFile(filepath.Join(dir, daemonGuard))
	if err != nil {
		t.Fatal(err)
	}
	unlock, ok, err := tryLockFile(filepath.Join(dir, daemonLockName))
	if err != nil || !ok {
		t.Fatalf("probe lock: %v %v", ok, err)
	}
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(100 * time.Millisecond)
		unlock()
		release()
	}()
	var log bytes.Buffer
	err = RunDaemon(root, &log)
	<-released
	if err != nil || !strings.Contains(log.String(), "started (") {
		t.Fatalf("daemon left on a brief lock: %v\n%s", err, log.String())
	}
}
