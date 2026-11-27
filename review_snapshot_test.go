package qrtx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReviewPacketSnapshot writes the packet of every saved bench plan
// (QRTX_PACKET_PLANS: a glob of plan.md files) to QRTX_PACKET_OUT, so two builds
// can be diffed. Each plan.md holds "## FILE: <slug>/<rel>" sections. Skipped
// unless both variables are set.
func TestReviewPacketSnapshot(t *testing.T) {
	glob, out := os.Getenv("QRTX_PACKET_PLANS"), os.Getenv("QRTX_PACKET_OUT")
	if glob == "" || out == "" {
		t.Skip("set QRTX_PACKET_PLANS and QRTX_PACKET_OUT")
	}
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	plans, _ := filepath.Glob(glob)
	if len(plans) == 0 {
		t.Fatalf("no plans match %s", glob)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, plan := range plans {
		raw, err := os.ReadFile(plan)
		if err != nil {
			t.Fatal(err)
		}
		change, phases, slug := splitSavedPlan(string(raw))
		p := buildReviewPacket(repoRoot, slug, change, phases)
		name := strings.NewReplacer("/", "_", " ", "_").Replace(strings.TrimPrefix(plan, os.Getenv("HOME")))
		if err := os.WriteFile(filepath.Join(out, name+".packet.md"), []byte(FormatReviewPacket(p)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("wrote %d packets to %s", len(plans), out)
}

// splitSavedPlan separates change.sdd.md from the phase files of a saved plan.
func splitSavedPlan(raw string) (change, phases, slug string) {
	var cur *strings.Builder
	var c, ph strings.Builder
	for _, line := range strings.SplitAfter(raw, "\n") {
		if rest, ok := strings.CutPrefix(line, "## FILE: "); ok {
			rest = strings.TrimSpace(rest)
			if strings.HasSuffix(rest, "/change.sdd.md") {
				slug = strings.TrimSuffix(rest, "/change.sdd.md")
				cur = &c
				continue
			}
			cur = &ph
			ph.WriteString(line)
			continue
		}
		if cur != nil {
			cur.WriteString(line)
		}
	}
	return c.String(), ph.String(), slug
}
