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
	conversation      model.AssistantConversation
	documents         []model.RetrievedDocument
	definitions       []model.StepDefinition
	pageDetail        model.PageDetail
	revision          model.Revision
	revisions         []model.Revision
	err               error
	definitionErr     error
	pageErr           error
	mutation          model.AssistantMutationContext
	changeSet         model.AIChangeSet
	organization      string
	user              string
	query             string
	definitionQuery   string
	limit             int
	queued            bool
	claimed           model.DevelopmentTask
	claimHook         func()
	development       model.ScenarioDevelopment
	completedRevision string
	blockedRevision   string
	completed         string
	blocked           string
}

func (r *assistantToolRepository) ListStepDefinitions(_ context.Context, _ string, query string, _ *model.StepRole) ([]model.StepDefinition, error) {
	r.definitionQuery = query
	return r.definitions, r.definitionErr
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

func (r *assistantToolRepository) ApplyAIChangeSet(_ context.Context, organizationID, userID string, mutation model.AssistantMutationContext, changeSet model.AIChangeSet) ([]model.Revision, error) {
	r.organization = organizationID
	r.user = userID
	r.mutation = mutation
	r.changeSet = changeSet
	return r.revisions, r.err
}

func (r *assistantToolRepository) HasQueuedScenarioDevelopment(context.Context) (bool, error) {
	return r.queued, r.err
}

func (r *assistantToolRepository) ClaimScenarioDevelopment(context.Context) (model.DevelopmentTask, error) {
	if r.claimHook != nil {
		r.claimHook()
	}
	return r.claimed, r.err
}

func (r *assistantToolRepository) CompleteScenarioDevelopment(_ context.Context, revisionID, detail string) (model.ScenarioDevelopment, error) {
	r.completedRevision = revisionID
	r.completed = detail
	return r.development, r.err
}

func (r *assistantToolRepository) BlockScenarioDevelopment(_ context.Context, revisionID, detail string) (model.ScenarioDevelopment, error) {
	r.blockedRevision = revisionID
	r.blocked = detail
	return r.development, r.err
}

type fakeDevelopmentTarget struct {
	implementation string
	receipt        string
	err            error
}

func (target *fakeDevelopmentTarget) Apply(_ context.Context, implementation string) (string, error) {
	target.implementation = implementation
	return target.receipt, target.err
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
	repository := &assistantToolRepository{
		documents:   []model.RetrievedDocument{{RevisionID: "revision-1", Draft: true}},
		definitions: []model.StepDefinition{{ID: "definition-1", Expression: "zákazník podpíše zmluvu", Role: model.StepAction, Approved: true, UsageCount: 3}},
	}
	server := &Server{repository: repository, logger: discardLogger()}
	conversation := toolConversation()

	invoke := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		server.handleHermesSearch(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), conversation, model.AssistantEdit)
		return recorder
	}
	if recorder := invoke(`{"query":"  zmluva  ","limit":4}`); recorder.Code != http.StatusOK || repository.query != "zmluva" || repository.definitionQuery != "zmluva" || repository.limit != 4 || !strings.Contains(recorder.Body.String(), "revision-1") || !strings.Contains(recorder.Body.String(), "definition-1") {
		t.Fatalf("search status=%d query=%q definitionQuery=%q limit=%d body=%s", recorder.Code, repository.query, repository.definitionQuery, repository.limit, recorder.Body.String())
	}
	repository.definitionQuery = ""
	recorder := httptest.NewRecorder()
	server.handleHermesSearch(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"query":"zmluva"}`)), conversation, model.AssistantQA)
	if recorder.Code != http.StatusOK || repository.definitionQuery != "" || strings.Contains(recorder.Body.String(), "stepDefinitions") {
		t.Fatalf("Q&A search exposed step definitions: status=%d body=%s", recorder.Code, recorder.Body.String())
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
	repository.err = nil
	repository.definitionErr = errors.New("definition search failed")
	if recorder := invoke(`{"query":"zmluva"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("definition search error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHermesPageAndRevisionReadsExposeOnlyCurrentRevisions(t *testing.T) {
	t.Parallel()
	approvedID, draftID := "revision-approved", "revision-draft"
	repository := &assistantToolRepository{pageDetail: model.PageDetail{
		Page:             model.Page{ID: "page-1", ApprovedRevisionID: &approvedID, LatestDraftRevisionID: &draftID},
		ApprovedRevision: &model.Revision{ID: approvedID},
		DraftRevision:    &model.Revision{ID: draftID},
	}}
	server := &Server{repository: repository, logger: discardLogger()}
	conversation := toolConversation()

	page := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		server.handleHermesGetPage(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), conversation)
		return recorder
	}
	if recorder := page(`{"pageId":" page-1 "}`); recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "approvedRevision") || !strings.Contains(recorder.Body.String(), "draftRevision") {
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
	repository.pageDetail.ApprovedRevision = nil
	repository.pageDetail.DraftRevision = nil
	if recorder := page(`{"pageId":"page-1"}`); recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), `"approvedRevision":`) || strings.Contains(recorder.Body.String(), `"draftRevision":`) {
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
	if recorder := revision(`{"revisionId":"revision-approved"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("revision error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil
	repository.revision = model.Revision{ID: approvedID, PageID: "page-1"}
	repository.pageErr = errors.New("detail failed")
	if recorder := revision(`{"revisionId":"revision-approved"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("revision detail error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.pageErr = nil
	for id, wantStatus := range map[string]int{
		approvedID: http.StatusOK,
		draftID:    http.StatusOK,
		"old":      http.StatusNotFound,
	} {
		repository.revision.ID = id
		if recorder := revision(`{"revisionId":"` + id + `"}`); recorder.Code != wantStatus {
			t.Fatalf("revision %q status=%d want=%d body=%s", id, recorder.Code, wantStatus, recorder.Body.String())
		}
	}
}

func TestHermesEditAppliesOnlyNonClarificationChangeSetsAsSafeDraftReceipts(t *testing.T) {
	t.Parallel()
	repository := &assistantToolRepository{revisions: []model.Revision{
		{ID: "revision-1", PageID: "page-1", Title: "Zmluva", BodyMD: "sensitive draft body"},
		{PageID: "page-without-revision", Title: "Missing revision ID"},
		{ID: "revision-without-page", Title: "Missing page ID"},
	}}
	server := &Server{repository: repository, logger: discardLogger()}
	conversation := toolConversation()
	turn := &assistantTurn{ID: "turn-1", StoredID: "edit-stored"}

	invoke := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		server.handleHermesApplyDraftChanges(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), conversation, turn)
		return recorder
	}
	valid := `{"summary":"Zmena","operations":[{"operation":"create","kind":"concept","slug":"zmluva","content":{"title":"Zmluva","bodyMd":""}}]}`
	if recorder := invoke(valid); recorder.Code != http.StatusOK || repository.organization != conversation.OrganizationID || repository.user != conversation.UserID || repository.mutation.ConversationID != conversation.ID || repository.mutation.TurnID != turn.ID || repository.mutation.HermesProfile != "viki-edit" || repository.mutation.HermesSessionID != turn.StoredID || !strings.Contains(recorder.Body.String(), `"drafts":[{"revisionId":"revision-1","pageId":"page-1","pageTitle":"Zmluva"}]`) || strings.Contains(recorder.Body.String(), "sensitive draft body") {
		t.Fatalf("draft status=%d organization=%q user=%q mutation=%+v body=%s", recorder.Code, repository.organization, repository.user, repository.mutation, recorder.Body.String())
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
	repository.err = errors.New("apply failed")
	if recorder := invoke(valid); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("apply error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func withAuthorization(request *http.Request, value string) *http.Request {
	request.Header.Set("Authorization", value)
	return request
}

func TestHermesToolRouterFailsClosedAndDispatchesEveryAllowedTool(t *testing.T) {
	t.Parallel()
	conversation := toolConversation()
	approvedID := "revision-approved"
	repository := &assistantToolRepository{
		conversation: conversation,
		documents:    []model.RetrievedDocument{{RevisionID: approvedID}},
		pageDetail: model.PageDetail{
			Page:             model.Page{ID: "page-1", ApprovedRevisionID: &approvedID},
			ApprovedRevision: &model.Revision{ID: approvedID},
		},
		revision:  model.Revision{ID: approvedID, PageID: "page-1"},
		revisions: []model.Revision{{ID: "revision-draft", PageID: "page-draft", Title: "Zmluva"}},
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
		{name: "not allowed", tool: "apply_viki_draft_changeset", token: "Bearer service-secret", profile: "qa", sessionID: "qa-stored", body: `{"operations":[]}`, status: http.StatusForbidden},
		{name: "legacy mutation", tool: "propose_viki_changeset", token: "Bearer service-secret", profile: "edit", sessionID: "edit-stored", body: `{"operations":[]}`, status: http.StatusForbidden},
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
		{"get_viki_revision", "qa", "qa-stored", `{"revisionId":"revision-approved"}`},
		{"apply_viki_draft_changeset", "edit", "edit-stored", `{"summary":"Zmena","operations":[{"operation":"create","kind":"concept","slug":"zmluva","content":{"title":"Zmluva","bodyMd":""}}]}`},
	} {
		if recorder := invoke(test.tool, "Bearer service-secret", test.profile, test.sessionID, test.body); recorder.Code != http.StatusOK {
			t.Fatalf("tool=%s status=%d body=%s", test.tool, recorder.Code, recorder.Body.String())
		}
	}
}

func TestDeveloperToolsProcessOneScenarioWithoutModelControlledIdentity(t *testing.T) {
	t.Parallel()
	repository := &assistantToolRepository{
		queued: true,
		claimed: model.DevelopmentTask{
			ScenarioDevelopment: model.ScenarioDevelopment{RevisionID: "revision-1", Status: model.DevelopmentRunning},
			Scenario: model.Revision{ID: "revision-1", Title: "Customer signs a contract", Steps: []model.Step{
				{Keyword: model.KeywordGiven, Text: "a contract is ready"},
				{Keyword: model.KeywordWhen, Text: "the customer signs"},
				{Keyword: model.KeywordThen, Text: "the signature is stored"},
			}},
		},
		development: model.ScenarioDevelopment{RevisionID: "revision-1", Status: model.DevelopmentDeveloped, Detail: "target-receipt-1"},
	}
	target := &fakeDevelopmentTarget{receipt: "target-receipt-1"}
	server := &Server{
		repository: repository,
		assistant:  bareAssistantRuntime(repository, nil),
		target:     target,
		options: Options{
			HermesToolToken:    "assistant-secret",
			DeveloperEnabled:   true,
			DeveloperToolToken: "developer-secret",
		},
		logger: discardLogger(),
	}

	pendingRequest := withAuthorization(httptest.NewRequest(http.MethodGet, "/", nil), "Bearer developer-secret")
	pending := httptest.NewRecorder()
	server.handleDevelopmentPending(pending, pendingRequest)
	if pending.Code != http.StatusOK || !strings.Contains(pending.Body.String(), `"wakeAgent":true`) {
		t.Fatalf("pending status=%d body=%s", pending.Code, pending.Body.String())
	}

	invoke := func(tool, token, profile, sessionID, taskID, lease, body string) *httptest.ResponseRecorder {
		request := withAuthorization(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), token)
		request.SetPathValue("tool", tool)
		request.Header.Set("X-Hermes-Profile", profile)
		request.Header.Set("X-Hermes-Session-ID", sessionID)
		request.Header.Set("X-Hermes-Task-ID", taskID)
		request.Header.Set("X-Viki-Development-Lease", lease)
		recorder := httptest.NewRecorder()
		server.handleHermesTool(recorder, request)
		return recorder
	}

	claim := invoke("claim_next_scenario", "Bearer developer-secret", "viki-developer", "cron-session-1", "task-1", "", `{}`)
	lease := claim.Header().Get("X-Viki-Development-Lease")
	if claim.Code != http.StatusOK || lease == "" || strings.Contains(claim.Body.String(), lease) || !strings.Contains(claim.Body.String(), "Customer signs a contract") {
		t.Fatalf("claim status=%d lease=%q body=%s", claim.Code, lease, claim.Body.String())
	}
	if recorder := invoke("complete_scenario_development", "Bearer developer-secret", "viki-developer", "cron-session-1", "task-1", lease, `{"implementation":"Implemented contract signing"}`); recorder.Code != http.StatusOK || repository.completedRevision != "revision-1" || repository.completed != "target-receipt-1" || target.implementation != "Implemented contract signing" {
		t.Fatalf("complete status=%d revision=%q completed=%q implementation=%q body=%s", recorder.Code, repository.completedRevision, repository.completed, target.implementation, recorder.Body.String())
	}

	claim = invoke("claim_next_scenario", "Bearer developer-secret", "developer", "cron-session-2", "task-2", "", `{}`)
	lease = claim.Header().Get("X-Viki-Development-Lease")
	if claim.Code != http.StatusOK || lease == "" {
		t.Fatalf("second claim status=%d lease=%q body=%s", claim.Code, lease, claim.Body.String())
	}
	repository.development = model.ScenarioDevelopment{RevisionID: "revision-1", Status: model.DevelopmentBlocked, Detail: "Missing API"}
	if recorder := invoke("block_scenario_development", "Bearer developer-secret", "developer", "cron-session-2", "task-2", lease, `{"reason":"Missing API"}`); recorder.Code != http.StatusOK || repository.blockedRevision != "revision-1" || repository.blocked != "Missing API" {
		t.Fatalf("block status=%d revision=%q blocked=%q body=%s", recorder.Code, repository.blockedRevision, repository.blocked, recorder.Body.String())
	}

	for _, test := range []struct {
		tool, token, profile, session, task string
		status                              int
	}{
		{tool: "claim_next_scenario", token: "Bearer assistant-secret", profile: "developer", session: "cron-session", task: "task-1", status: http.StatusUnauthorized},
		{tool: "search_viki", token: "Bearer developer-secret", profile: "qa", session: "qa-stored", task: "task-1", status: http.StatusUnauthorized},
		{tool: "claim_next_scenario", token: "Bearer developer-secret", profile: "qa", session: "qa-stored", task: "task-1", status: http.StatusUnauthorized},
		{tool: "search_viki", token: "Bearer developer-secret", profile: "developer", session: "cron-session", task: "task-1", status: http.StatusForbidden},
		{tool: "claim_next_scenario", token: "Bearer developer-secret", profile: "developer", session: "cron-session", task: "", status: http.StatusForbidden},
	} {
		if recorder := invoke(test.tool, test.token, test.profile, test.session, test.task, "", `{}`); recorder.Code != test.status {
			t.Fatalf("tool=%s profile=%s status=%d want=%d body=%s", test.tool, test.profile, recorder.Code, test.status, recorder.Body.String())
		}
	}
}

func TestDeveloperToolsAreDisabledWithoutAnExplicitCapability(t *testing.T) {
	t.Parallel()
	repository := &assistantToolRepository{claimed: model.DevelopmentTask{
		ScenarioDevelopment: model.ScenarioDevelopment{RevisionID: "revision-1", Status: model.DevelopmentRunning},
	}}
	server := &Server{
		repository: repository,
		assistant:  bareAssistantRuntime(repository, nil),
		options:    Options{HermesToolToken: "assistant-secret"},
		logger:     discardLogger(),
	}

	request := withAuthorization(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)), "Bearer assistant-secret")
	request.SetPathValue("tool", "claim_next_scenario")
	request.Header.Set("X-Hermes-Profile", "developer")
	request.Header.Set("X-Hermes-Session-ID", "arbitrary-session")
	recorder := httptest.NewRecorder()

	server.handleHermesTool(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("developer tool without an explicit capability status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	pending := httptest.NewRecorder()
	server.handleDevelopmentPending(pending, httptest.NewRequest(http.MethodGet, "/", nil))
	if pending.Code != http.StatusOK || !strings.Contains(pending.Body.String(), `"wakeAgent":false`) {
		t.Fatalf("disabled pending status=%d body=%s", pending.Code, pending.Body.String())
	}
}

func TestDeveloperClaimFailsClosedWhenItsLeaseCannotBeBound(t *testing.T) {
	t.Parallel()
	repository := &assistantToolRepository{claimed: model.DevelopmentTask{
		ScenarioDevelopment: model.ScenarioDevelopment{RevisionID: "revision-1", Status: model.DevelopmentRunning},
	}}
	server := &Server{
		repository: repository,
		assistant:  bareAssistantRuntime(repository, nil),
		options: Options{
			DeveloperEnabled:   true,
			DeveloperToolToken: "developer-secret",
		},
		logger: discardLogger(),
	}
	repository.claimHook = func() {
		server.claims.mu.Lock()
		server.claims.bySession["cron-session"] = developmentClaim{taskID: "other-task", lease: "other-lease"}
		server.claims.mu.Unlock()
	}
	request := withAuthorization(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)), "Bearer developer-secret")
	request.SetPathValue("tool", "claim_next_scenario")
	request.Header.Set("X-Hermes-Profile", "developer")
	request.Header.Set("X-Hermes-Session-ID", "cron-session")
	request.Header.Set("X-Hermes-Task-ID", "task-1")
	recorder := httptest.NewRecorder()

	server.handleHermesTool(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("unbound claim status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDeveloperClaimFailsClosedWhenLeaseGenerationFails(t *testing.T) {
	original := readDevelopmentLeaseRandom
	readDevelopmentLeaseRandom = func([]byte) (int, error) { return 0, errors.New("entropy unavailable") }
	t.Cleanup(func() { readDevelopmentLeaseRandom = original })
	repository := &assistantToolRepository{claimHook: func() { t.Fatal("repository claim ran without a lease") }}
	server := &Server{
		repository: repository,
		assistant:  bareAssistantRuntime(repository, nil),
		options: Options{
			DeveloperEnabled:   true,
			DeveloperToolToken: "developer-secret",
		},
		logger: discardLogger(),
	}
	request := withAuthorization(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)), "Bearer developer-secret")
	request.SetPathValue("tool", "claim_next_scenario")
	request.Header.Set("X-Hermes-Profile", "developer")
	request.Header.Set("X-Hermes-Session-ID", "cron-session")
	request.Header.Set("X-Hermes-Task-ID", "task-1")
	recorder := httptest.NewRecorder()

	server.handleHermesTool(recorder, request)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("lease generation status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDeveloperToolFailuresAreReturnedWithoutChangingState(t *testing.T) {
	t.Parallel()
	repository := &assistantToolRepository{
		claimed:     model.DevelopmentTask{ScenarioDevelopment: model.ScenarioDevelopment{RevisionID: "revision-1", Status: model.DevelopmentRunning}},
		development: model.ScenarioDevelopment{RevisionID: "revision-1", Status: model.DevelopmentDeveloped, Detail: "target-receipt-1"},
	}
	target := &fakeDevelopmentTarget{receipt: "target-receipt-1"}
	server := &Server{
		repository: repository,
		assistant:  bareAssistantRuntime(repository, nil),
		target:     target,
		options: Options{
			HermesToolToken:    "assistant-secret",
			DeveloperEnabled:   true,
			DeveloperToolToken: "developer-secret",
		},
		logger: discardLogger(),
	}

	pending := func(authorization string) *httptest.ResponseRecorder {
		request := withAuthorization(httptest.NewRequest(http.MethodGet, "/", nil), authorization)
		recorder := httptest.NewRecorder()
		server.handleDevelopmentPending(recorder, request)
		return recorder
	}
	if recorder := pending("Bearer wrong"); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized pending status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = errors.New("queue unavailable")
	if recorder := pending("Bearer developer-secret"); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("failed pending status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil

	invoke := func(tool, sessionID, taskID, lease, body string) *httptest.ResponseRecorder {
		request := withAuthorization(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), "Bearer developer-secret")
		request.SetPathValue("tool", tool)
		request.Header.Set("X-Hermes-Profile", "developer")
		request.Header.Set("X-Hermes-Session-ID", sessionID)
		request.Header.Set("X-Hermes-Task-ID", taskID)
		request.Header.Set("X-Viki-Development-Lease", lease)
		recorder := httptest.NewRecorder()
		server.handleHermesTool(recorder, request)
		return recorder
	}
	claim := func(sessionID, taskID string) string {
		t.Helper()
		recorder := invoke("claim_next_scenario", sessionID, taskID, "", `{}`)
		lease := recorder.Header().Get("X-Viki-Development-Lease")
		if recorder.Code != http.StatusOK || lease == "" {
			t.Fatalf("claim status=%d lease=%q body=%s", recorder.Code, lease, recorder.Body.String())
		}
		return lease
	}

	for _, tool := range []string{"claim_next_scenario", "complete_scenario_development", "block_scenario_development"} {
		if recorder := invoke(tool, "cron-session", "task-1", "", `{`); recorder.Code != http.StatusBadRequest {
			t.Fatalf("malformed %s status=%d body=%s", tool, recorder.Code, recorder.Body.String())
		}
	}
	if recorder := invoke("complete_scenario_development", "cron-session", "task-1", "", `{"implementation":" "}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty implementation status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := invoke("block_scenario_development", "cron-session", "task-1", "", `{"reason":" "}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty reason status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := invoke("complete_scenario_development", "cron-session", "task-1", "", `{"implementation":"implementation"}`); recorder.Code != http.StatusForbidden || target.implementation != "" {
		t.Fatalf("unclaimed completion status=%d implementation=%q body=%s", recorder.Code, target.implementation, recorder.Body.String())
	}
	if recorder := invoke("block_scenario_development", "cron-session", "task-1", "", `{"reason":"reason"}`); recorder.Code != http.StatusForbidden {
		t.Fatalf("unclaimed block status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	repository.err = errors.New("repository unavailable")
	if recorder := invoke("claim_next_scenario", "cron-session", "task-1", "", `{}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("failed claim status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil

	lease := claim("cron-session", "task-1")
	if recorder := invoke("claim_next_scenario", "cron-session", "task-2", "", `{}`); recorder.Code != http.StatusConflict {
		t.Fatalf("duplicate claim status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = errors.New("repository unavailable")
	if recorder := invoke("complete_scenario_development", "cron-session", "task-1", lease, `{"implementation":"implementation"}`); recorder.Code != http.StatusUnprocessableEntity || target.implementation != "implementation" {
		t.Fatalf("failed completion status=%d implementation=%q body=%s", recorder.Code, target.implementation, recorder.Body.String())
	}
	if recorder := invoke("complete_scenario_development", "cron-session", "task-1", lease, `{"implementation":"implementation"}`); recorder.Code != http.StatusForbidden {
		t.Fatalf("released completion status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil

	target.err = errors.New("target unavailable")
	lease = claim("cron-session", "task-2")
	if recorder := invoke("complete_scenario_development", "cron-session", "task-2", lease, `{"implementation":"second implementation"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("failed target status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := invoke("complete_scenario_development", "cron-session", "task-2", lease, `{"implementation":"second implementation"}`); recorder.Code != http.StatusForbidden {
		t.Fatalf("released target failure status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	target.err = nil

	lease = claim("cron-session", "task-3")
	repository.err = errors.New("repository unavailable")
	if recorder := invoke("block_scenario_development", "cron-session", "task-3", lease, `{"reason":"reason"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("failed block status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil
	repository.development = model.ScenarioDevelopment{RevisionID: "revision-1", Status: model.DevelopmentBlocked, Detail: "reason"}
	if recorder := invoke("block_scenario_development", "cron-session", "task-3", lease, `{"reason":"reason"}`); recorder.Code != http.StatusOK || repository.blockedRevision != "revision-1" {
		t.Fatalf("retried block status=%d revision=%q body=%s", recorder.Code, repository.blockedRevision, recorder.Body.String())
	}
}
