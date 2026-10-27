package postgres

import (
	"strings"
	"unicode"
)

// Rank keyword overlap so natural-language tool queries do not require one
// contiguous phrase. Exact phrases retain priority over partial matches.
func repositorySearchScore(text, query string) float64 {
	text = strings.ToLower(text)
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 1
	}
	tokens := strings.FieldsFunc(query, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	unique := map[string]bool{}
	matched := 0
	for _, token := range tokens {
		if unique[token] {
			continue
		}
		unique[token] = true
		if strings.Contains(text, token) {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	score := float64(matched) / float64(len(unique))
	if strings.Contains(text, query) {
		score++
	}
	return score
}
