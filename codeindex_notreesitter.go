//go:build !cgo

package qrtx

// treeSitterEnabled reports whether non-Go definitions are indexed. Builds
// without cgo have no tree-sitter, so only Go definitions are found.
const treeSitterEnabled = false

// treeSitterDefs is a no-op without cgo.
func treeSitterDefs(srcs []sourceFile, out map[string][]codeDef, want map[string]bool) {}
