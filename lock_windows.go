//go:build windows

package sdd

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// lockFile creates path exclusively, retrying for 10s. A lock file older
// than 60s is taken over: every operation under it is one file rewrite.
func lockFile(path string) (func(), error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, serr := os.Stat(path); serr == nil && time.Since(info.ModTime()) > 60*time.Second {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("lock %s is held by another sdd process", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
