import {
  createContext,
  type FormEvent,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { api, openAssistantEventStream } from './api/client'
import { useRouter } from './router'
import type {
  AssistantConnectionState,
  AssistantConversation,
  AssistantConversationSummary,
  AssistantDraftProposal,
  AssistantDraftReceipt,
  AssistantClarification,
  AssistantMessage,
  AssistantMode,
  AssistantStatus,
  AssistantStreamEvent,
  Citation,
} from './api/types'
import { useSlovakVoiceInput } from './voice'
import { useWorkspace } from './workspace'
import { useI18n } from './i18n'

interface Activity {
  state: string
  mode: AssistantMode
}

type PendingClarification = AssistantClarification & { choices?: string[]; turnId?: string }

interface AssistantValue {
  status: AssistantStatus | null
  conversations: AssistantConversationSummary[]
  conversation: AssistantConversation | null
  loading: boolean
  mode: AssistantMode
  composer: string
  connection: AssistantConnectionState
  activity: Activity | null
  proposals: Record<string, AssistantDraftProposal>
  clarification: PendingClarification | null
  clarificationResponse: string
  error: string
  modeAvailable: boolean
  voice: ReturnType<typeof useSlovakVoiceInput>
  setComposer: (value: string) => void
  setMode: (mode: AssistantMode) => void
  setClarificationResponse: (value: string) => void
  selectConversation: (id: string) => Promise<void>
  createConversation: () => Promise<void>
  send: (event?: FormEvent) => Promise<void>
  stop: () => Promise<void>
  respondToClarification: (event?: FormEvent) => Promise<void>
  reconnect: () => void
  refresh: () => Promise<void>
}

const AssistantContext = createContext<AssistantValue | null>(null)

const unavailableStatus: AssistantStatus = {
  available: false,
  qa: { mode: 'qa', connected: false, configured: false, ready: false },
  edit: { mode: 'edit', connected: false, configured: false, ready: false },
}

function isConversationActive(value: AssistantConversation | null | undefined): boolean {
  return value?.state === 'running' || value?.state === 'awaiting_clarification'
}

function normalizeConversation(value: AssistantConversation): AssistantConversation {
  return {
    ...value,
    messages: (value.messages ?? []).map((message) => ({
      ...message,
      citations: message.citations ?? [],
      drafts: message.drafts ?? [],
    })),
  }
}

function summary(value: AssistantConversation): AssistantConversationSummary {
  const { messages: _messages, clarification: _clarification, ...result } = value
  return result
}

function upsertAssistantMessage(
  conversation: AssistantConversation,
  turnId: string,
  mode: AssistantMode,
  update: (message: AssistantMessage) => AssistantMessage,
): AssistantConversation {
  const id = `turn-${turnId}`
  const index = conversation.messages.findIndex((message) => message.id === id)
  const message = index >= 0
    ? conversation.messages[index]
    : {
        id,
        role: 'assistant' as const,
        mode,
        content: '',
        citations: [],
        drafts: [],
        createdAt: new Date().toISOString(),
      }
  const next = update(message)
  const messages = index >= 0
    ? conversation.messages.map((item, position) => position === index ? next : item)
    : [...conversation.messages, next]
  return { ...conversation, state: 'running', lastMode: mode, messages }
}

function appendCitation(message: AssistantMessage, citation: Citation): AssistantMessage {
  if (message.citations.some((item) => item.revisionId === citation.revisionId)) return message
  return { ...message, citations: [...message.citations, citation] }
}

function appendDraft(message: AssistantMessage, draft: AssistantDraftReceipt): AssistantMessage {
  if (message.drafts.some((item) => item.revisionId === draft.revisionId)) return message
  return { ...message, drafts: [...message.drafts, draft] }
}

function isManagementCommand(content: string): boolean {
  return content.trimStart().startsWith('/')
}

export function AssistantProvider({ children }: { children: ReactNode }) {
  const { t } = useI18n()
  const { reloadPages } = useWorkspace()
  const { navigate } = useRouter()
  const [status, setStatus] = useState<AssistantStatus | null>(null)
  const [conversations, setConversations] = useState<AssistantConversationSummary[]>([])
  const [conversation, setConversation] = useState<AssistantConversation | null>(null)
  const [loading, setLoading] = useState(true)
  const [mode, setModeState] = useState<AssistantMode>('qa')
  const [composer, setComposer] = useState('')
  const [connection, setConnection] = useState<AssistantConnectionState>('connecting')
  const [activity, setActivity] = useState<Activity | null>(null)
  const [proposals, setProposals] = useState<Record<string, AssistantDraftProposal>>({})
  const [clarification, setClarification] = useState<PendingClarification | null>(null)
  const [clarificationResponse, setClarificationResponse] = useState('')
  const [error, setError] = useState('')
  const [streamGeneration, setStreamGeneration] = useState(0)
  const initialized = useRef(false)
  const connectionState = useRef<AssistantConnectionState>('connecting')
  const streamedConversationId = useRef<string | null>(null)

  const rememberConversation = useCallback((next: AssistantConversation) => {
    const normalized = normalizeConversation(next)
    setConversation(normalized)
    setClarification(normalized.clarification ?? null)
    setConversations((items) => {
      const entry = summary(normalized)
      const exists = items.some((item) => item.id === entry.id)
      return exists
        ? items.map((item) => item.id === entry.id ? entry : item)
        : [entry, ...items]
    })
    return normalized
  }, [])

  const loadConversation = useCallback(async (id: string) => {
    const next = rememberConversation(await api.assistantConversation(id))
    setModeState(next.lastMode)
    return next
  }, [rememberConversation])

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    let nextStatus = unavailableStatus
    try {
      nextStatus = await api.assistantStatus()
    } catch {
      // Conversation history remains useful even while Hermes is unavailable.
    }
    setStatus(nextStatus)
    try {
      const result = await api.assistantConversations()
      setConversations(result.conversations)
      const selected = conversation && result.conversations.some((item) => item.id === conversation.id)
        ? conversation.id
        : result.conversations[0]?.id
      if (selected) {
        await loadConversation(selected)
      } else if (nextStatus[mode].ready) {
        const next = rememberConversation(await api.createAssistantConversation(mode))
        setModeState(next.lastMode)
      } else {
        setConversation(null)
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t('assistant.loadConversationsFailed'))
    } finally {
      setLoading(false)
    }
  }, [conversation, loadConversation, mode, rememberConversation, t])

  useEffect(() => {
    if (initialized.current) return
    initialized.current = true
    void refresh()
  }, [refresh])

  const handleStreamEvent = useCallback((event: AssistantStreamEvent, conversationId: string) => {
    if (event.type === 'activity') {
      setActivity({ state: event.data.state, mode: event.data.mode })
      setConversation((current) => current?.id === conversationId ? { ...current, state: 'running' } : current)
      return
    }
    if (event.type === 'message_delta') {
      setActivity({ state: 'thinking', mode: event.data.mode })
      setConversation((current) => current?.id === conversationId
        ? upsertAssistantMessage(current, event.data.turnId, event.data.mode, (message) => ({
            ...message,
            content: message.content + event.data.delta,
          }))
        : current)
      return
    }
    if (event.type === 'citation') {
      setConversation((current) => current?.id === conversationId
        ? upsertAssistantMessage(current, event.data.turnId, event.data.mode, (message) => appendCitation(message, event.data.citation))
        : current)
      return
    }
    if (event.type === 'draft_created') {
      setConversation((current) => current?.id === conversationId
        ? upsertAssistantMessage(current, event.data.turnId, event.data.mode, (message) => appendDraft(message, event.data.draft))
        : current)
      void Promise.resolve(reloadPages())
      return
    }
    if (event.type === 'draft_proposed' || event.type === 'draft_published' || event.type === 'draft_discarded') {
      const proposal = event.data.proposal
      setProposals((current) => ({ ...current, [proposal.id]: proposal, [proposal.turnId]: proposal }))
      if (event.type === 'draft_proposed') {
        setActivity({ state: 'awaiting_approval', mode: event.data.mode })
      } else {
        void Promise.resolve(reloadPages())
      }
      return
    }
    if (event.type === 'clarification') {
      setClarification({ turnId: event.data.turnId, requestId: event.data.requestId, mode: event.data.mode, message: event.data.message, choices: event.data.choices })
      setActivity({ state: 'clarifying', mode: event.data.mode })
      setConversation((current) => current?.id === conversationId ? { ...current, state: 'awaiting_clarification' } : current)
      return
    }
    if (event.type === 'error') {
      setError(event.data.message)
      setActivity(null)
      setConversation((current) => current?.id === conversationId ? { ...current, state: 'error' } : current)
      return
    }
    setActivity(null)
    setClarification(null)
    setClarificationResponse('')
    setConversation((current) => current?.id === conversationId ? { ...current, state: event.type === 'stopped' ? 'stopped' : 'idle' } : current)
    void loadConversation(conversationId).catch(() => {
      setConnection('reconnecting')
    })
    if (event.type === 'completed') void Promise.resolve(reloadPages())
  }, [loadConversation, reloadPages])

  useEffect(() => {
    if (!conversation) {
      streamedConversationId.current = null
      connectionState.current = 'disconnected'
      setConnection('disconnected')
      return
    }
    if (streamedConversationId.current !== conversation.id) {
      streamedConversationId.current = conversation.id
      connectionState.current = 'connecting'
    }
    setConnection('connecting')
    const conversationId = conversation.id
    try {
      return openAssistantEventStream(
        conversationId,
        (event) => handleStreamEvent(event, conversationId),
        (next) => {
          const previous = connectionState.current
          connectionState.current = next
          setConnection(next)
          if (next === 'connected' && (previous === 'reconnecting' || previous === 'disconnected')) {
            void loadConversation(conversationId).catch(() => {
              connectionState.current = 'reconnecting'
              setConnection('reconnecting')
            })
          }
        },
      )
    } catch {
      connectionState.current = 'disconnected'
      setConnection('disconnected')
    }
  }, [conversation?.id, handleStreamEvent, loadConversation, streamGeneration])

  const selectConversation = useCallback(async (id: string) => {
    if (id === conversation?.id || isConversationActive(conversation)) return
    setLoading(true)
    setError('')
    setClarification(null)
    try {
      await loadConversation(id)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t('assistant.loadConversationFailed'))
    } finally {
      setLoading(false)
    }
  }, [conversation, loadConversation, t])

  const createConversation = useCallback(async () => {
    if (isConversationActive(conversation) || !status?.[mode]?.ready) return
    setError('')
    try {
      const next = rememberConversation(await api.createAssistantConversation(mode))
      setModeState(next.lastMode)
      setComposer('')
      setClarification(null)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t('assistant.createConversationFailed'))
    }
  }, [conversation, mode, rememberConversation, status, t])

  const setMode = useCallback((next: AssistantMode) => {
    if (isConversationActive(conversation)) return
    setModeState(next)
    setError('')
  }, [conversation])

  const modeAvailable = Boolean(status?.[mode]?.ready)
  const voice = useSlovakVoiceInput(composer, setComposer, isConversationActive(conversation) || !modeAvailable, t)

  const send = useCallback(async (event?: FormEvent) => {
    event?.preventDefault()
    const content = composer.trim()
    if (!conversation || !content || isConversationActive(conversation) || !modeAvailable) return
    if (isManagementCommand(content)) {
      setError(t('assistant.managementForbidden'))
      return
    }
    const optimisticId = `local-${Date.now()}`
    const optimistic: AssistantMessage = {
      id: optimisticId,
      role: 'user',
      mode,
      content,
      citations: [],
      drafts: [],
      createdAt: new Date().toISOString(),
    }
    setConversation((current) => current?.id === conversation.id
      ? { ...current, state: 'running', lastMode: mode, messages: [...current.messages, optimistic] }
      : current)
    setComposer('')
    setError('')
    setActivity({ state: 'submitting', mode })
    try {
      const accepted = await api.sendAssistantMessage(conversation.id, content, mode)
      if (mode === 'edit') navigate(`/drafts/${encodeURIComponent(accepted.turnId)}`)
    } catch (reason) {
      setConversation((current) => current?.id === conversation.id
        ? { ...current, state: 'idle', messages: current.messages.filter((message) => message.id !== optimisticId) }
        : current)
      setComposer(content)
      setActivity(null)
      setError(reason instanceof Error ? reason.message : t('assistant.sendFailed'))
    }
  }, [composer, conversation, mode, modeAvailable, navigate, t])

  const stop = useCallback(async () => {
    if (!conversation || !isConversationActive(conversation)) return
    setActivity({ state: 'stopping', mode: conversation.lastMode })
    try {
      await api.stopAssistantConversation(conversation.id)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t('assistant.stopFailed'))
    }
  }, [conversation, t])

  const respondToClarification = useCallback(async (event?: FormEvent) => {
    event?.preventDefault()
    const response = clarificationResponse.trim()
    if (!conversation || !clarification || !response) return
    const pending = clarification
    setClarification(null)
    setClarificationResponse('')
    setActivity({ state: 'submitting', mode: pending.mode })
    try {
      await api.respondToAssistantClarification(conversation.id, pending.requestId, response)
    } catch (reason) {
      setClarification(pending)
      setClarificationResponse(response)
      setError(reason instanceof Error ? reason.message : t('assistant.clarificationFailed'))
    }
  }, [clarification, clarificationResponse, conversation, t])

  const reconnect = useCallback(() => setStreamGeneration((value) => value + 1), [])

  const value = useMemo<AssistantValue>(() => ({
    status,
    conversations,
    conversation,
    loading,
    mode,
    composer,
    connection,
    activity,
    proposals,
    clarification,
    clarificationResponse,
    error,
    modeAvailable,
    voice,
    setComposer,
    setMode,
    setClarificationResponse,
    selectConversation,
    createConversation,
    send,
    stop,
    respondToClarification,
    reconnect,
    refresh,
  }), [
    activity,
    clarification,
    clarificationResponse,
    composer,
    connection,
    conversation,
    conversations,
    createConversation,
    error,
    loading,
    mode,
    modeAvailable,
    proposals,
    reconnect,
    refresh,
    respondToClarification,
    selectConversation,
    send,
    status,
    stop,
    voice,
  ])

  return <AssistantContext.Provider value={value}>{children}</AssistantContext.Provider>
}

export function useAssistant(): AssistantValue {
  const value = useContext(AssistantContext)
  if (!value) throw new Error('useAssistant must be used inside AssistantProvider')
  return value
}
