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

	"viki/internal/model"
	"viki/internal/store"
)

type pagesRepository struct {
	store.Repository
	err                 error
	searchOptions       model.SearchOptions
	listedKind          *model.PageKind
	stepDefinitionQuery string
	stepDefinitionRole  *model.StepRole
	createdInput        model.CreatePageInput
	savedInput          model.SaveRevisionInput
	objectionReason     string
	resolvedObjectionID string
	commentBody         string
	auditLimit          int
}

func (r *pagesRepository) ListStepDefinitions(_ context.Context, _ string, query string, role *model.StepRole) ([]model.StepDefinition, error) {
	r.stepDefinitionQuery = query
	r.stepDefinitionRole = role
	return []model.StepDefinition{{ID: "definition-1", Expression: "zákazník má zmluvu", Role: model.StepContext, Approved: true, UsageCount: 2}}, r.err
}

func (r *pagesRepository) ListPages(_ context.Context, _ string, kind *model.PageKind) ([]model.Page, error) {
	r.listedKind = kind
	return []model.Page{{ID: "page-1", Title: "Zmluva"}}, r.err
}

func (r *pagesRepository) SearchPages(_ context.Context, _ string, options model.SearchOptions) ([]model.SearchResult, error) {
	r.searchOptions = options
	return []model.SearchResult{{Page: model.Page{ID: "page-1", Title: "Zmluva"}, RevisionID: "revision-1"}}, r.err
}

func (r *pagesRepository) CreatePage(_ context.Context, _, _ string, input model.CreatePageInput) (model.PageDetail, error) {
	r.createdInput = input
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

func (r *pagesRepository) ApproveRevision(context.Context, string, string, string) (model.PageDetail, error) {
	return model.PageDetail{Page: model.Page{ID: "page-1", Approved: true}}, r.err
}

func (r *pagesRepository) AddObjection(_ context.Context, _, _, revisionID, reason string) (model.Objection, error) {
	r.objectionReason = reason
	return model.Objection{ID: "objection-1", RevisionID: revisionID, Body: reason}, r.err
}

func (r *pagesRepository) ResolveObjection(_ context.Context, _, _, objectionID string) (model.Objection, error) {
	r.resolvedObjectionID = objectionID
	return model.Objection{ID: objectionID, Body: "resolved"}, r.err
}

func (r *pagesRepository) AddComment(_ context.Context, _, _, _, _ string, _ *string, body string) (model.Comment, error) {
	r.commentBody = body
	return model.Comment{ID: "comment-1", Body: body}, r.err
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

func TestStepDefinitionListingFiltersApprovedCatalog(t *testing.T) {
	t.Parallel()

	repository := &pagesRepository{}
	server := &Server{repository: repository, logger: discardLogger()}
	recorder := httptest.NewRecorder()
	server.listStepDefinitions(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/step-definitions?q=zmluva&role=context", nil), pageAuth())
	if recorder.Code != http.StatusOK || repository.stepDefinitionQuery != "zmluva" || repository.stepDefinitionRole == nil || *repository.stepDefinitionRole != model.StepContext || !strings.Contains(recorder.Body.String(), `"usageCount":2`) {
		t.Fatalf("status=%d query=%q role=%v body=%s", recorder.Code, repository.stepDefinitionQuery, repository.stepDefinitionRole, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	server.listStepDefinitions(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/step-definitions?role=unknown", nil), pageAuth())
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "invalid_step_role") {
		t.Fatalf("invalid role status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	repository.err = errors.New("query failed")
	recorder = httptest.NewRecorder()
	server.listStepDefinitions(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/step-definitions", nil), pageAuth())
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("repository error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPageCreationRevisionAndApprovalHandlers(t *testing.T) {
	t.Parallel()

	repository := &pagesRepository{}
	server := &Server{repository: repository, logger: discardLogger()}
	auth := pageAuth()

	recorder := httptest.NewRecorder()
	server.createPage(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/pages", strings.NewReader(`{"kind":"concept","slug":"zmluva","content":{"title":"Zmluva","bodyMd":""}}`)), auth)
	if recorder.Code != http.StatusCreated || repository.createdInput.Slug != "zmluva" {
		t.Fatalf("create status=%d input=%+v body=%s", recorder.Code, repository.createdInput, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	server.createPage(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/pages", strings.NewReader(`{"kind":"concept","conceptKind":"noun","slug":"stary-format","content":{"title":"Starý formát","bodyMd":"","aliases":["alias"]}}`)), auth)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_json") {
		t.Fatalf("legacy alias payload status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	server.createPage(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/pages", strings.NewReader(`{"kind":"feature","slug":"contracts","content":{"title":"Contracts","bodyMd":""}}`)), auth)
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "feature_requires_scenario") {
		t.Fatalf("feature without scenario status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	server.createPage(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/pages", strings.NewReader(`{"kind":"feature","slug":"contracts","content":{"title":"Contracts","bodyMd":""},"initialScenario":{"slug":"customer-signs","content":{"title":"Customer signs","bodyMd":"","steps":[{"keyword":"given","text":"a contract exists"},{"keyword":"when","text":"the customer signs"},{"keyword":"then","text":"the signature is stored"}]}}}`)), auth)
	if recorder.Code != http.StatusCreated || repository.createdInput.InitialScenario == nil || repository.createdInput.InitialScenario.Slug != "customer-signs" {
		t.Fatalf("feature with scenario status=%d input=%+v body=%s", recorder.Code, repository.createdInput, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	server.createPage(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/pages", strings.NewReader(`{"kind":"concept","conceptKind":"noun","slug":"contract","content":{"title":"Contract","bodyMd":""},"initialScenario":{"slug":"invalid","content":{"title":"Invalid","bodyMd":""}}}`)), auth)
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "invalid_initial_scenario") {
		t.Fatalf("concept with scenario status=%d body=%s", recorder.Code, recorder.Body.String())
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
		{name: "approve revision", pathName: "revisionID", pathValue: "revision-1", method: http.MethodPost, wantStatus: http.StatusOK, invoke: func(w *httptest.ResponseRecorder, r *http.Request) { server.approveRevision(w, r, auth) }},
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

func TestObjectionCommentAndAuditHandlers(t *testing.T) {
	t.Parallel()

	repository := &pagesRepository{}
	server := &Server{repository: repository, logger: discardLogger()}
	auth := pageAuth()

	objection := func(body, revisionID string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.SetPathValue("revisionID", revisionID)
		recorder := httptest.NewRecorder()
		server.raiseObjection(recorder, request, auth)
		return recorder
	}
	if recorder := objection(`{"reason":"Chýba potvrdenie"}`, "revision-1"); recorder.Code != http.StatusCreated || repository.objectionReason != "Chýba potvrdenie" || !strings.Contains(recorder.Body.String(), `"id":"objection-1"`) {
		t.Fatalf("objection status=%d reason=%q body=%s", recorder.Code, repository.objectionReason, recorder.Body.String())
	}
	if recorder := objection(`{"reason":"Chýba potvrdenie"}`, ""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing objection ID status=%d", recorder.Code)
	}
	if recorder := objection(`{`, "revision-1"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid objection status=%d", recorder.Code)
	}
	if recorder := objection(`{"reason":" "}`, "revision-1"); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty objection status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = errors.New("objection failed")
	if recorder := objection(`{"reason":"Chýba potvrdenie"}`, "revision-1"); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("objection error status=%d body=%s", recorder.Code, recorder.Body.String())
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
		request.SetPathValue("objectionID", id)
		recorder := httptest.NewRecorder()
		server.resolveObjection(recorder, request, auth)
		return recorder
	}
	if recorder := resolve("objection-1"); recorder.Code != http.StatusOK || repository.resolvedObjectionID != "objection-1" {
		t.Fatalf("resolve status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := resolve(""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing objection ID status=%d", recorder.Code)
	}
	repository.err = errors.New("resolve failed")
	if recorder := resolve("objection-1"); recorder.Code != http.StatusUnprocessableEntity {
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
