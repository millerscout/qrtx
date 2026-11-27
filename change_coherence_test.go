package qrtx

import "testing"

func TestChangesMapStaleAndEmptyMemory(t *testing.T) {
	root := t.TempDir()
	if err := NewChange(root, "bound"); err != nil {
		t.Fatal(err)
	}
	m := ChangesMap(root, "")
	if m["active_slug"] != "" {
		t.Fatalf("empty mem active_slug=%v", m["active_slug"])
	}
	m = ChangesMap(root, "missing")
	if m["active_slug"] != "" {
		t.Fatalf("stale active_slug=%v", m["active_slug"])
	}
	if m["stale_pin"] != "missing" {
		t.Fatalf("stale_pin=%v", m["stale_pin"])
	}
	m = ChangesMap(root, "bound")
	if m["active_slug"] != "bound" {
		t.Fatalf("bound active_slug=%v", m["active_slug"])
	}
	if _, ok := m["stale_pin"]; ok {
		t.Fatalf("unexpected stale_pin=%v", m["stale_pin"])
	}
}
