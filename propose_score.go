package sdd

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// sourceGraph is the CodeHit.Source for CodeIndex.Related hits.
const sourceGraph = "graph"

// CodeHit is one propose-context code lookup row.
type CodeHit struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Source string `json:"source"`
}

var tokenRE = regexp.MustCompile(`[A-Za-z0-9_./-]+`)

var letterWordRE = regexp.MustCompile(`[A-Za-z]{4,}`)

var confirmWordRE = regexp.MustCompile(`\bconfirm\b`)

// AcceptanceEcho reports a bullet whose every word of four or more letters is already in the request.
// A bullet with no such word is an echo.
func AcceptanceEcho(bullet, request string) bool {
	have := map[string]bool{}
	for _, word := range letterWordRE.FindAllString(strings.ToLower(request), -1) {
		have[word] = true
	}
	words := letterWordRE.FindAllString(strings.ToLower(bullet), -1)
	for _, word := range words {
		if !have[word] {
			return false
		}
	}
	return true
}

// manualRequestEcho reports a manual checkbox that only repeats the request.
// The manual: marker and the word confirm are ignored.
func manualRequestEcho(line, request string) bool {
	lower := strings.ToLower(line)
	idx := strings.Index(lower, "manual:")
	if idx < 0 {
		return false
	}
	rest := confirmWordRE.ReplaceAllString(lower[idx+len("manual:"):], " ")
	return AcceptanceEcho(rest, request)
}

// OutcomeBullets returns acceptance bullets that add a word the request did not use.
func OutcomeBullets(request, acceptance string) []string {
	var out []string
	for _, line := range strings.Split(acceptance, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "- ") || strings.Contains(trim, "(TBD") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
		if AcceptanceEcho(body, request) {
			continue
		}
		out = append(out, body)
	}
	return out
}

// OnlyEchoAcceptance reports acceptance that has bullets and every one repeats the request.
func OnlyEchoAcceptance(request, acceptance string) bool {
	echo := 0
	for _, line := range strings.Split(acceptance, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "- ") || strings.Contains(trim, "(TBD") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
		if !AcceptanceEcho(body, request) {
			return false
		}
		echo++
	}
	return echo > 0
}

// GraphHitCount counts Kùzu hits for a request that names a path or an identifier.
// A product request with neither stays at zero and does not query the graph.
func GraphHitCount(project, request string) int {
	if !hasPath(request) && !hasIdentifier(request) {
		return 0
	}
	n := 0
	for _, h := range queryRelatedHits(project, request, 3) {
		if h.Source == sourceGraph && h.Path != "" {
			n++
		}
	}
	return n
}

// queryRelatedHits, querySymbolHits and queryEmbeddingHits are the code
// lookups. Tests replace them.
var (
	queryRelatedHits   = indexRelatedHits
	querySymbolHits    = indexSymbolHits
	queryEmbeddingHits = indexEmbeddingHits
)

// indexRelatedHits is the graph lookup.
func indexRelatedHits(project, query string, limit int) []CodeHit {
	if strings.TrimSpace(query) == "" || limit <= 0 || codeIndex == nil {
		return nil
	}
	return withSource(codeIndex.Related(project, query, limit), sourceGraph)
}

// indexSymbolHits is the exact-name lookup.
func indexSymbolHits(project, name string, limit int) []CodeHit {
	if codeIndex == nil {
		return nil
	}
	return withSource(codeIndex.Symbols(project, name, limit), "symbol")
}

// indexEmbeddingHits is the embedding lookup. An empty table returns no rows and does not fail.
func indexEmbeddingHits(project, query string, limit int) []CodeHit {
	if codeIndex == nil {
		return nil
	}
	return withSource(codeIndex.Semantic(project, query, limit), "embedding")
}

// ProposeBand classifies a request as simple, context, or goldfish.
func ProposeBand(request string, bullets int, overlap bool) string {
	return proposeBand(request, bullets, overlap)
}

// AcceptanceBulletCount counts request lines that are already acceptance checks.
func AcceptanceBulletCount(section string) int {
	return acceptanceBulletCount(section)
}

// FirstRequestPath returns the first path token in the request.
func FirstRequestPath(request string) string {
	for _, tok := range requestTokens(request) {
		if isPathToken(tok) {
			return tok
		}
	}
	return ""
}

func proposeBand(request string, bullets int, overlap bool) string {
	pts := 0
	if !hasPath(request) && !hasIdentifier(request) {
		pts++
	}
	if bullets >= 2 {
		pts++
	}
	if overlap {
		pts++
	}
	switch {
	case pts >= 2:
		return "goldfish"
	case pts == 1:
		return "context"
	default:
		return "simple"
	}
}

func acceptanceBulletCount(section string) int {
	n := 0
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			n++
		}
	}
	return n
}

// groundedAcceptanceBullets counts an acceptance line only when its words are
// already in the user request. A TBD placeholder and a bullet the model invented
// do not add a point.
func groundedAcceptanceBullets(request, acceptance string) int {
	req := strings.ToLower(request)
	n := 0
	for _, line := range strings.Split(acceptance, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if strings.Contains(body, "TBD") || !bulletWordsInRequest(req, body) {
			continue
		}
		n++
	}
	return n
}

func bulletWordsInRequest(request, body string) bool {
	words := contentWords(body)
	if len(words) == 0 {
		return false
	}
	for _, word := range words {
		if !strings.Contains(request, word) {
			return false
		}
	}
	return true
}

func contentWords(text string) []string {
	var out []string
	for _, word := range requestTokens(strings.ToLower(text)) {
		if len(word) < 4 || stopWord[word] {
			continue
		}
		out = append(out, word)
	}
	return out
}

var stopWord = map[string]bool{
	"this": true, "that": true, "with": true, "from": true, "have": true,
	"will": true, "they": true, "them": true, "your": true, "into": true,
	"over": true, "only": true, "when": true, "does": true, "game": true,
}

func requestTokens(s string) []string {
	return tokenRE.FindAllString(s, -1)
}

func isPathToken(tok string) bool {
	if strings.Contains(tok, "/") {
		return true
	}
	lower := strings.ToLower(tok)
	return strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".md")
}

func isDottedName(tok string) bool {
	i := strings.Index(tok, ".")
	if i <= 0 || i >= len(tok)-1 {
		return false
	}
	return unicode.IsLetter(rune(tok[0])) && unicode.IsLetter(rune(tok[i+1]))
}

// identToken reports a code identifier. A single leading capital on the first
// token of the request is a sentence start, not an identifier. The same shape
// later in the request, as with Goldfish, is an identifier.
func identToken(tok string, first bool) bool {
	if tok == "" || isPathToken(tok) {
		return false
	}
	if strings.Contains(tok, "_") || isDottedName(tok) {
		return true
	}
	ups, internal := 0, false
	for i, r := range tok {
		if unicode.IsUpper(r) {
			ups++
			if i > 0 {
				internal = true
			}
		}
	}
	if internal || ups > 1 {
		return true
	}
	return ups == 1 && !first
}

func hasPath(request string) bool {
	for _, tok := range requestTokens(request) {
		if isPathToken(tok) {
			return true
		}
	}
	return false
}

func hasIdentifier(request string) bool {
	toks := requestTokens(request)
	for i, tok := range toks {
		if identToken(tok, i == 0) {
			return true
		}
	}
	return false
}

// queryCodeToken is a name worth searching. A single leading capital, as in
// Score or Goldfish, and an all-caps word, as in SDD, stay sentence words.
// CamelCase, underscores, dotted names, and paths are kept.
func queryCodeToken(tok string) bool {
	if tok == "" {
		return false
	}
	if isPathToken(tok) || strings.Contains(tok, "_") || isDottedName(tok) {
		return true
	}
	hasLower, internal := false, false
	for i, r := range tok {
		if unicode.IsLower(r) {
			hasLower = true
		}
		if unicode.IsUpper(r) && i > 0 {
			internal = true
		}
	}
	return internal && hasLower
}

func queryTokens(request string) []string {
	var out []string
	seen := map[string]bool{}
	for _, tok := range requestTokens(request) {
		if !queryCodeToken(tok) || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

func firstPath(request string) string {
	for _, tok := range requestTokens(request) {
		if isPathToken(tok) {
			return strings.TrimPrefix(tok, "./")
		}
	}
	return ""
}

func proposeDomain(request string, hitPaths []string, observed string) string {
	p := firstPath(request)
	if p == "" {
		p = firstPath(observed)
	}
	if p == "" {
		for _, h := range hitPaths {
			if path := firstPath(h); path != "" {
				p = path
				break
			}
		}
	}
	if p == "" {
		return "(none)"
	}
	dir := filepath.ToSlash(filepath.Dir(p))
	if dir == "." || dir == "" {
		return "(none)"
	}
	return dir
}

func inDomain(domain, file string) bool {
	domain = filepath.ToSlash(strings.TrimSpace(domain))
	file = filepath.ToSlash(strings.TrimSpace(file))
	if domain == "" || domain == "(none)" || file == "" {
		return false
	}
	return file == domain || strings.HasPrefix(file, domain+"/")
}

func collectCodeHits(band, project, request string) []CodeHit {
	if band == "simple" {
		return nil
	}
	tokens := queryTokens(request)
	if len(tokens) == 0 {
		return nil
	}
	var hits []CodeHit
	seen := map[string]bool{}
	add := func(rows []CodeHit) {
		for _, h := range rows {
			if len(hits) >= 3 || h.Path == "" || seen[h.Path] {
				continue
			}
			seen[h.Path] = true
			hits = append(hits, h)
		}
	}
	for _, tok := range tokens {
		if isPathToken(tok) || len(hits) >= 3 {
			continue
		}
		add(querySymbolHits(project, tok, 3-len(hits)))
	}
	q := strings.Join(tokens, " ")
	if len(hits) < 3 {
		add(queryRelatedHits(project, q, 3-len(hits)))
	}
	if len(hits) < 3 {
		add(queryEmbeddingHits(project, q, 3-len(hits)))
	}
	return hits
}
