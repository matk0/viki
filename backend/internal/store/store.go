package store

import (
	"context"
	"errors"
	"time"

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
	ListStepDefinitions(context.Context, string, string, *model.StepRole) ([]model.StepDefinition, error)
	CreatePage(context.Context, string, string, model.CreatePageInput) (model.PageDetail, error)
	SaveRevision(context.Context, string, string, string, model.SaveRevisionInput) (model.Revision, error)
	ApproveRevision(context.Context, string, string, string) (model.PageDetail, error)
	HasQueuedScenarioDevelopment(context.Context) (bool, error)
	ClaimScenarioDevelopment(context.Context) (model.DevelopmentTask, error)
	CompleteScenarioDevelopment(context.Context, string) (model.ScenarioDevelopment, error)
	BlockScenarioDevelopment(context.Context, string) (model.ScenarioDevelopment, error)

	AddComment(context.Context, string, string, string, string, *string, string) (model.Comment, error)
	AddObjection(context.Context, string, string, string, string) (model.Objection, error)
	ResolveObjection(context.Context, string, string, string) (model.Objection, error)

	Retrieve(context.Context, string, string, bool, int) ([]model.RetrievedDocument, error)
	ApplyAIChangeSet(context.Context, string, string, model.AssistantMutationContext, model.AIChangeSet) ([]model.Revision, error)

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
