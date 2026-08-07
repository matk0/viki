import type {
  AssistantMode,
  AssistantCommandAccepted,
  AssistantConversation,
  AssistantConversationSummary,
  AssistantConnectionState,
  AssistantStatus,
  AssistantStreamEvent,
  AuditEvent,
  Comment,
  Objection,
  Page,
  PageDetail,
  PageKind,
  Revision,
  RevisionContent,
  SearchResult,
  StepDefinition,
  StepRole,
  User,
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
  if (code === 'parent_feature_not_approved') {
    return document.documentElement.lang === 'en'
      ? 'Approve the parent feature before approving this scenario.'
      : 'Pred schválením scenára najprv schváľte nadradenú funkciu.'
  }
  if (document.documentElement.lang !== 'en') return fallback
  const messages: Record<string, string> = {
    unauthorized: 'Please sign in again.', csrf_failed: 'The security token is invalid. Refresh the page.',
    invalid_json: 'The request has an invalid format.', not_found: 'The record was not found.',
    revision_conflict: 'Another user changed this page. Refresh the draft and apply your changes again.',
    duplicate_slug: 'A page with this address already exists.', invalid_page: 'The page hierarchy or reference is invalid.',
    objection_reason_required: 'You must provide a reason when raising an objection.',
    unresolved_objection: 'An unresolved objection blocks approval.',
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
  stepDefinitions: (query = '', role?: StepRole) => {
    const params = new URLSearchParams()
    if (query) params.set('q', query)
    if (role) params.set('role', role)
    const suffix = params.size > 0 ? `?${params}` : ''
    return request<{ definitions: StepDefinition[] }>(`/api/v1/step-definitions${suffix}`)
  },
  page: (id: string) => request<PageDetail>(`/api/v1/pages/${id}`),
  revision: (id: string) => request<Revision>(`/api/v1/revisions/${id}`),
  createPage: (input: { kind: PageKind; conceptKind?: 'noun' | 'verb'; parentId?: string; slug: string; content: RevisionContent; initialScenario?: { slug: string; content: RevisionContent } }) =>
    request<PageDetail>('/api/v1/pages', { method: 'POST', body: JSON.stringify(input) }),
  saveRevision: (pageId: string, baseRevisionId: string, content: RevisionContent) =>
    request<Revision>(`/api/v1/pages/${pageId}/revisions`, { method: 'POST', body: JSON.stringify({ baseRevisionId, content }) }),
  approve: (revisionId: string) => request<PageDetail>(`/api/v1/revisions/${revisionId}/approve`, { method: 'POST' }),
  raiseObjection: (revisionId: string, reason: string) => request<Objection>(`/api/v1/revisions/${revisionId}/objections`, {
    method: 'POST', body: JSON.stringify({ reason }),
  }),
  comment: (input: { pageId: string; revisionId: string; body: string; parentCommentId?: string }) =>
    request<Comment>('/api/v1/comments', { method: 'POST', body: JSON.stringify(input) }),
  resolveObjection: (id: string) => request<Objection>(`/api/v1/objections/${id}/resolve`, { method: 'POST' }),
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
}

const assistantEventNames: AssistantStreamEvent['type'][] = [
  'message_delta',
  'activity',
  'citation',
  'draft_created',
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
