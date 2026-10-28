package postgres

import (
	"strings"
	"unicode"
)

// Rank keyword overlap so natural-language tool queries do not require one
// contiguous phrase. Exact phrases retain priority over partial matches.
func repositorySearchScore(text, query string) float64 {
	text = normalizeRepositorySearch(text)
	query = normalizeRepositorySearch(strings.TrimSpace(query))
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

func normalizeRepositorySearch(text string) string {
	return strings.Map(func(r rune) rune {
		const accents = "áäčďéíĺľňóôŕšťúýž"
		const plain = "aacdeillnoorstuyz"
		for i, a := range []rune(accents) {
			if r == a {
				return []rune(plain)[i]
			}
		}
		return r
	}, strings.ToLower(text))
}
