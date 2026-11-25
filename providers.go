package sdd

// CodeIndex looks up code for a propose request. Hits carry Path and Line;
// sdd sets Source. Without one, propose context has no code hits.
type CodeIndex interface {
	// Related returns graph neighbours of the query text.
	Related(project, query string, limit int) []CodeHit
	// Symbols returns definitions of an exact identifier.
	Symbols(project, name string, limit int) []CodeHit
	// Semantic returns embedding-search hits for the query text.
	Semantic(project, query string, limit int) []CodeHit
}

// ContextProvider supplies project health for propose preflight. Without
// one, the report has no issues summary and no known fixes.
type ContextProvider interface {
	// OpenIssues counts open issues per rule. ok is false when unavailable.
	OpenIssues(project string) (byRule map[string]int, total int, ok bool)
	// KnownFixes returns recent fixes that passed, newest first.
	KnownFixes(project string, limit int) []KnownFix
}

var (
	codeIndex       CodeIndex
	contextProvider ContextProvider
)

// SetCodeIndex installs the code index. Call it once, before serving.
func SetCodeIndex(ci CodeIndex) { codeIndex = ci }

// SetContextProvider installs the context provider. Call it once, before serving.
func SetContextProvider(cp ContextProvider) { contextProvider = cp }

func withSource(hits []CodeHit, source string) []CodeHit {
	if len(hits) == 0 {
		return nil
	}
	out := make([]CodeHit, 0, len(hits))
	for _, h := range hits {
		h.Source = source
		out = append(out, h)
	}
	return out
}
