import type { components } from './generated'

type Schemas = components['schemas']

export type PageKind = 'primitive' | 'scenario' | 'subscenario'
export type PrimitiveKind = 'noun' | 'verb'
export type RevisionStatus = 'draft' | 'accepted' | 'superseded'
export type VoteValue = 'approve' | 'reject'
export type AssistantMode = Schemas['AssistantMode']
export type BDDKeyword = 'given' | 'when' | 'then' | 'and' | 'but'

export type AssistantConversationState = Schemas['AssistantConversationState']
export type AssistantProfileStatus = Schemas['AssistantProfileStatus']
export type AssistantStatus = Schemas['AssistantStatus']
export type AssistantConversationSummary = Schemas['AssistantConversationSummary']
export type AssistantDraftReceipt = Schemas['AssistantDraftReceipt']
export type AssistantChangeOperation = Schemas['AssistantChangeOperation']
export type AssistantDraftProposal = Schemas['AssistantDraftProposal']
export type AssistantDraftProposalStatus = Schemas['AssistantDraftProposalStatus']
export type AssistantMessage = Schemas['AssistantMessage']
export type AssistantClarification = Schemas['AssistantClarification']
export type AssistantConversation = Schemas['AssistantConversation']
export type AssistantCommandAccepted = Schemas['AssistantTurnAccepted']

export type AssistantConnectionState = 'connecting' | 'connected' | 'reconnecting' | 'disconnected'

export type AssistantStreamEvent =
  | { id: string; type: 'message_delta'; data: { turnId: string; mode: AssistantMode; delta: string } }
  | { id: string; type: 'activity'; data: { turnId: string; mode: AssistantMode; state: string; label: string } }
  | { id: string; type: 'citation'; data: { turnId: string; mode: AssistantMode; citation: Citation } }
  | { id: string; type: 'draft_created'; data: { turnId: string; mode: AssistantMode; draft: AssistantDraftReceipt } }
  | { id: string; type: 'draft_proposed'; data: { turnId: string; mode: AssistantMode; proposal: AssistantDraftProposal } }
  | { id: string; type: 'draft_published'; data: { turnId: string; mode: AssistantMode; proposal: AssistantDraftProposal } }
  | { id: string; type: 'draft_discarded'; data: { turnId: string; mode: AssistantMode; proposal: AssistantDraftProposal } }
  | { id: string; type: 'clarification'; data: { turnId: string; mode: AssistantMode; requestId: string; message: string; choices?: string[] } }
  | { id: string; type: 'completed'; data: { turnId: string; mode: AssistantMode } }
  | { id: string; type: 'stopped'; data: { turnId: string; mode: AssistantMode } }
  | { id: string; type: 'error'; data: { turnId?: string; mode?: AssistantMode; code: string; message: string } }

export interface User {
  id: string
  email: string
  displayName: string
  createdAt: string
}

export interface Page {
  id: string
  kind: PageKind
  primitiveKind?: PrimitiveKind
  parentId?: string
  slug: string
  title: string
  acceptedRevisionId?: string
  latestDraftRevisionId?: string
  accepted: boolean
  hasDraft: boolean
  unresolvedRejections: number
  createdAt: string
  updatedAt: string
}

export interface Step {
  id?: string
  stableId?: string
  position?: number
  keyword: BDDKeyword
  text: string
}

export interface PageReference {
  targetPageId: string
  targetTitle?: string
  relation: string
}

export interface RevisionContent {
  title: string
  bodyMd: string
  aliases: string[]
  steps: Step[]
  references: PageReference[]
}

export interface Revision extends RevisionContent {
  id: string
  pageId: string
  number: number
  status: RevisionStatus
  baseRevisionId?: string
  createdBy: User
  createdAt: string
  acceptedAt?: string
}

export interface RevisionSummary {
  id: string
  number: number
  status: RevisionStatus
  title: string
  createdBy: User
  createdAt: string
  acceptedAt?: string
}

export interface Comment {
  id: string
  pageId: string
  revisionId: string
  parentCommentId?: string
  anchorKind?: string
  anchorId?: string
  body: string
  blocking: boolean
  author: User
  createdAt: string
  resolvedAt?: string
  resolvedBy?: User
  replies: Comment[]
}

export interface Vote {
  revisionId: string
  value: VoteValue
  user: User
  commentId?: string
  createdAt: string
}

export interface PageDetail {
  page: Page
  acceptedRevision?: Revision
  draftRevision?: Revision
  revisions: RevisionSummary[]
  comments: Comment[]
  votes: Vote[]
  children: Page[]
}

export type Citation = Schemas['Citation']

export interface AuditEvent {
  id: string
  action: string
  entityType: string
  entityId?: string
  actor?: User
  metadata: Record<string, unknown>
  createdAt: string
}

export interface SearchResult {
  page: Page
  revisionId: string
  excerpt: string
  score: number
  draft: boolean
}

export interface APIErrorBody {
  error: { code: string; message: string }
}
