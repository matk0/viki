package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"viki/internal/model"
	"viki/internal/store"
)

type assistantToolRepository struct {
	store.Repository
	conversation model.AssistantConversation
	documents    []model.RetrievedDocument
	pageDetail   model.PageDetail
	revision     model.Revision
	proposal     model.AssistantDraftProposal
	err          error
	pageErr      error
	mutation     model.AssistantMutationContext
	changeSet    model.AIChangeSet
	query        string
	limit        int
}

func (r *assistantToolRepository) AssistantConversation(context.Context, string, string, string) (model.AssistantConversation, error) {
	return r.conversation, r.err
}

func (r *assistantToolRepository) Retrieve(_ context.Context, _ string, query string, includeDrafts bool, limit int) ([]model.RetrievedDocument, error) {
	if !includeDrafts {
		return nil, errors.New("drafts were excluded")
	}
	r.query = query
	r.limit = limit
	return r.documents, r.err
}

func (r *assistantToolRepository) PageDetail(context.Context, string, string) (model.PageDetail, error) {
	return r.pageDetail, r.pageErr
}

func (r *assistantToolRepository) Revision(context.Context, string, string) (model.Revision, error) {
	return r.revision, r.err
}

func (r *assistantToolRepository) StageAssistantDraftProposal(_ context.Context, _, _ string, mutation model.AssistantMutationContext, changeSet model.AIChangeSet) (model.AssistantDraftProposal, error) {
	r.mutation = mutation
	r.changeSet = changeSet
	return r.proposal, r.err
}

func toolConversation() model.AssistantConversation {
	qa, edit := "qa-stored", "edit-stored"
	return model.AssistantConversation{
		ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1",
		QASessionID: &qa, EditSessionID: &edit,
	}
}

func TestHermesToolCredentialProfileAndBindingValidation(t *testing.T) {
	t.Parallel()
	conversation := toolConversation()
	server := &Server{options: Options{HermesToolToken: "service-secret"}}

	request := httptest.NewRequest(http.MethodPost, "/", nil)
	if !server.authorizeHermesToolRequest(withAuthorization(request, "Bearer service-secret")) {
		t.Fatal("valid service credential was rejected")
	}
	for _, authorization := range []string{"", "Bearer ", "Bearer wrong"} {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		if server.authorizeHermesToolRequest(withAuthorization(request, authorization)) {
			t.Fatalf("credential %q was accepted", authorization)
		}
	}
	server.options.HermesToolToken = ""
	if server.authorizeHermesToolRequest(withAuthorization(httptest.NewRequest(http.MethodPost, "/", nil), "Bearer service-secret")) {
		t.Fatal("tool request was authorized without configured credential")
	}

	for input, expected := range map[string]model.AssistantMode{
		"qa": model.AssistantQA, " viki-qa ": model.AssistantQA,
		"edit": model.AssistantEdit, "viki-edit": model.AssistantEdit,
		"unknown": "",
	} {
		if got := normalizeHermesProfile(input); got != expected {
			t.Fatalf("profile %q normalized to %q, want %q", input, got, expected)
		}
	}

	turn := &assistantTurn{Mode: model.AssistantQA, StoredID: "qa-stored"}
	if !assistantBindingMatches(conversation, turn, "qa-stored") {
		t.Fatal("exact Q&A binding did not match")
	}
	if assistantBindingMatches(conversation, turn, "runtime-id") {
		t.Fatal("runtime ID matched a durable binding")
	}
	turn.StoredID = "other"
	if assistantBindingMatches(conversation, turn, "other") {
		t.Fatal("mismatched stored ID was accepted")
	}
	conversation.QASessionID = nil
	if assistantBindingMatches(conversation, turn, "other") {
		t.Fatal("missing stored binding was accepted")
	}
}

func TestHermesSearchValidatesQueryClampsLimitAndReturnsDraftAwareResults(t *testing.T) {
	t.Parallel()
	repository := &assistantToolRepository{documents: []model.RetrievedDocument{{RevisionID: "revision-1", Draft: true}}}
	server := &Server{repository: repository, logger: discardLogger()}
	conversation := toolConversation()

	invoke := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		server.handleHermesSearch(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), conversation)
		return recorder
	}
	if recorder := invoke(`{"query":"  zmluva  ","limit":4}`); recorder.Code != http.StatusOK || repository.query != "zmluva" || repository.limit != 4 || !strings.Contains(recorder.Body.String(), "revision-1") {
		t.Fatalf("search status=%d query=%q limit=%d body=%s", recorder.Code, repository.query, repository.limit, recorder.Body.String())
	}
	for _, limit := range []int{0, 21} {
		if recorder := invoke(`{"query":"zmluva","limit":` + strconv.Itoa(limit) + `}`); recorder.Code != http.StatusOK || repository.limit != 10 {
			t.Fatalf("clamped limit input=%d got=%d status=%d", limit, repository.limit, recorder.Code)
		}
	}
	if recorder := invoke(`{`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid search status=%d", recorder.Code)
	}
	if recorder := invoke(`{"query":"   "}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blank search status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = errors.New("search failed")
	if recorder := invoke(`{"query":"zmluva"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("search error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHermesPageAndRevisionReadsExposeOnlyCurrentRevisions(t *testing.T) {
	t.Parallel()
	acceptedID, draftID := "revision-accepted", "revision-draft"
	repository := &assistantToolRepository{pageDetail: model.PageDetail{
		Page:             model.Page{ID: "page-1", AcceptedRevisionID: &acceptedID, LatestDraftRevisionID: &draftID},
		AcceptedRevision: &model.Revision{ID: acceptedID},
		DraftRevision:    &model.Revision{ID: draftID},
	}}
	server := &Server{repository: repository, logger: discardLogger()}
	conversation := toolConversation()

	page := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		server.handleHermesGetPage(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), conversation)
		return recorder
	}
	if recorder := page(`{"pageId":" page-1 "}`); recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "acceptedRevision") || !strings.Contains(recorder.Body.String(), "draftRevision") {
		t.Fatalf("page status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := page(`{`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid page status=%d", recorder.Code)
	}
	repository.pageErr = errors.New("page failed")
	if recorder := page(`{"pageId":"page-1"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("page error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.pageErr = nil
	repository.pageDetail.AcceptedRevision = nil
	repository.pageDetail.DraftRevision = nil
	if recorder := page(`{"pageId":"page-1"}`); recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), `"acceptedRevision":`) || strings.Contains(recorder.Body.String(), `"draftRevision":`) {
		t.Fatalf("empty revisions body=%s", recorder.Body.String())
	}

	revision := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		server.handleHermesGetRevision(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), conversation)
		return recorder
	}
	if recorder := revision(`{`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid revision status=%d", recorder.Code)
	}
	repository.err = errors.New("revision failed")
	if recorder := revision(`{"revisionId":"revision-accepted"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("revision error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil
	repository.revision = model.Revision{ID: acceptedID, PageID: "page-1"}
	repository.pageErr = errors.New("detail failed")
	if recorder := revision(`{"revisionId":"revision-accepted"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("revision detail error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.pageErr = nil
	for id, wantStatus := range map[string]int{
		acceptedID: http.StatusOK,
		draftID:    http.StatusOK,
		"old":      http.StatusNotFound,
	} {
		repository.revision.ID = id
		if recorder := revision(`{"revisionId":"` + id + `"}`); recorder.Code != wantStatus {
			t.Fatalf("revision %q status=%d want=%d body=%s", id, recorder.Code, wantStatus, recorder.Body.String())
		}
	}
}

func TestHermesEditStagesOnlyNonClarificationChangeSets(t *testing.T) {
	t.Parallel()
	repository := &assistantToolRepository{proposal: model.AssistantDraftProposal{ID: "proposal-1"}}
	server := &Server{repository: repository, logger: discardLogger()}
	conversation := toolConversation()
	turn := &assistantTurn{ID: "turn-1", StoredID: "edit-stored"}

	invoke := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		server.handleHermesProposeChanges(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), conversation, turn)
		return recorder
	}
	valid := `{"summary":"Zmena","operations":[{"operation":"create","kind":"concept","slug":"zmluva","content":{"title":"Zmluva","bodyMd":""}}]}`
	if recorder := invoke(valid); recorder.Code != http.StatusOK || repository.mutation.ConversationID != conversation.ID || repository.mutation.TurnID != turn.ID || repository.mutation.HermesProfile != "viki-edit" || repository.mutation.HermesSessionID != turn.StoredID || !strings.Contains(recorder.Body.String(), "proposal-1") {
		t.Fatalf("proposal status=%d mutation=%+v body=%s", recorder.Code, repository.mutation, recorder.Body.String())
	}
	for _, body := range []string{
		`{`,
		`{"summary":"Zmena","operations":[]}`,
		`{"clarification":"Otázka?","operations":[{"operation":"create"}]}`,
	} {
		recorder := invoke(body)
		want := http.StatusUnprocessableEntity
		if body == "{" {
			want = http.StatusBadRequest
		}
		if recorder.Code != want {
			t.Fatalf("changeset body=%q status=%d want=%d response=%s", body, recorder.Code, want, recorder.Body.String())
		}
	}
	repository.err = errors.New("stage failed")
	if recorder := invoke(valid); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("stage error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func withAuthorization(request *http.Request, value string) *http.Request {
	request.Header.Set("Authorization", value)
	return request
}

func TestHermesToolRouterFailsClosedAndDispatchesEveryAllowedTool(t *testing.T) {
	t.Parallel()
	conversation := toolConversation()
	acceptedID := "revision-accepted"
	repository := &assistantToolRepository{
		conversation: conversation,
		documents:    []model.RetrievedDocument{{RevisionID: acceptedID}},
		pageDetail: model.PageDetail{
			Page:             model.Page{ID: "page-1", AcceptedRevisionID: &acceptedID},
			AcceptedRevision: &model.Revision{ID: acceptedID},
		},
		revision: model.Revision{ID: acceptedID, PageID: "page-1"},
		proposal: model.AssistantDraftProposal{ID: "proposal-1"},
	}
	runtime := bareAssistantRuntime(repository, nil)
	qaTurn := &assistantTurn{ID: "turn-qa", ConversationID: conversation.ID, OrganizationID: conversation.OrganizationID, UserID: conversation.UserID, Mode: model.AssistantQA, StoredID: "qa-stored"}
	editTurn := &assistantTurn{ID: "turn-edit", ConversationID: conversation.ID, OrganizationID: conversation.OrganizationID, UserID: conversation.UserID, Mode: model.AssistantEdit, StoredID: "edit-stored"}
	runtime.activeByStored[assistantSessionKey(model.AssistantQA, qaTurn.StoredID)] = qaTurn
	runtime.activeByStored[assistantSessionKey(model.AssistantEdit, editTurn.StoredID)] = editTurn
	server := &Server{
		repository: repository,
		assistant:  runtime,
		options:    Options{HermesToolToken: "service-secret"},
		logger:     discardLogger(),
	}

	invoke := func(tool, token, profile, sessionID, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.SetPathValue("tool", tool)
		request.Header.Set("Authorization", token)
		request.Header.Set("X-Hermes-Profile", profile)
		request.Header.Set("X-Hermes-Session-ID", sessionID)
		recorder := httptest.NewRecorder()
		server.handleHermesTool(recorder, request)
		return recorder
	}

	for _, test := range []struct {
		name, tool, token, profile, sessionID, body string
		status                                      int
	}{
		{name: "credential", tool: "search_viki", profile: "qa", sessionID: "qa-stored", body: `{"query":"x"}`, status: http.StatusUnauthorized},
		{name: "profile", tool: "search_viki", token: "Bearer service-secret", profile: "unknown", sessionID: "qa-stored", body: `{"query":"x"}`, status: http.StatusForbidden},
		{name: "session", tool: "search_viki", token: "Bearer service-secret", profile: "qa", body: `{"query":"x"}`, status: http.StatusForbidden},
		{name: "inactive", tool: "search_viki", token: "Bearer service-secret", profile: "qa", sessionID: "missing", body: `{"query":"x"}`, status: http.StatusForbidden},
		{name: "not allowed", tool: "propose_viki_changeset", token: "Bearer service-secret", profile: "qa", sessionID: "qa-stored", body: `{"operations":[]}`, status: http.StatusForbidden},
		{name: "unknown", tool: "unknown", token: "Bearer service-secret", profile: "qa", sessionID: "qa-stored", body: `{}`, status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			if recorder := invoke(test.tool, test.token, test.profile, test.sessionID, test.body); recorder.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}

	repository.err = errors.New("binding lookup failed")
	if recorder := invoke("search_viki", "Bearer service-secret", "qa", "qa-stored", `{"query":"x"}`); recorder.Code != http.StatusForbidden {
		t.Fatalf("lookup failure status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil
	repository.conversation.QASessionID = nil
	if recorder := invoke("search_viki", "Bearer service-secret", "qa", "qa-stored", `{"query":"x"}`); recorder.Code != http.StatusForbidden {
		t.Fatalf("binding mismatch status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.conversation = conversation

	for _, test := range []struct {
		tool, profile, sessionID, body string
	}{
		{"search_viki", "qa", "qa-stored", `{"query":"zmluva"}`},
		{"get_viki_page", "qa", "qa-stored", `{"pageId":"page-1"}`},
		{"get_viki_revision", "qa", "qa-stored", `{"revisionId":"revision-accepted"}`},
		{"propose_viki_changeset", "edit", "edit-stored", `{"summary":"Zmena","operations":[{"operation":"create","kind":"concept","slug":"zmluva","content":{"title":"Zmluva","bodyMd":""}}]}`},
	} {
		if recorder := invoke(test.tool, "Bearer service-secret", test.profile, test.sessionID, test.body); recorder.Code != http.StatusOK {
			t.Fatalf("tool=%s status=%d body=%s", test.tool, recorder.Code, recorder.Body.String())
		}
	}
}
