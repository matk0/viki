package postgres

import "testing"

func TestRepositorySearchRanksPhrasesAndKeywordOverlap(t *testing.T) {
	for _, tc := range []struct {
		name, text, query string
		score             float64
	}{
		{"browse", "Customer consent", "  ", 1},
		{"exact phrase case insensitive", "Customer PAPER consent", "paper consent", 2},
		{"separated words", "A customer has existing paper consent", "customer paper", 1},
		{"reversed words", "A customer has existing paper consent", "paper customer", 1},
		{"partial overlap", "A customer has existing paper consent", "withdrawn customer", 0.5},
		{"deduplicated words", "customer", "customer customer missing", 0.5},
		{"punctuation", "customer with paper consent", "customer, paper?", 1},
		{"unicode", "Zákazník má súhlas", "SÚHLAS zákazník", 1},
		{"no match", "customer consent", "withdrawn", 0},
		{"no words", "customer consent", "?!", 0},
		{"stable ID", "VOZ-SC-CONSENT-001", "VOZ-SC-CONSENT-001", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := repositorySearchScore(tc.text, tc.query); got != tc.score {
				t.Fatalf("score=%v want=%v", got, tc.score)
			}
		})
	}
}
