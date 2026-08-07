import type { components } from './generated'

type Schemas = components['schemas']

export type PageKind = Schemas['PageKind']
export type ConceptKind = Schemas['ConceptKind']
export type RevisionStatus = Schemas['RevisionStatus']
export type DevelopmentStatus = 'queued' | 'running' | 'developed' | 'blocked'
export type AssistantMode = Schemas['AssistantMode']
export type BDDKeyword = Schemas['BDDKeyword']
export type StepRole = Schemas['StepRole']
export type StepDefinition = Schemas['StepDefinition']

export type AssistantConversationState = Schemas['AssistantConversationState']
export type AssistantProfileStatus = Schemas['AssistantProfileStatus']
export type AssistantStatus = Schemas['AssistantStatus']
export type AssistantConversationSummary = Schemas['AssistantConversationSummary']
export type AssistantDraftReceipt = Schemas['AssistantDraftReceipt']
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
  conceptKind?: ConceptKind
  parentId?: string
  slug: string
  title: string
  approvedRevisionId?: string
  latestDraftRevisionId?: string
  approvedRevisionTitle?: string
  draftRevisionTitle?: string
  approved: boolean
  hasDraft: boolean
  unresolvedObjections: number
  createdAt: string
  updatedAt: string
}

export interface Step {
  id?: string
  stableId?: string
  position?: number
  keyword: BDDKeyword
  definitionId?: string
  expression?: string
  arguments?: string[]
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
  approvedAt?: string
}

export interface RevisionSummary {
  id: string
  number: number
  status: RevisionStatus
  title: string
  createdBy: User
  createdAt: string
  approvedAt?: string
}

export interface Comment {
  id: string
  pageId: string
  revisionId: string
  parentCommentId?: string
  body: string
  author: User
  createdAt: string
  replies: Comment[]
}

export interface Objection {
  id: string
  pageId: string
  revisionId: string
  revisionNumber: number
  body: string
  author: User
  createdAt: string
  resolvedAt?: string
  resolvedBy?: User
}

export type ReviewReadiness = 'approved' | 'ready' | 'blocked'
export type ReviewBlockerType = 'objection' | 'parent_feature'

interface ReviewBlockerBase {
  id: string
  type: ReviewBlockerType
}

export interface ObjectionReviewBlocker extends ReviewBlockerBase {
  type: 'objection'
  sourceRevisionId: string
  sourceRevisionNumber: number
  body: string
  author: User
}

export interface ParentFeatureReviewBlocker extends ReviewBlockerBase {
  type: 'parent_feature'
  relatedPageId: string
  relatedPageTitle: string
}

export type ReviewBlocker = ObjectionReviewBlocker | ParentFeatureReviewBlocker

export interface RevisionReviewState {
  revisionId: string
  state: ReviewReadiness
  blockers: ReviewBlocker[]
}

export interface PageDetail {
  page: Page
  approvedRevision?: Revision
  draftRevision?: Revision
  revisions: RevisionSummary[]
  comments: Comment[]
  objections: Objection[]
  children: Page[]
  reviewStates: RevisionReviewState[]
  development?: {
    revisionId: string
    status: DevelopmentStatus
    detail: string
    updatedAt: string
  }
  developmentProgress?: {
    developed: number
    total: number
  }
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
