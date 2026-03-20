package main

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// normalize lowercases and strips diacritics (e.g. ö→o, é→e).
func normalize(s string) string {
	// NFD decomposes characters, then we strip combining marks.
	t := transform.Chain(norm.NFD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r) // Mn = nonspacing mark (accents, umlauts)
	}), norm.NFC)
	result, _, _ := transform.String(t, strings.ToLower(s))
	return result
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Use a single row of the matrix to save memory.
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr := make([]int, len(b)+1)
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev = curr
	}
	return prev[len(b)]
}

type fuzzyCandidate struct {
	name     string
	distance int
}

// fuzzyMatchError represents a failed lookup with suggestions.
type fuzzyMatchError struct {
	query      string
	candidates []fuzzyCandidate
}

func (e *fuzzyMatchError) Error() string {
	if len(e.candidates) == 0 {
		return fmt.Sprintf("contact %q not found", e.query)
	}
	names := make([]string, len(e.candidates))
	for i, c := range e.candidates {
		names[i] = c.name
	}
	return fmt.Sprintf("contact %q not found. Did you mean: %s?", e.query, strings.Join(names, ", "))
}

const fuzzyMaxDistance = 3

// normalizedTokensEqual returns true if two names contain the same tokens
// after normalization (lowercasing + stripping diacritics), regardless of order.
// e.g. "Alice Müller" matches "Mueller Alice".
func normalizedTokensEqual(a, b string) bool {
	aToks := strings.Fields(normalize(a))
	bToks := strings.Fields(normalize(b))
	if len(aToks) != len(bToks) || len(aToks) == 0 {
		return normalize(a) == normalize(b)
	}
	counts := make(map[string]int)
	for _, t := range aToks {
		counts[t]++
	}
	for _, t := range bToks {
		counts[t]--
		if counts[t] < 0 {
			return false
		}
	}
	for _, v := range counts {
		if v != 0 {
			return false
		}
	}
	return true
}

// tokenDistance computes a fuzzy distance between two names by comparing
// individual tokens (words). This handles reordered names (e.g. "Mueller Alice"
// vs "Alice Müller") and per-token diacritics gracefully.
// Returns the sum of best per-token edit distances, using normalized forms.
func tokenDistance(a, b string) int {
	aToks := strings.Fields(normalize(a))
	bToks := strings.Fields(normalize(b))

	if len(aToks) == 0 || len(bToks) == 0 {
		return levenshtein(normalize(a), normalize(b))
	}

	// For each token in a, find the best matching token in b.
	// Use the longer list as the outer loop to handle mismatched token counts.
	outer, inner := aToks, bToks
	if len(inner) > len(outer) {
		outer, inner = inner, outer
	}

	total := 0
	used := make([]bool, len(inner))
	for _, ot := range outer {
		bestDist := len(ot) + 10 // worst case
		bestIdx := -1
		for j, it := range inner {
			if used[j] {
				continue
			}
			d := levenshtein(ot, it)
			if d < bestDist {
				bestDist = d
				bestIdx = j
			}
		}
		if bestIdx >= 0 {
			used[bestIdx] = true
		}
		total += bestDist
	}
	return total
}

// fuzzyFind searches a list of contact names for approximate matches.
// It normalizes unicode (strips diacritics), handles reordered names via
// token-level matching, and tries substring matching before edit distance.
// Returns candidates sorted by relevance.
func fuzzyFind(query string, names []string) []fuzzyCandidate {
	queryNorm := normalize(query)

	// Try substring matches on normalized forms.
	var substringMatches []fuzzyCandidate
	for _, name := range names {
		if name == "" {
			continue
		}
		nameNorm := normalize(name)
		if strings.Contains(nameNorm, queryNorm) || strings.Contains(queryNorm, nameNorm) {
			substringMatches = append(substringMatches, fuzzyCandidate{name: name, distance: 0})
		}
	}
	if len(substringMatches) > 0 {
		return substringMatches
	}

	// Fall back to token-level edit distance.
	var candidates []fuzzyCandidate
	for _, name := range names {
		if name == "" {
			continue
		}
		dist := tokenDistance(query, name)
		if dist <= fuzzyMaxDistance {
			candidates = append(candidates, fuzzyCandidate{name: name, distance: dist})
		}
	}

	// Sort by distance (simple insertion sort, list is small).
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].distance < candidates[j-1].distance; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	return candidates
}
