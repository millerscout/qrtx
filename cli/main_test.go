package cli

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// In-process Run calls never start the sync daemon: os.Executable is the
	// test binary. Tests that need a daemon run the built qrtx binary and
	// clear this variable in its environment.
	os.Setenv("QRTX_NO_DAEMON", "1")
	os.Exit(m.Run())
}
