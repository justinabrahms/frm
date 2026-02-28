package main

import (
	"fmt"
	"strings"
)

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

const fuzzyMaxDistance = 2

// fuzzyFind searches a list of contact names for approximate matches.
// It tries substring matching first, then falls back to edit distance.
// Returns candidates sorted by relevance (substring matches first, then by distance).
func fuzzyFind(query string, names []string) []fuzzyCandidate {
	queryLower := strings.ToLower(query)

	// Try substring matches: name contains query, or query contains name.
	var substringMatches []fuzzyCandidate
	for _, name := range names {
		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, queryLower) || strings.Contains(queryLower, nameLower) {
			substringMatches = append(substringMatches, fuzzyCandidate{name: name, distance: 0})
		}
	}
	if len(substringMatches) > 0 {
		return substringMatches
	}

	// Fall back to edit distance.
	var candidates []fuzzyCandidate
	for _, name := range names {
		dist := levenshtein(strings.ToLower(name), queryLower)
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
