// Package kittest runs the sdd tracker conformance checks against a kit
// provider in-process, with writes on.
package kittest

import (
	"encoding/json"
	"testing"

	"github.com/millerscout/sdd/providers/kit"
	"github.com/millerscout/sdd/tracker"
)

// Run initialises p with config and fails t for every conformance op that
// does not pass.
func Run(t testing.TB, p kit.Provider, config json.RawMessage) []tracker.CheckResult {
	t.Helper()
	if err := p.Init(config); err != nil {
		t.Fatalf("Init: %v", err)
	}
	results := tracker.Conformance(kit.AsTracker(p), true)
	for _, r := range results {
		if !r.OK {
			t.Errorf("conformance %s: %s", r.Op, r.Detail)
		}
	}
	return results
}
