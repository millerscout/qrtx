//go:build windows

package sdd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Windows has no flock in the standard library. A lock is a file created
// exclusively that holds "<pid> <nonce>". A holder refreshes or removes only
// a file that still carries its own nonce, and a file older than 60s is
// taken over: every short operation under it is one file rewrite, and
// long-lived holders (the sync daemon) refresh it every 30s.

var lockNonces = map[string]string{}

func lockOwner() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%d %s", os.Getpid(), hex.EncodeToString(b))
}

func ownsLock(path, owner string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(b)) == owner
}

func tryCreateLock(path string) (owner string, ok bool, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err == nil {
		owner = lockOwner()
		_, _ = f.WriteString(owner)
		_ = f.Close()
		return owner, true, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return "", false, err
	}
	if info, serr := os.Stat(path); serr == nil && time.Since(info.ModTime()) > 60*time.Second {
		_ = os.Remove(path)
		return tryCreateLock(path)
	}
	return "", false, nil
}

func releaser(path, owner string) func() {
	return func() {
		if ownsLock(path, owner) {
			_ = os.Remove(path)
		}
	}
}

// lockFile creates path exclusively, retrying for 10s.
func lockFile(path string) (func(), error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		owner, ok, err := tryCreateLock(path)
		if err != nil {
			return nil, err
		}
		if ok {
			return releaser(path, owner), nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("lock %s is held by another sdd process", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// tryLockFile makes one attempt to take path.
func tryLockFile(path string) (func(), bool, error) {
	owner, ok, err := tryCreateLock(path)
	if err != nil || !ok {
		return nil, false, err
	}
	lockNonces[path] = owner
	return releaser(path, owner), true, nil
}

// refreshLock touches a long-held lock this process owns, so the 60s stale
// rule never takes it over.
func refreshLock(path string) {
	if owner, ok := lockNonces[path]; ok && ownsLock(path, owner) {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}
}
