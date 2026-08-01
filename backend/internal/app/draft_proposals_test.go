package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"viki/internal/model"
	"viki/internal/store"
)

type draftProposalRepository struct {
	store.Repository
	err      error
	proposal model.AssistantDraftProposal
}

func (r *draftProposalRepository) ListAssistantDraftProposals(context.Context, string, string) ([]model.AssistantDraftProposal, error) {
	return []model.AssistantDraftProposal{r.proposal}, r.err
}

func (r *draftProposalRepository) AssistantDraftProposal(context.Context, string, string, string) (model.AssistantDraftProposal, error) {
	return r.proposal, r.err
}

func (r *draftProposalRepository) PublishAssistantDraftProposal(context.Context, string, string, string) (model.AssistantDraftProposal, error) {
	return r.proposal, r.err
}

func (r *draftProposalRepository) ReviewAssistantDraftProposalOperation(context.Context, string, string, string, string, model.AssistantOperationReviewValue, string, bool) (model.AssistantDraftProposal, error) {
	return r.proposal, r.err
}

func (r *draftProposalRepository) DiscardAssistantDraftProposal(context.Context, string, string, string, string) (model.AssistantDraftProposal, error) {
	return r.proposal, r.err
}

func draftServer(repository *draftProposalRepository) *Server {
	return &Server{
		repository: repository,
		logger:     discardLogger(),
		assistant:  &assistantRuntime{streams: map[string]*assistantEventStream{}},
	}
}

func TestDraftProposalListReadApproveAndDiscardPaths(t *testing.T) {
	t.Parallel()
	proposal := model.AssistantDraftProposal{
		ID:             "proposal-1",
		ConversationID: "conversation-1",
		TurnID:         "turn-1",
		Status:         model.AssistantProposalPublished,
	}
	repository := &draftProposalRepository{proposal: proposal}
	server := draftServer(repository)
	auth := pageAuth()

	recorder := httptest.NewRecorder()
	server.listDraftProposals(recorder, httptest.NewRequest(http.MethodGet, "/", nil), auth)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "proposal-1") {
		t.Fatalf("list status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = errors.New("list failed")
	recorder = httptest.NewRecorder()
	server.listDraftProposals(recorder, httptest.NewRequest(http.MethodGet, "/", nil), auth)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("list error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil

	invokeWithID := func(handler func(http.ResponseWriter, *http.Request, authState), body, id string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.SetPathValue("proposalID", id)
		recorder := httptest.NewRecorder()
		handler(recorder, request, auth)
		return recorder
	}

	if recorder := invokeWithID(server.draftProposal, "", "proposal-1"); recorder.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := invokeWithID(server.draftProposal, "", ""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing read ID status=%d", recorder.Code)
	}
	repository.err = errors.New("read failed")
	if recorder := invokeWithID(server.draftProposal, "", "proposal-1"); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("read error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil

	_, events, unsubscribe := server.assistant.stream("conversation-1").subscribe(0, false)
	defer unsubscribe()
	if recorder := invokeWithID(server.approveDraftProposal, "", "proposal-1"); recorder.Code != http.StatusOK {
		t.Fatalf("approve status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if event := <-events; event.Type != "draft_published" {
		t.Fatalf("approve event = %+v", event)
	}
	if recorder := invokeWithID(server.approveDraftProposal, "", ""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing approve ID status=%d", recorder.Code)
	}
	repository.err = errors.New("approve failed")
	if recorder := invokeWithID(server.approveDraftProposal, "", "proposal-1"); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("approve error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil
	repository.proposal.Status = model.AssistantProposalDiscarded
	if recorder := invokeWithID(server.discardDraftProposal, `{"reason":"  Nie je správne.  "}`, "proposal-1"); recorder.Code != http.StatusOK {
		t.Fatalf("discard status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if event := <-events; event.Type != "draft_discarded" {
		t.Fatalf("discard event = %+v", event)
	}
	if recorder := invokeWithID(server.discardDraftProposal, `{`, "proposal-1"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid discard status=%d", recorder.Code)
	}
	if recorder := invokeWithID(server.discardDraftProposal, `{"reason":"ok"}`, ""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing discard ID status=%d", recorder.Code)
	}
	for _, reason := range []string{"   ", strings.Repeat("x", 2001)} {
		if recorder := invokeWithID(server.discardDraftProposal, `{"reason":"`+reason+`"}`, "proposal-1"); recorder.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid discard reason length=%d status=%d", len(reason), recorder.Code)
		}
	}
	repository.err = errors.New("discard failed")
	if recorder := invokeWithID(server.discardDraftProposal, `{"reason":"valid"}`, "proposal-1"); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("discard error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDraftOperationReviewValidatesInputAndPublishesTerminalEvents(t *testing.T) {
	t.Parallel()
	repository := &draftProposalRepository{proposal: model.AssistantDraftProposal{
		ID: "proposal-1", ConversationID: "conversation-1", TurnID: "turn-1",
		Status: model.AssistantProposalAwaitingApproval,
	}}
	server := draftServer(repository)
	auth := pageAuth()

	invoke := func(body, proposalID, operationKey string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.SetPathValue("proposalID", proposalID)
		request.SetPathValue("operationKey", operationKey)
		recorder := httptest.NewRecorder()
		server.reviewDraftProposalOperation(recorder, request, auth)
		return recorder
	}

	if recorder := invoke(`{"value":"approve"}`, "proposal-1", "operation-1"); recorder.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, test := range []struct {
		body, proposalID, operationKey string
		status                         int
	}{
		{`{"value":"approve"}`, "", "operation-1", http.StatusBadRequest},
		{`{"value":"approve"}`, "proposal-1", "", http.StatusBadRequest},
		{`{`, "proposal-1", "operation-1", http.StatusBadRequest},
		{`{"value":"unknown"}`, "proposal-1", "operation-1", http.StatusUnprocessableEntity},
		{`{"value":"reject","reason":""}`, "proposal-1", "operation-1", http.StatusUnprocessableEntity},
		{`{"value":"reject","reason":"` + strings.Repeat("x", 2001) + `"}`, "proposal-1", "operation-1", http.StatusUnprocessableEntity},
	} {
		if recorder := invoke(test.body, test.proposalID, test.operationKey); recorder.Code != test.status {
			t.Fatalf("review body length=%d ids=%q/%q status=%d want=%d body=%s", len(test.body), test.proposalID, test.operationKey, recorder.Code, test.status, recorder.Body.String())
		}
	}

	repository.err = errors.New("review failed")
	if recorder := invoke(`{"value":"approve"}`, "proposal-1", "operation-1"); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("review error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.err = nil

	_, events, unsubscribe := server.assistant.stream("conversation-1").subscribe(0, false)
	defer unsubscribe()
	for status, eventType := range map[model.AssistantDraftProposalStatus]string{
		model.AssistantProposalPublished: "draft_published",
		model.AssistantProposalDiscarded: "draft_discarded",
	} {
		repository.proposal.Status = status
		if recorder := invoke(`{"value":"approve"}`, "proposal-1", "operation-1"); recorder.Code != http.StatusOK {
			t.Fatalf("terminal review status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		if event := <-events; event.Type != eventType {
			t.Fatalf("terminal event=%+v want=%s", event, eventType)
		}
	}

	server.publishProposalEvent(model.AssistantDraftProposal{}, "ignored")
	server.assistant = nil
	server.publishProposalEvent(repository.proposal, "ignored")
}
