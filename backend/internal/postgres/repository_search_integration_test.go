package postgres

import (
	"context"
	"strings"
	"testing"

	"viki/internal/model"
)

func TestRepositorySearchFindsSeparatedKeywordsAndRetrievalIncludesStableIdentity(t *testing.T) {
	r, _, _, org := syncFixture(t)
	ctx := context.Background()
	snapshot := eligibleFixture(t)
	importOK(t, r, org, snapshot)
	for _, query := range []string{"customer paper", "paper customer", "VOZ-SC-CONSENT-001", "subscription withdrawn customer email group"} {
		results, err := r.SearchPages(ctx, org, model.SearchOptions{Query: query, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, result := range results {
			if result.Page.RepositorySource.ExternalID == "VOZ-SC-CONSENT-001" {
				found = true
			}
		}
		if !found {
			t.Fatalf("query %q omitted known paper-consent scenario: %+v", query, results)
		}
		documents, err := r.Retrieve(ctx, org, query, true, 10)
		if err != nil {
			t.Fatal(err)
		}
		found = false
		for _, document := range documents {
			if strings.Contains(document.Content, "VOZ-SC-CONSENT-001") && strings.Contains(document.Content, "source commit "+snapshot.Source.Commit) {
				found = true
			}
		}
		if !found {
			t.Fatalf("query %q did not expose stable source identity and version to Q&A: %+v", query, documents)
		}
	}
}

func TestRepositorySearchRanksBeforeLimitingAndRetrievalPreservesScore(t *testing.T) {
	r, _, _, org := syncFixture(t)
	ctx := context.Background()
	importOK(t, r, org, eligibleFixture(t))
	results, err := r.SearchPages(ctx, org, model.SearchOptions{Query: "VOZ-SC-CONSENT-001", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Page.RepositorySource.ExternalID != "VOZ-SC-CONSENT-001" || results[0].Score != 2 {
		t.Fatalf("ranked search: %+v", results)
	}
	documents, err := r.Retrieve(ctx, org, "VOZ-SC-CONSENT-001", true, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 || documents[0].RevisionID != results[0].RevisionID || documents[0].Score != results[0].Score {
		t.Fatalf("ranked retrieval: %+v", documents)
	}
}
