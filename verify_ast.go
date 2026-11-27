package qrtx

import (
	"fmt"
	"io"
)

// verifyASTStub is the P0 wiring check; P1 replaces with real drift verification.
func verifyASTStub(w io.Writer) error {
	_, err := fmt.Fprintln(w, "qrtx verify-ast: stub OK (full verify in P1)")
	return err
}
