package sdd

import "testing"

// The bundled example must stay a valid manifest as the format evolves.
func TestExampleChangeIsValid(t *testing.T) {
	if err := ValidateChangeManifest("examples/tic-tac-toe", "tic-tac-toe"); err != nil {
		t.Fatal(err)
	}
}
