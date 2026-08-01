import type {
  AssistantMode,
  AssistantOperationReviewValue,
  AssistantCommandAccepted,
  AssistantConversation,
  AssistantConversationSummary,
  AssistantConnectionState,
  AssistantDraftProposal,
  AssistantStatus,
  AssistantStreamEvent,
  AuditEvent,
  Comment,
  Page,
  PageDetail,
  PageKind,
  Revision,
  RevisionContent,
  SearchResult,
  User,
  Vote,
  VoteValue,
} from './types'

export class APIError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'APIError'
    this.status = status
    this.code = code
  }
}

function csrfToken(): string {
  const cookie = document.cookie
    .split('; ')
    .find((value) => value.startsWith('viki_csrf='))
  return cookie ? decodeURIComponent(cookie.split('=').slice(1).join('=')) : ''
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  if (options.body && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }
  if (options.method && !['GET', 'HEAD'].includes(options.method)) {
    headers.set('X-CSRF-Token', csrfToken())
  }
  const response = await fetch(path, { ...options, headers, credentials: 'same-origin' })
  if (!response.ok) {
    let code = 'request_failed'
    let message = document.documentElement.lang === 'en'
      ? `Request failed (${response.status}).`
      : `Požiadavka zlyhala (${response.status}).`
    try {
      const body = await response.json() as { error?: { code?: string; message?: string } }
      code = body.error?.code ?? code
      message = body.error?.message ?? message
    } catch {
      // A non-JSON proxy error keeps the generic user-facing message.
    }
    throw new APIError(response.status, code, localizedErrorMessage(code, message))
  }
  if (response.status === 204) return undefined as T
  const body = await response.text()
  if (!body) return undefined as T
  return JSON.parse(body) as T
}

function localizedErrorMessage(code: string, fallback: string): string {
  if (document.documentElement.lang !== 'en') return fallback
  const messages: Record<string, string> = {
    unauthorized: 'Please sign in again.', csrf_failed: 'The security token is invalid. Refresh the page.',
    invalid_json: 'The request has an invalid format.', not_found: 'The record was not found.',
    revision_conflict: 'Another user changed this page. Refresh the draft and apply your changes again.',
    duplicate_slug: 'A page with this address already exists.', invalid_page: 'The page hierarchy or reference is invalid.',
    rejection_reason_required: 'You must provide a reason when rejecting.', invalid_vote: 'The vote is invalid.',
    unresolved_rejection: 'An unresolved rejection blocks publication.',
    rejected_proposal_dependency: 'An approved change depends on a rejected concept. Reject the dependent change or approve the concept.',
    invalid_mode: 'Select Questions or Edit mode.', invalid_settings: 'Choose a setting to change.',
    assistant_busy: 'Wait for the current message to finish or stop it.', assistant_idle: 'No message is currently running.',
    invalid_message: 'The message must contain between 1 and 12,000 characters.',
    invalid_answer: 'The answer must contain between 1 and 12,000 characters.',
    clarification_mismatch: 'This clarification request is no longer active.',
    management_command_forbidden: 'Assistant management commands are not allowed in viki.',
    assistant_unavailable: 'The viki assistant is currently unavailable.', streaming_unavailable: 'Streaming is unavailable.',
    invalid_operation_review: 'The decision is invalid.', invalid_credentials: 'The email or password is incorrect.',
    login_rate_limited: 'Too many failed attempts. Try again in five minutes.',
    database_unavailable: 'The database is not ready.', internal_error: 'An unexpected error occurred.',
    frontend_unavailable: 'The frontend has not been built yet.', missing_id: 'A required identifier is missing.',
    request_failed: 'The request could not be processed.',
  }
  return messages[code] ?? fallback
}

export const api = {
  login: (email: string, password: string) => request<{ user: User; csrfToken: string }>('/api/v1/auth/login', {
    method: 'POST', body: JSON.stringify({ email, password }),
  }),
  me: () => request<{ user: User }>('/api/v1/auth/me'),
  logout: () => request<void>('/api/v1/auth/logout', { method: 'POST' }),
  pages: (kind?: PageKind) => request<{ pages: Page[] }>(`/api/v1/pages${kind ? `?kind=${kind}` : ''}`),
  search: (query: string, kind?: PageKind, includeDrafts = false) => {
    const params = new URLSearchParams({ q: query, includeDrafts: String(includeDrafts) })
    if (kind) params.set('kind', kind)
    return request<{ results: SearchResult[] }>(`/api/v1/pages?${params}`)
  },
  page: (id: string) => request<PageDetail>(`/api/v1/pages/${id}`),
  revision: (id: string) => request<Revision>(`/api/v1/revisions/${id}`),
  createPage: (input: { kind: PageKind; conceptKind?: 'noun' | 'verb'; parentId?: string; slug: string; content: RevisionContent }) =>
    request<PageDetail>('/api/v1/pages', { method: 'POST', body: JSON.stringify(input) }),
  saveRevision: (pageId: string, baseRevisionId: string, content: RevisionContent) =>
    request<Revision>(`/api/v1/pages/${pageId}/revisions`, { method: 'POST', body: JSON.stringify({ baseRevisionId, content }) }),
  publish: (revisionId: string) => request<PageDetail>(`/api/v1/revisions/${revisionId}/publish`, { method: 'POST' }),
  vote: (revisionId: string, value: VoteValue, reason = '') => request<Vote>(`/api/v1/revisions/${revisionId}/vote`, {
    method: 'PUT', body: JSON.stringify({ value, reason }),
  }),
  comment: (input: { pageId: string; revisionId: string; body: string; parentCommentId?: string; anchorKind?: string; anchorId?: string }) =>
    request<Comment>('/api/v1/comments', { method: 'POST', body: JSON.stringify(input) }),
  resolveComment: (id: string) => request<Comment>(`/api/v1/comments/${id}/resolve`, { method: 'POST' }),
  audit: () => request<{ events: AuditEvent[] }>('/api/v1/audit?limit=80'),
  assistantStatus: () => request<AssistantStatus>('/api/v1/assistant/status'),
  assistantConversations: () => request<{ conversations: AssistantConversationSummary[] }>('/api/v1/assistant/conversations'),
  createAssistantConversation: (primaryMode: AssistantMode = 'qa') => request<AssistantConversation>('/api/v1/assistant/conversations', {
    method: 'POST', body: JSON.stringify({ primaryMode }),
  }),
  assistantConversation: (id: string) => request<AssistantConversation>(`/api/v1/assistant/conversations/${id}`),
  sendAssistantMessage: (id: string, content: string, mode: AssistantMode) => request<AssistantCommandAccepted>(`/api/v1/assistant/conversations/${id}/messages`, {
    method: 'POST', body: JSON.stringify({ content, mode }),
  }),
  stopAssistantConversation: (id: string) => request<void>(`/api/v1/assistant/conversations/${id}/stop`, {
    method: 'POST',
  }),
  respondToAssistantClarification: (id: string, requestId: string, answer: string) => request<void>(`/api/v1/assistant/conversations/${id}/clarifications/${requestId}`, {
    method: 'POST', body: JSON.stringify({ answer }),
  }),
  draftProposals: () => request<{ proposals: AssistantDraftProposal[] }>('/api/v1/draft-proposals'),
  draftProposal: (id: string) => request<AssistantDraftProposal>(`/api/v1/draft-proposals/${id}`),
  reviewDraftProposalOperation: (id: string, operationKey: string, value: AssistantOperationReviewValue, reason = '', cascadeDescendants = false) =>
    request<AssistantDraftProposal>(`/api/v1/draft-proposals/${id}/operations/${encodeURIComponent(operationKey)}/review`, {
      method: 'POST', body: JSON.stringify({ value, ...(reason ? { reason } : {}), ...(cascadeDescendants ? { cascadeDescendants: true } : {}) }),
    }),
  approveDraftProposal: (id: string) => request<AssistantDraftProposal>(`/api/v1/draft-proposals/${id}/approve`, { method: 'POST' }),
  discardDraftProposal: (id: string, reason: string) => request<AssistantDraftProposal>(`/api/v1/draft-proposals/${id}/discard`, {
    method: 'POST', body: JSON.stringify({ reason }),
  }),
}

const assistantEventNames: AssistantStreamEvent['type'][] = [
  'message_delta',
  'activity',
  'citation',
  'draft_created',
  'draft_proposed',
  'draft_published',
  'draft_discarded',
  'clarification',
  'completed',
  'stopped',
  'error',
]

export function openAssistantEventStream(
  conversationId: string,
  onEvent: (event: AssistantStreamEvent) => void,
  onConnection: (state: AssistantConnectionState) => void,
): () => void {
  const source = new EventSource(`/api/v1/assistant/conversations/${encodeURIComponent(conversationId)}/events`, {
    withCredentials: true,
  })
  source.onopen = () => onConnection('connected')
  source.onerror = () => onConnection(source.readyState === EventSource.CLOSED ? 'disconnected' : 'reconnecting')
  for (const type of assistantEventNames) {
    source.addEventListener(type, (raw) => {
      try {
        const message = raw as MessageEvent<string>
        onEvent({ id: message.lastEventId, type, data: JSON.parse(message.data) } as AssistantStreamEvent)
      } catch {
        // Ignore malformed or unsupported transport data. The canonical history
        // is reconciled from Hermes after a completed turn or reconnect.
      }
    })
  }
  return () => source.close()
}
