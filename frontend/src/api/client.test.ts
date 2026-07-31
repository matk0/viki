import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, openAssistantEventStream } from './client'

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
    await api.sendAssistantMessage('conversation-1', 'Vytvor pojem.', 'edit')
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
    expect(fetchMock.mock.calls[3][1]).toMatchObject({ method: 'POST', body: JSON.stringify({ content: 'Vytvor pojem.', mode: 'edit' }) })
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
})
