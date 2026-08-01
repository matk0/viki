import { afterEach, describe, expect, it, vi } from 'vitest'
import { APIError, api, openAssistantEventStream } from './client'

describe('assistant API client', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('loads Hermes profile availability from the assistant status endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      available: true,
      qa: { mode: 'qa', connected: true, configured: true, ready: true },
      edit: { mode: 'edit', connected: true, configured: true, ready: true },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.assistantStatus()).resolves.toMatchObject({ available: true })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/assistant/status', expect.objectContaining({
      credentials: 'same-origin',
    }))
  })

  it('routes conversation commands through the versioned assistant endpoints', async () => {
    const conversation = {
      id: 'conversation-1',
      primaryMode: 'qa',
      lastMode: 'edit',
      state: 'idle',
      messages: [],
      createdAt: '2026-07-30T10:00:00Z',
      updatedAt: '2026-07-30T10:00:00Z',
    }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ conversations: [conversation] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(conversation), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(conversation), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ turnId: 'turn-1', mode: 'edit' }), { status: 202 }))
      .mockResolvedValueOnce(new Response(null, { status: 202 }))
      .mockResolvedValueOnce(new Response(null, { status: 202 }))
    vi.stubGlobal('fetch', fetchMock)

    await api.assistantConversations()
    await api.createAssistantConversation('edit')
    await api.assistantConversation('conversation-1')
    await api.sendAssistantMessage('conversation-1', 'Vytvor koncept.', 'edit')
    await api.stopAssistantConversation('conversation-1')
    await api.respondToAssistantClarification('conversation-1', 'request-1', 'Pre domácnosť.')

    expect(fetchMock.mock.calls.map(([path]) => path)).toEqual([
      '/api/v1/assistant/conversations',
      '/api/v1/assistant/conversations',
      '/api/v1/assistant/conversations/conversation-1',
      '/api/v1/assistant/conversations/conversation-1/messages',
      '/api/v1/assistant/conversations/conversation-1/stop',
      '/api/v1/assistant/conversations/conversation-1/clarifications/request-1',
    ])
    expect(fetchMock.mock.calls[1][1]).toMatchObject({ method: 'POST', body: JSON.stringify({ primaryMode: 'edit' }) })
    expect(fetchMock.mock.calls[3][1]).toMatchObject({ method: 'POST', body: JSON.stringify({ content: 'Vytvor koncept.', mode: 'edit' }) })
    expect(fetchMock.mock.calls[5][1]).toMatchObject({ method: 'POST', body: JSON.stringify({ answer: 'Pre domácnosť.' }) })
  })

  it('subscribes only to the safe public assistant event vocabulary', () => {
    class FakeEventSource {
      static CLOSED = 2
      static instances: FakeEventSource[] = []
      readyState = 1
      onopen: (() => void) | null = null
      onerror: (() => void) | null = null
      listeners = new Map<string, (event: MessageEvent) => void>()
      close = vi.fn()

      constructor(public url: string, public options: EventSourceInit) {
        FakeEventSource.instances.push(this)
      }

      addEventListener(name: string, listener: EventListenerOrEventListenerObject) {
        this.listeners.set(name, listener as (event: MessageEvent) => void)
      }

      emit(name: string, data: unknown) {
        this.listeners.get(name)?.(new MessageEvent(name, { data: JSON.stringify(data), lastEventId: '17' }))
      }
    }
    vi.stubGlobal('EventSource', FakeEventSource)
    const onEvent = vi.fn()
    const onConnection = vi.fn()

    const close = openAssistantEventStream('conversation-1', onEvent, onConnection)
    const source = FakeEventSource.instances[0]
    source.onopen?.()
    source.emit('message_delta', { turnId: 'turn-1', mode: 'qa', delta: 'Dobrý deň' })
    source.emit('tool.start', { arguments: { secret: 'must-not-leak' } })

    expect(source.url).toBe('/api/v1/assistant/conversations/conversation-1/events')
    expect(source.options).toEqual({ withCredentials: true })
    expect(onConnection).toHaveBeenCalledWith('connected')
    expect(onEvent).toHaveBeenCalledOnce()
    expect(onEvent).toHaveBeenCalledWith(expect.objectContaining({
      type: 'message_delta',
      id: '17',
      data: expect.objectContaining({ delta: 'Dobrý deň' }),
    }))

    close()
    expect(source.close).toHaveBeenCalledOnce()
  })

  it('covers every wiki endpoint wrapper and request serialization rule', async () => {
    document.cookie = 'viki_csrf=csrf%20token; path=/'
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response('{}', { status: 200 })))
    vi.stubGlobal('fetch', fetchMock)

    await api.login('matej@example.com', 'password')
    await api.me()
    await api.logout()
    await api.pages()
    await api.pages('concept')
    await api.search('query')
    await api.search('query', 'feature', true)
    await api.page('page 1')
    await api.revision('revision-1')
    await api.createPage({ kind: 'concept', conceptKind: 'noun', slug: 'zmluva', content: { title: 'Zmluva', bodyMd: '', aliases: [], steps: [], references: [] } })
    await api.saveRevision('page-1', 'revision-1', { title: 'Zmluva', bodyMd: 'Body', aliases: [], steps: [], references: [] })
    await api.publish('revision-2')
    await api.vote('revision-2', 'approve')
    await api.vote('revision-2', 'reject', 'Reason')
    await api.comment({ pageId: 'page-1', revisionId: 'revision-2', body: 'Comment', parentCommentId: 'comment-1', anchorKind: 'field', anchorId: 'bodyMd' })
    await api.resolveComment('comment-1')
    await api.audit()
    await api.assistantStatus()
    await api.assistantConversations()
    await api.createAssistantConversation()
    await api.assistantConversation('conversation-1')
    await api.sendAssistantMessage('conversation-1', 'Question', 'qa')
    await api.stopAssistantConversation('conversation-1')
    await api.respondToAssistantClarification('conversation-1', 'request-1', 'Answer')
    await api.draftProposals()
    await api.draftProposal('proposal-1')
    await api.reviewDraftProposalOperation('proposal-1', 'concept / one', 'approve')
    await api.reviewDraftProposalOperation('proposal-1', 'concept', 'reject', 'Reason', true)
    await api.approveDraftProposal('proposal-1')
    await api.discardDraftProposal('proposal-1', 'Reason')

    const calls = fetchMock.mock.calls as Array<[string, RequestInit]>
    expect(calls.map(([path]) => path)).toContain('/api/v1/pages')
    expect(calls.map(([path]) => path)).toContain('/api/v1/pages?kind=concept')
    expect(calls.map(([path]) => path)).toContain('/api/v1/pages?q=query&includeDrafts=true&kind=feature')
    expect(calls.map(([path]) => path)).toContain('/api/v1/draft-proposals/proposal-1/operations/concept%20%2F%20one/review')
    expect(calls[0][1].headers).toEqual(expect.objectContaining({}))
    const loginHeaders = calls[0][1].headers as Headers
    expect(loginHeaders.get('Content-Type')).toBe('application/json')
    expect(loginHeaders.get('X-CSRF-Token')).toBe('csrf token')
    expect(calls.every(([, options]) => options.credentials === 'same-origin')).toBe(true)
    expect(calls.at(-3)?.[1].body).toBe(JSON.stringify({ value: 'reject', reason: 'Reason', cascadeDescendants: true }))
  })

  it('handles no-content, empty-body, JSON errors, proxy errors, and English localization', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response('', { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { code: 'duplicate_slug', message: 'fallback' } }), { status: 409 }))
      .mockResolvedValueOnce(new Response('proxy failure', { status: 502 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { code: 'unknown_code', message: 'Server fallback' } }), { status: 400 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: {} }), { status: 400 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.logout()).resolves.toBeUndefined()
    await expect(api.me()).resolves.toBeUndefined()
    document.documentElement.lang = 'en'
    await expect(api.createPage({ kind: 'feature', slug: 'duplicate', content: { title: 'Duplicate', bodyMd: '', aliases: [], steps: [], references: [] } }))
      .rejects.toMatchObject({ status: 409, code: 'duplicate_slug', message: 'A page with this address already exists.' })
    await expect(api.me()).rejects.toMatchObject({ status: 502, code: 'request_failed', message: 'The request could not be processed.' })
    await expect(api.me()).rejects.toMatchObject({ status: 400, code: 'unknown_code', message: 'Server fallback' })
    await expect(api.me()).rejects.toMatchObject({ status: 400, code: 'request_failed', message: 'The request could not be processed.' })
    document.documentElement.lang = 'sk'
    vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify({ error: { code: 'unknown_code', message: 'Slovenská správa' } }), { status: 400 }))
    await expect(api.me()).rejects.toMatchObject({ status: 400, code: 'unknown_code', message: 'Slovenská správa' })
  })

  it('constructs API errors with stable public fields', () => {
    const error = new APIError(409, 'revision_conflict', 'Conflict')
    expect(error).toBeInstanceOf(Error)
    expect(error.name).toBe('APIError')
    expect(error.status).toBe(409)
    expect(error.code).toBe('revision_conflict')
  })

  it('reports both reconnect states and ignores malformed public events', () => {
    class FakeEventSource {
      static CLOSED = 2
      static instance: FakeEventSource
      readyState = 0
      onopen: (() => void) | null = null
      onerror: (() => void) | null = null
      listeners = new Map<string, (event: MessageEvent) => void>()
      close = vi.fn()

      constructor() { FakeEventSource.instance = this }
      addEventListener(name: string, listener: EventListenerOrEventListenerObject) {
        this.listeners.set(name, listener as (event: MessageEvent) => void)
      }
    }
    vi.stubGlobal('EventSource', FakeEventSource)
    const onEvent = vi.fn()
    const onConnection = vi.fn()
    openAssistantEventStream('conversation / one', onEvent, onConnection)
    const source = FakeEventSource.instance
    source.onerror?.()
    source.readyState = FakeEventSource.CLOSED
    source.onerror?.()
    source.listeners.get('error')?.(new MessageEvent('error', { data: 'not-json' }))
    expect(onConnection).toHaveBeenNthCalledWith(1, 'reconnecting')
    expect(onConnection).toHaveBeenNthCalledWith(2, 'disconnected')
    expect(onEvent).not.toHaveBeenCalled()
  })
})
