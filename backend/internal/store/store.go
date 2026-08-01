package store

import (
	"context"
	"errors"
	"time"

	"viki/internal/governance"
	"viki/internal/model"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("conflict")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrInvalidHierarchy = errors.New("invalid page hierarchy")
	ErrDuplicateSlug    = errors.New("duplicate slug")
	ErrInvalidReference = errors.New("invalid page reference")
)

type Repository interface {
	Ping(context.Context) error
	CredentialByEmail(context.Context, string) (model.Credential, error)
	CreateSession(context.Context, string, []byte, []byte, time.Time) error
	SessionByHash(context.Context, []byte) (model.Session, error)
	DeleteSession(context.Context, []byte) error

	ListPages(context.Context, string, *model.PageKind) ([]model.Page, error)
	SearchPages(context.Context, string, model.SearchOptions) ([]model.SearchResult, error)
	PageDetail(context.Context, string, string) (model.PageDetail, error)
	Revision(context.Context, string, string) (model.Revision, error)
	CreatePage(context.Context, string, string, model.CreatePageInput, model.RevisionStatus) (model.PageDetail, error)
	SaveRevision(context.Context, string, string, string, model.SaveRevisionInput) (model.Revision, error)
	PublishRevision(context.Context, string, string, string) (model.PageDetail, error)

	AddComment(context.Context, string, string, string, string, *string, *string, *string, string, bool) (model.Comment, error)
	ResolveComment(context.Context, string, string, string) (model.Comment, error)
	SetVote(context.Context, string, string, string, governance.VoteValue, string) (model.Vote, error)

	Retrieve(context.Context, string, string, bool, int) ([]model.RetrievedDocument, error)
	ApplyAIChangeSet(context.Context, string, string, model.AssistantMutationContext, model.AIChangeSet) ([]model.Revision, error)
	StageAssistantDraftProposal(context.Context, string, string, model.AssistantMutationContext, model.AIChangeSet) (model.AssistantDraftProposal, error)
	ListAssistantDraftProposals(context.Context, string, string) ([]model.AssistantDraftProposal, error)
	AssistantDraftProposal(context.Context, string, string, string) (model.AssistantDraftProposal, error)
	ReviewAssistantDraftProposalOperation(context.Context, string, string, string, string, model.AssistantOperationReviewValue, string, bool) (model.AssistantDraftProposal, error)
	PublishAssistantDraftProposal(context.Context, string, string, string) (model.AssistantDraftProposal, error)
	DiscardAssistantDraftProposal(context.Context, string, string, string, string) (model.AssistantDraftProposal, error)

	ListAssistantConversations(context.Context, string, string) ([]model.AssistantConversation, error)
	CreateAssistantConversation(context.Context, string, string, model.AssistantMode) (model.AssistantConversation, error)
	AssistantConversation(context.Context, string, string, string) (model.AssistantConversation, error)
	AssistantConversationBySession(context.Context, model.AssistantMode, string) (model.AssistantConversation, error)
	SetAssistantSession(context.Context, string, string, string, model.AssistantMode, string) error
	SetAssistantPrimaryMode(context.Context, string, string, string, model.AssistantMode) error
	UpdateAssistantMode(context.Context, string, string, string, model.AssistantMode) error
	UpdateAssistantHandoffCursor(context.Context, string, string, string, model.AssistantMode, int) error
	AssistantDraftReceipts(context.Context, string, string) (map[string][]model.AssistantDraftReceipt, error)

	ListAudit(context.Context, string, int) ([]model.AuditEvent, error)
}
