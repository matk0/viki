package model

import "time"

type PageKind string

const (
	PageConcept  PageKind = "concept"
	PageFeature  PageKind = "feature"
	PageScenario PageKind = "scenario"
)

type ConceptKind string

const (
	ConceptNoun ConceptKind = "noun"
	ConceptVerb ConceptKind = "verb"
)

type RevisionStatus string

const (
	RevisionDraft      RevisionStatus = "draft"
	RevisionApproved   RevisionStatus = "approved"
	RevisionSuperseded RevisionStatus = "superseded"
)

type DevelopmentStatus string

const (
	DevelopmentQueued    DevelopmentStatus = "queued"
	DevelopmentRunning   DevelopmentStatus = "running"
	DevelopmentDeveloped DevelopmentStatus = "developed"
	DevelopmentBlocked   DevelopmentStatus = "blocked"
)

type BDDKeyword string

const (
	KeywordGiven BDDKeyword = "given"
	KeywordWhen  BDDKeyword = "when"
	KeywordThen  BDDKeyword = "then"
	KeywordAnd   BDDKeyword = "and"
	KeywordBut   BDDKeyword = "but"
)

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Credential struct {
	User
	OrganizationID string
	PasswordHash   string
	Active         bool
}

type Session struct {
	User           User
	OrganizationID string
	CSRFHash       []byte
	Expires        time.Time
}

type Page struct {
	ID                    string       `json:"id"`
	Kind                  PageKind     `json:"kind"`
	ConceptKind           *ConceptKind `json:"conceptKind,omitempty"`
	ParentID              *string      `json:"parentId,omitempty"`
	Slug                  string       `json:"slug"`
	Title                 string       `json:"title"`
	ApprovedRevisionID    *string      `json:"approvedRevisionId,omitempty"`
	LatestDraftRevisionID *string      `json:"latestDraftRevisionId,omitempty"`
	ApprovedRevisionTitle *string      `json:"approvedRevisionTitle,omitempty"`
	DraftRevisionTitle    *string      `json:"draftRevisionTitle,omitempty"`
	Approved              bool         `json:"approved"`
	HasDraft              bool         `json:"hasDraft"`
	UnresolvedObjections  int          `json:"unresolvedObjections"`
	CreatedAt             time.Time    `json:"createdAt"`
	UpdatedAt             time.Time    `json:"updatedAt"`
}

type Step struct {
	ID           string     `json:"id"`
	StableID     string     `json:"stableId"`
	Position     int        `json:"position"`
	Keyword      BDDKeyword `json:"keyword"`
	DefinitionID string     `json:"definitionId,omitempty"`
	Expression   string     `json:"expression,omitempty"`
	Arguments    []string   `json:"arguments"`
	Text         string     `json:"text"`
}

type StepRole string

const (
	StepContext StepRole = "context"
	StepAction  StepRole = "action"
	StepOutcome StepRole = "outcome"
)

type StepDefinition struct {
	ID         string   `json:"id"`
	Expression string   `json:"expression"`
	Role       StepRole `json:"role"`
	Approved   bool     `json:"approved"`
	UsageCount int      `json:"usageCount"`
}

type PageReference struct {
	TargetPageID    string `json:"targetPageId,omitempty"`
	TargetClientKey string `json:"targetClientKey,omitempty"`
	TargetTitle     string `json:"targetTitle,omitempty"`
	Relation        string `json:"relation"`
}

type Revision struct {
	ID             string          `json:"id"`
	PageID         string          `json:"pageId"`
	Number         int             `json:"number"`
	Status         RevisionStatus  `json:"status"`
	Title          string          `json:"title"`
	BodyMD         string          `json:"bodyMd"`
	Steps          []Step          `json:"steps"`
	References     []PageReference `json:"references"`
	BaseRevisionID *string         `json:"baseRevisionId,omitempty"`
	CreatedBy      User            `json:"createdBy"`
	CreatedAt      time.Time       `json:"createdAt"`
	ApprovedAt     *time.Time      `json:"approvedAt,omitempty"`
}

type RevisionSummary struct {
	ID         string         `json:"id"`
	Number     int            `json:"number"`
	Status     RevisionStatus `json:"status"`
	Title      string         `json:"title"`
	CreatedBy  User           `json:"createdBy"`
	CreatedAt  time.Time      `json:"createdAt"`
	ApprovedAt *time.Time     `json:"approvedAt,omitempty"`
}

type Comment struct {
	ID              string    `json:"id"`
	PageID          string    `json:"pageId"`
	RevisionID      string    `json:"revisionId"`
	ParentCommentID *string   `json:"parentCommentId,omitempty"`
	Body            string    `json:"body"`
	Author          User      `json:"author"`
	CreatedAt       time.Time `json:"createdAt"`
	Replies         []Comment `json:"replies"`
}

type Objection struct {
	ID             string     `json:"id"`
	PageID         string     `json:"pageId"`
	RevisionID     string     `json:"revisionId"`
	RevisionNumber int        `json:"revisionNumber"`
	Body           string     `json:"body"`
	Author         User       `json:"author"`
	CreatedAt      time.Time  `json:"createdAt"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
	ResolvedBy     *User      `json:"resolvedBy,omitempty"`
}

type ReviewReadiness string

const (
	ReviewApproved ReviewReadiness = "approved"
	ReviewReady    ReviewReadiness = "ready"
	ReviewBlocked  ReviewReadiness = "blocked"
)

type ReviewBlockerType string

const (
	BlockerObjection     ReviewBlockerType = "objection"
	BlockerParentFeature ReviewBlockerType = "parent_feature"
)

type ReviewBlocker struct {
	ID                   string            `json:"id"`
	Type                 ReviewBlockerType `json:"type"`
	SourceRevisionID     *string           `json:"sourceRevisionId,omitempty"`
	SourceRevisionNumber *int              `json:"sourceRevisionNumber,omitempty"`
	Body                 *string           `json:"body,omitempty"`
	Author               *User             `json:"author,omitempty"`
	RelatedPageID        *string           `json:"relatedPageId,omitempty"`
	RelatedPageTitle     *string           `json:"relatedPageTitle,omitempty"`
}

type RevisionReviewState struct {
	RevisionID string          `json:"revisionId"`
	State      ReviewReadiness `json:"state"`
	Blockers   []ReviewBlocker `json:"blockers"`
}

type PageDetail struct {
	Page             Page                  `json:"page"`
	ApprovedRevision *Revision             `json:"approvedRevision,omitempty"`
	DraftRevision    *Revision             `json:"draftRevision,omitempty"`
	Revisions        []RevisionSummary     `json:"revisions"`
	Comments         []Comment             `json:"comments"`
	Objections       []Objection           `json:"objections"`
	Children         []Page                `json:"children"`
	ReviewStates     []RevisionReviewState `json:"reviewStates"`
	Development      *ScenarioDevelopment  `json:"development,omitempty"`
}

type ScenarioDevelopment struct {
	RevisionID string            `json:"revisionId"`
	Status     DevelopmentStatus `json:"status"`
	Detail     string            `json:"detail"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

type DevelopmentTask struct {
	ScenarioDevelopment
	Scenario Revision `json:"scenario"`
}

type RevisionContent struct {
	Title      string          `json:"title"`
	BodyMD     string          `json:"bodyMd"`
	Steps      []Step          `json:"steps"`
	References []PageReference `json:"references"`
}

type CreatePageInput struct {
	Kind            PageKind              `json:"kind"`
	ConceptKind     *ConceptKind          `json:"conceptKind,omitempty"`
	ParentID        *string               `json:"parentId,omitempty"`
	Slug            string                `json:"slug"`
	Content         RevisionContent       `json:"content"`
	InitialScenario *InitialScenarioInput `json:"initialScenario,omitempty"`
}

type InitialScenarioInput struct {
	Slug    string          `json:"slug"`
	Content RevisionContent `json:"content"`
}

type SaveRevisionInput struct {
	BaseRevisionID string          `json:"baseRevisionId"`
	Content        RevisionContent `json:"content"`
}

type SearchOptions struct {
	Query         string
	Kind          *PageKind
	IncludeDrafts bool
	Limit         int
}

type SearchResult struct {
	Page       Page    `json:"page"`
	RevisionID string  `json:"revisionId"`
	Excerpt    string  `json:"excerpt"`
	Score      float64 `json:"score"`
	Draft      bool    `json:"draft"`
}

type AssistantMode string

const (
	AssistantQA   AssistantMode = "qa"
	AssistantEdit AssistantMode = "edit"
)

type AssistantConversation struct {
	ID                string                     `json:"id"`
	OrganizationID    string                     `json:"-"`
	UserID            string                     `json:"-"`
	QASessionID       *string                    `json:"-"`
	EditSessionID     *string                    `json:"-"`
	PrimaryMode       AssistantMode              `json:"primaryMode"`
	LastMode          AssistantMode              `json:"lastMode"`
	QAHandoffCursor   int                        `json:"-"`
	EditHandoffCursor int                        `json:"-"`
	Title             string                     `json:"title"`
	State             AssistantConversationState `json:"state"`
	Messages          []AssistantMessage         `json:"messages,omitempty"`
	Clarification     *AssistantClarification    `json:"clarification,omitempty"`
	CreatedAt         time.Time                  `json:"createdAt"`
	UpdatedAt         time.Time                  `json:"updatedAt"`
}

type AssistantConversationState string

const (
	AssistantStateIdle                  AssistantConversationState = "idle"
	AssistantStateRunning               AssistantConversationState = "running"
	AssistantStateAwaitingClarification AssistantConversationState = "awaiting_clarification"
	AssistantStateStopped               AssistantConversationState = "stopped"
	AssistantStateError                 AssistantConversationState = "error"
	AssistantStateUnavailable           AssistantConversationState = "unavailable"
)

type AssistantClarification struct {
	RequestID string        `json:"requestId"`
	Mode      AssistantMode `json:"mode"`
	Message   string        `json:"message"`
	Choices   []string      `json:"choices,omitempty"`
}

type AssistantDraftReceipt struct {
	RevisionID string `json:"revisionId"`
	PageID     string `json:"pageId"`
	PageTitle  string `json:"pageTitle"`
}

type AssistantMessage struct {
	ID        string                  `json:"id"`
	Role      string                  `json:"role"`
	Mode      AssistantMode           `json:"mode"`
	Content   string                  `json:"content"`
	Citations []Citation              `json:"citations"`
	Drafts    []AssistantDraftReceipt `json:"drafts"`
	CreatedAt time.Time               `json:"createdAt"`
}

type Citation struct {
	RevisionID string `json:"revisionId"`
	PageID     string `json:"pageId"`
	PageTitle  string `json:"pageTitle"`
	Draft      bool   `json:"draft"`
}

type RetrievedDocument struct {
	RevisionID string  `json:"revisionId"`
	PageID     string  `json:"pageId"`
	PageTitle  string  `json:"pageTitle"`
	Content    string  `json:"content"`
	Draft      bool    `json:"draft"`
	Score      float64 `json:"score"`
}

type AIChangeOperation struct {
	Operation       string          `json:"operation"`
	ClientKey       string          `json:"clientKey,omitempty"`
	PageID          *string         `json:"pageId,omitempty"`
	BaseRevisionID  *string         `json:"baseRevisionId,omitempty"`
	Kind            PageKind        `json:"kind"`
	ConceptKind     *ConceptKind    `json:"conceptKind,omitempty"`
	ParentID        *string         `json:"parentId,omitempty"`
	ParentClientKey string          `json:"parentClientKey,omitempty"`
	Slug            string          `json:"slug"`
	Content         RevisionContent `json:"content"`
}

type AIChangeSet struct {
	Clarification string              `json:"clarification,omitempty"`
	Summary       string              `json:"summary"`
	Operations    []AIChangeOperation `json:"operations"`
}

type AssistantMutationContext struct {
	ConversationID  string
	TurnID          string
	HermesProfile   string
	HermesSessionID string
}

type AuditEvent struct {
	ID         string         `json:"id"`
	Action     string         `json:"action"`
	EntityType string         `json:"entityType"`
	EntityID   *string        `json:"entityId,omitempty"`
	Actor      *User          `json:"actor,omitempty"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"createdAt"`
}
