package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"viki/internal/governance"
	"viki/internal/model"
	"viki/internal/store"
)

type pagesRepository struct {
	store.Repository
	err           error
	searchOptions model.SearchOptions
	listedKind    *model.PageKind
	createdInput  model.CreatePageInput
	savedInput    model.SaveRevisionInput
	voteValue     governance.VoteValue
	voteReason    string
	commentBody   string
	auditLimit    int
}

func (r *pagesRepository) ListPages(_ context.Context, _ string, kind *model.PageKind) ([]model.Page, error) {
	r.listedKind = kind
	return []model.Page{{ID: "page-1", Title: "Zmluva"}}, r.err
}

func (r *pagesRepository) SearchPages(_ context.Context, _ string, options model.SearchOptions) ([]model.SearchResult, error) {
	r.searchOptions = options
	return []model.SearchResult{{Page: model.Page{ID: "page-1", Title: "Zmluva"}, RevisionID: "revision-1"}}, r.err
}

func (r *pagesRepository) CreatePage(_ context.Context, _, _ string, input model.CreatePageInput, status model.RevisionStatus) (model.PageDetail, error) {
	r.createdInput = input
	if status != model.RevisionDraft {
		return model.PageDetail{}, errors.New("unexpected status")
	}
	return model.PageDetail{Page: model.Page{ID: "page-1", Title: input.Content.Title}}, r.err
}

func (r *pagesRepository) PageDetail(context.Context, string, string) (model.PageDetail, error) {
	return model.PageDetail{Page: model.Page{ID: "page-1", Title: "Zmluva"}}, r.err
}

func (r *pagesRepository) Revision(context.Context, string, string) (model.Revision, error) {
	return model.Revision{ID: "revision-1", Title: "Zmluva"}, r.err
}

func (r *pagesRepository) SaveRevision(_ context.Context, _, _, _ string, input model.SaveRevisionInput) (model.Revision, error) {
	r.savedInput = input
	return model.Revision{ID: "revision-2", Title: input.Content.Title}, r.err
}

func (r *pagesRepository) PublishRevision(context.Context, string, string, string) (model.PageDetail, error) {
	return model.PageDetail{Page: model.Page{ID: "page-1", Accepted: true}}, r.err
}

func (r *pagesRepository) SetVote(_ context.Context, _, _, _ string, value governance.VoteValue, reason string) (model.Vote, error) {
	r.voteValue = value
	r.voteReason = reason
	return model.Vote{RevisionID: "revision-1", Value: string(value)}, r.err
}

func (r *pagesRepository) AddComment(_ context.Context, _, _, _, _ string, _, _, _ *string, body string, _ bool) (model.Comment, error) {
	r.commentBody = body
	return model.Comment{ID: "comment-1", Body: body}, r.err
}

func (r *pagesRepository) ResolveComment(context.Context, string, string, string) (model.Comment, error) {
	return model.Comment{ID: "comment-1", Body: "resolved"}, r.err
}

func (r *pagesRepository) ListAudit(_ context.Context, _ string, limit int) ([]model.AuditEvent, error) {
	r.auditLimit = limit
	return []model.AuditEvent{{ID: "event-1", Action: "page.created"}}, r.err
}

func pageAuth() authState {
	return authState{Session: model.Session{
		User:           model.User{ID: "user-1"},
		OrganizationID: "organization-1",
	}}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPageListingSupportsKindSearchDraftsAndRepositoryErrors(t *testing.T) {
	t.Parallel()

	server := &Server{repository: &pagesRepository{}, logger: discardLogger()}
	recorder := httptest.NewRecorder()
	server.listPages(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/pages?kind=concept", nil), pageAuth())
	repository := server.repository.(*pagesRepository)
	if recorder.Code != http.StatusOK || repository.listedKind == nil || *repository.listedKind != model.PageConcept || !strings.Contains(recorder.Body.String(), `"pages"`) {
		t.Fatalf("list status=%d kind=%v body=%s", recorder.Code, repository.listedKind, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	server.listPages(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/pages?q=zmluva&kind=feature&includeDrafts=true", nil), pageAuth())
	if recorder.Code != http.StatusOK || repository.searchOptions.Query != "zmluva" || repository.searchOptions.Kind == nil || *repository.searchOptions.Kind != model.PageFeature || !repository.searchOptions.IncludeDrafts || repository.searchOptions.Limit != 50 {
		t.Fatalf("search status=%d options=%+v body=%s", recorder.Code, repository.searchOptions, recorder.Body.String())
	}

	for _, requestPath := range []string{"/api/v1/pages", "/api/v1/pages?q=zmluva"} {
		repository.err = errors.New("query failed")
		recorder = httptest.NewRecorder()
		server.listPages(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil), pageAuth())
		if recorder.Code != http.StatusUnprocessableEntity {
			t.Fatalf("query error path=%s status=%d body=%s", requestPath, recorder.Code, recorder.Body.String())
		}
		repository.err = nil
	}
}

func TestPageCreationRevisionAndPublicationHandlers(t *testing.T) {
	t.Parallel()

	repository := &pagesRepository{}
	server := &Server{repository: repository, logger: discardLogger()}
	auth := pageAuth()

	recorder := httptest.NewRecorder()
	server.createPage(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/pages", strings.NewReader(`{"kind":"concept","slug":"zmluva","content":{"title":"Zmluva","bodyMd":""}}`)), auth)
	if recorder.Code != http.StatusCreated || repository.createdInput.Slug != "zmluva" {
		t.Fatalf("create status=%d input=%+v body=%s", recorder.Code, repository.createdInput, recorder.Body.String())
	}
	assertHandlerInvalidJSONAndRepositoryError(t, server.createPage, repository, http.MethodPost, "/api/v1/pages", auth, `{`)

	tests := []struct {
		name       string
		pathName   string
		pathValue  string
		method     string
		body       string
		wantStatus int
		invoke     func(*httptest.ResponseRecorder, *http.Request)
	}{
		{name: "page detail", pathName: "pageID", pathValue: "page-1", method: http.MethodGet, wantStatus: http.StatusOK, invoke: func(w *httptest.ResponseRecorder, r *http.Request) { server.pageDetail(w, r, auth) }},
		{name: "revision detail", pathName: "revisionID", pathValue: "revision-1", method: http.MethodGet, wantStatus: http.StatusOK, invoke: func(w *httptest.ResponseRecorder, r *http.Request) { server.revisionDetail(w, r, auth) }},
		{name: "save revision", pathName: "pageID", pathValue: "page-1", method: http.MethodPost, body: `{"baseRevisionId":"revision-1","content":{"title":"Zmluva 2","bodyMd":""}}`, wantStatus: http.StatusCreated, invoke: func(w *httptest.ResponseRecorder, r *http.Request) { server.saveRevision(w, r, auth) }},
		{name: "publish revision", pathName: "revisionID", pathValue: "revision-1", method: http.MethodPost, wantStatus: http.StatusOK, invoke: func(w *httptest.ResponseRecorder, r *http.Request) { server.publishRevision(w, r, auth) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/", strings.NewReader(test.body))
			request.SetPathValue(test.pathName, test.pathValue)
			recorder := httptest.NewRecorder()
			test.invoke(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}

			missing := httptest.NewRequest(test.method, "/", strings.NewReader(test.body))
			missingRecorder := httptest.NewRecorder()
			test.invoke(missingRecorder, missing)
			if missingRecorder.Code != http.StatusBadRequest {
				t.Fatalf("missing ID status=%d body=%s", missingRecorder.Code, missingRecorder.Body.String())
			}

			repository.err = errors.New("repository failed")
			failed := httptest.NewRequest(test.method, "/", strings.NewReader(test.body))
			failed.SetPathValue(test.pathName, test.pathValue)
			failedRecorder := httptest.NewRecorder()
			test.invoke(failedRecorder, failed)
			if failedRecorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("repository error status=%d body=%s", failedRecorder.Code, failedRecorder.Body.String())
			}
			repository.err = nil
		})
	}

	invalidSave := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`))
	invalidSave.SetPathValue("pageID", "page-1")
	recorder = httptest.NewRecorder()
	server.saveRevision(recorder, invalidSave, auth)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid save status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestVoteCommentResolutionAndAuditHandlers(t *testing.T) {
	t.Parallel()

	repository := &pagesRepository{}
	server := &Server{repository: repository, logger: discardLogger()}
	auth := pageAuth()

	vote := func(body string, revisionID string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
		request.SetPathValue("revisionID", revisionID)
		recorder := httptest.NewRecorder()
		server.setVote(recorder, request, auth)
		return recorder
	}
	if recorder := vote(`{"value":"reject","reason":"Chýba cena"}`, "revision-1"); recorder.Code != http.StatusOK || repository.voteValue != governance.VoteReject || repository.voteReason != "Chýba cena" {
		t.Fatalf("vote status=%d value=%q reason=%q body=%s", recorder.Code, repository.voteValue, repository.voteReason, recorder.Body.String())
	}
	if recorder := vote(`{`, "revision-1"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid vote status=%d", recorder.Code)
	}
	if recorder := vote(`{"value":"unknown"}`, "revision-1"); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported vote status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := vote(`{"value":"approve"}`, ""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing vote ID status=%d", recorder.Code)
	}
	repository.err = errors.New("vote failed")
	if recorder := vote(`{"value":"approve"}`, "revision-1"); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("vote error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil

	commentBody := `{"pageId":"page-1","revisionId":"revision-1","body":"Poznámka"}`
	recorder := httptest.NewRecorder()
	server.addComment(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(commentBody)), auth)
	if recorder.Code != http.StatusCreated || repository.commentBody != "Poznámka" {
		t.Fatalf("comment status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, body := range []string{"{", commentBody} {
		repository.err = nil
		if body == commentBody {
			repository.err = errors.New("comment failed")
		}
		recorder = httptest.NewRecorder()
		server.addComment(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), auth)
		if body == "{" && recorder.Code != http.StatusBadRequest || body != "{" && recorder.Code != http.StatusUnprocessableEntity {
			t.Fatalf("comment body=%q status=%d response=%s", body, recorder.Code, recorder.Body.String())
		}
	}
	repository.err = nil

	resolve := func(id string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.SetPathValue("commentID", id)
		recorder := httptest.NewRecorder()
		server.resolveComment(recorder, request, auth)
		return recorder
	}
	if recorder := resolve("comment-1"); recorder.Code != http.StatusOK {
		t.Fatalf("resolve status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := resolve(""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing comment ID status=%d", recorder.Code)
	}
	repository.err = errors.New("resolve failed")
	if recorder := resolve("comment-1"); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("resolve error status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	repository.err = nil
	recorder = httptest.NewRecorder()
	server.listAudit(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/audit?limit=25", nil), auth)
	if recorder.Code != http.StatusOK || repository.auditLimit != 25 {
		t.Fatalf("audit status=%d limit=%d body=%s", recorder.Code, repository.auditLimit, recorder.Body.String())
	}
	repository.err = errors.New("audit failed")
	recorder = httptest.NewRecorder()
	server.listAudit(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/audit?limit=invalid", nil), auth)
	if recorder.Code != http.StatusUnprocessableEntity || repository.auditLimit != 0 {
		t.Fatalf("audit error status=%d limit=%d body=%s", recorder.Code, repository.auditLimit, recorder.Body.String())
	}
}

func assertHandlerInvalidJSONAndRepositoryError(
	t *testing.T,
	handler func(http.ResponseWriter, *http.Request, authState),
	repository *pagesRepository,
	method, path string,
	auth authState,
	invalidBody string,
) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(method, path, strings.NewReader(invalidBody)), auth)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = errors.New("repository failed")
	valid := `{"kind":"concept","slug":"zmluva","content":{"title":"Zmluva","bodyMd":""}}`
	recorder = httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(method, path, strings.NewReader(valid)), auth)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("repository error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil
}
