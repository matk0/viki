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
	RevisionAccepted   RevisionStatus = "accepted"
	RevisionSuperseded RevisionStatus = "superseded"
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
	AcceptedRevisionID    *string      `json:"acceptedRevisionId,omitempty"`
	LatestDraftRevisionID *string      `json:"latestDraftRevisionId,omitempty"`
	Accepted              bool         `json:"accepted"`
	HasDraft              bool         `json:"hasDraft"`
	UnresolvedRejections  int          `json:"unresolvedRejections"`
	CreatedAt             time.Time    `json:"createdAt"`
	UpdatedAt             time.Time    `json:"updatedAt"`
}

type Step struct {
	ID       string     `json:"id"`
	StableID string     `json:"stableId"`
	Position int        `json:"position"`
	Keyword  BDDKeyword `json:"keyword"`
	Text     string     `json:"text"`
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
	Aliases        []string        `json:"aliases"`
	Steps          []Step          `json:"steps"`
	References     []PageReference `json:"references"`
	BaseRevisionID *string         `json:"baseRevisionId,omitempty"`
	CreatedBy      User            `json:"createdBy"`
	CreatedAt      time.Time       `json:"createdAt"`
	AcceptedAt     *time.Time      `json:"acceptedAt,omitempty"`
}

type RevisionSummary struct {
	ID         string         `json:"id"`
	Number     int            `json:"number"`
	Status     RevisionStatus `json:"status"`
	Title      string         `json:"title"`
	CreatedBy  User           `json:"createdBy"`
	CreatedAt  time.Time      `json:"createdAt"`
	AcceptedAt *time.Time     `json:"acceptedAt,omitempty"`
}

type Comment struct {
	ID              string     `json:"id"`
	PageID          string     `json:"pageId"`
	RevisionID      string     `json:"revisionId"`
	ParentCommentID *string    `json:"parentCommentId,omitempty"`
	AnchorKind      *string    `json:"anchorKind,omitempty"`
	AnchorID        *string    `json:"anchorId,omitempty"`
	Body            string     `json:"body"`
	Blocking        bool       `json:"blocking"`
	Author          User       `json:"author"`
	CreatedAt       time.Time  `json:"createdAt"`
	ResolvedAt      *time.Time `json:"resolvedAt,omitempty"`
	ResolvedBy      *User      `json:"resolvedBy,omitempty"`
	Replies         []Comment  `json:"replies"`
}

type Vote struct {
	RevisionID string    `json:"revisionId"`
	Value      string    `json:"value"`
	User       User      `json:"user"`
	CommentID  *string   `json:"commentId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type PageDetail struct {
	Page             Page              `json:"page"`
	AcceptedRevision *Revision         `json:"acceptedRevision,omitempty"`
	DraftRevision    *Revision         `json:"draftRevision,omitempty"`
	Revisions        []RevisionSummary `json:"revisions"`
	Comments         []Comment         `json:"comments"`
	Votes            []Vote            `json:"votes"`
	Children         []Page            `json:"children"`
}

type RevisionContent struct {
	Title      string          `json:"title"`
	BodyMD     string          `json:"bodyMd"`
	Aliases    []string        `json:"aliases"`
	Steps      []Step          `json:"steps"`
	References []PageReference `json:"references"`
}

type CreatePageInput struct {
	Kind        PageKind        `json:"kind"`
	ConceptKind *ConceptKind    `json:"conceptKind,omitempty"`
	ParentID    *string         `json:"parentId,omitempty"`
	Slug        string          `json:"slug"`
	Content     RevisionContent `json:"content"`
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

type AssistantDraftProposalStatus string

const (
	AssistantProposalAwaitingApproval AssistantDraftProposalStatus = "awaiting_approval"
	AssistantProposalPublished        AssistantDraftProposalStatus = "published"
	AssistantProposalDiscarded        AssistantDraftProposalStatus = "discarded"
)

type AssistantOperationReviewValue string

const (
	AssistantReviewApprove AssistantOperationReviewValue = "approve"
	AssistantReviewReject  AssistantOperationReviewValue = "reject"
)

type AssistantOperationReview struct {
	OperationKey string                        `json:"operationKey"`
	Value        AssistantOperationReviewValue `json:"value"`
	Reason       string                        `json:"reason,omitempty"`
	ReviewedAt   time.Time                     `json:"reviewedAt"`
}

type AssistantDraftProposal struct {
	ID                 string                       `json:"id"`
	ConversationID     string                       `json:"conversationId"`
	TurnID             string                       `json:"turnId"`
	Summary            string                       `json:"summary"`
	Operations         []AIChangeOperation          `json:"operations"`
	OperationReviews   []AssistantOperationReview   `json:"operationReviews"`
	Status             AssistantDraftProposalStatus `json:"status"`
	RejectionReason    string                       `json:"rejectionReason,omitempty"`
	PublishedRevisions []Revision                   `json:"publishedRevisions"`
	CreatedAt          time.Time                    `json:"createdAt"`
	UpdatedAt          time.Time                    `json:"updatedAt"`
	PublishedAt        *time.Time                   `json:"publishedAt,omitempty"`
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
