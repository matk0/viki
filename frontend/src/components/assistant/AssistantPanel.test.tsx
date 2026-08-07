import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StrictMode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AssistantConnectionState, AssistantConversation, AssistantDraftReceipt, AssistantStatus, AssistantStreamEvent } from '../../api/types'
import { Router } from '../../router'
import { AssistantProvider, useAssistant } from '../../assistant'
import { I18nProvider, LanguageSwitcher } from '../../i18n'
import { AssistantPanel } from './AssistantPanel'

const mocks = vi.hoisted(() => ({
  api: {
    assistantStatus: vi.fn(),
    assistantConversations: vi.fn(),
    assistantConversation: vi.fn(),
    createAssistantConversation: vi.fn(),
    sendAssistantMessage: vi.fn(),
    stopAssistantConversation: vi.fn(),
    respondToAssistantClarification: vi.fn(),
  },
  openAssistantEventStream: vi.fn<(
    id: string,
    onEvent: (event: AssistantStreamEvent) => void,
    onConnection: (state: AssistantConnectionState) => void,
  ) => () => void>(),
  reloadPages: vi.fn(),
}))

vi.mock('../../api/client', () => ({
  api: mocks.api,
  openAssistantEventStream: mocks.openAssistantEventStream,
}))

vi.mock('../../workspace', () => ({
  useWorkspace: () => ({ reloadPages: mocks.reloadPages }),
}))

const status: AssistantStatus = {
  available: true,
  qa: { mode: 'qa', connected: true, configured: true, ready: true },
  edit: { mode: 'edit', connected: true, configured: true, ready: true },
}

const draft: AssistantDraftReceipt = {
  revisionId: 'revision-draft-2',
  pageId: 'page-new',
  pageTitle: 'Nový koncept',
}

const conversation: AssistantConversation = {
  id: 'conversation-1',
  title: 'Zmluvné pravidlá',
  primaryMode: 'qa',
  lastMode: 'edit',
  state: 'idle',
  createdAt: '2026-07-30T10:00:00Z',
  updatedAt: '2026-07-30T10:05:00Z',
  messages: [
    {
      id: 'message-1',
      role: 'assistant',
      mode: 'qa',
      content: 'Zmluva vyžaduje identifikačné údaje.',
      citations: [{
        revisionId: 'revision-approved-4',
        pageId: 'page-contract',
        pageTitle: 'Zmluva pre domácnosť',
        draft: false,
      }],
      drafts: [],
      createdAt: '2026-07-30T10:02:00Z',
    },
    {
      id: 'message-2',
      role: 'assistant',
      mode: 'edit',
      content: 'Pripravil som draft.',
      citations: [],
      drafts: [draft],
      createdAt: '2026-07-30T10:05:00Z',
    },
  ],
}

const secondConversation: AssistantConversation = {
  ...conversation,
  id: 'conversation-2',
  title: 'Inštalačný proces',
  messages: [],
}

class FakeSpeechRecognition {
  static latest: FakeSpeechRecognition | null = null

  lang = ''
  continuous = false
  interimResults = false
  onresult: ((event: { resultIndex: number; results: ArrayLike<{ readonly isFinal: boolean; readonly 0: { transcript: string } }> }) => void) | null = null
  onerror: ((event: { error: string }) => void) | null = null
  onend: (() => void) | null = null
  start = vi.fn()
  stop = vi.fn(() => this.onend?.())
  abort = vi.fn()

  constructor() {
    FakeSpeechRecognition.latest = this
  }

  emit(transcript: string, isFinal = true) {
    this.onresult?.({
      resultIndex: 0,
      results: [{ 0: { transcript }, isFinal }],
    })
  }
}

function ActivityModeProbe() {
  const { activity } = useAssistant()
  return <output aria-label="Režim aktivity">{activity?.mode ?? ''}</output>
}

let assistantProbe: ReturnType<typeof useAssistant> | null = null

function AssistantStateProbe() {
  assistantProbe = useAssistant()
  return <output aria-label="assistant-state">{assistantProbe.loading ? 'loading' : `${assistantProbe.connection}:${assistantProbe.error}`}</output>
}

function renderProbe() {
  return render(<Router><AssistantProvider><AssistantStateProbe /></AssistantProvider></Router>)
}

describe('AssistantPanel', () => {
  let emit: (event: AssistantStreamEvent) => void
  let changeConnection: (state: AssistantConnectionState) => void

  beforeEach(() => {
    vi.clearAllMocks()
    window.history.replaceState({}, '', '/')
    mocks.api.assistantStatus.mockResolvedValue(status)
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [conversation] })
    mocks.api.assistantConversation.mockResolvedValue(conversation)
    mocks.api.sendAssistantMessage.mockResolvedValue({ turnId: 'turn-new' })
    mocks.api.stopAssistantConversation.mockResolvedValue(undefined)
    mocks.api.respondToAssistantClarification.mockResolvedValue(undefined)
    FakeSpeechRecognition.latest = null
    assistantProbe = null
    Object.defineProperty(window, 'webkitSpeechRecognition', {
      configurable: true,
      value: FakeSpeechRecognition,
    })
    mocks.openAssistantEventStream.mockImplementation((_id, onEvent, onConnection) => {
      emit = onEvent
      changeConnection = onConnection
      onConnection('connected')
      return vi.fn()
    })
  })

  it('renders the unified Hermes history with mode, exact citations, and durable draft receipts', async () => {
    const { container } = render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )

    expect(await screen.findByText('Zmluva vyžaduje identifikačné údaje.')).toBeInTheDocument()
    const messages = container.querySelectorAll('.chat-message')
    expect(within(messages[0] as HTMLElement).getByText('Otázky')).toBeInTheDocument()
    expect(within(messages[1] as HTMLElement).getByText('Úpravy')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Zmluva pre domácnosť.*verzia revision/i })).toHaveAttribute(
      'href',
      '/page/page-contract?revision=revision-approved-4',
    )
    expect(screen.getByRole('link', { name: /Draft vytvorený.*Nový koncept.*verzia revision/i })).toHaveAttribute(
      'href',
      '/page/page-new?revision=revision-draft-2',
    )
    await waitFor(() => expect(mocks.openAssistantEventStream).toHaveBeenCalledWith('conversation-1', expect.any(Function), expect.any(Function)))
  })

  it('does not render the redundant mode explanation panel', async () => {
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )

    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    expect(screen.queryByText('Režim iba na čítanie')).not.toBeInTheDocument()
    expect(screen.queryByText('Režim návrhu zmien')).not.toBeInTheDocument()
  })

  it('switches conversations through a styled accessible menu', async () => {
    const user = userEvent.setup()
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [conversation, secondConversation] })
    mocks.api.assistantConversation.mockImplementation(async (id: string) => id === secondConversation.id ? secondConversation : conversation)
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )

    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    expect(screen.queryByRole('combobox', { name: 'Rozhovor' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Rozhovor: Zmluvné pravidlá' }))

    expect(screen.getByRole('listbox', { name: 'Rozhovory' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Zmluvné pravidlá' })).toHaveAttribute('aria-selected', 'true')
    await user.click(screen.getByRole('option', { name: 'Inštalačný proces' }))

    await waitFor(() => expect(mocks.api.assistantConversation).toHaveBeenLastCalledWith(secondConversation.id))
    expect(screen.getByRole('button', { name: 'Rozhovor: Inštalačný proces' })).toBeInTheDocument()
    expect(screen.queryByRole('listbox', { name: 'Rozhovory' })).not.toBeInTheDocument()
  })

  it('captures Slovak speech into an editable composer before sending it to Hermes', async () => {
    const user = userEvent.setup()
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')

    await user.click(screen.getByRole('button', { name: 'Začať hlasový vstup' }))
    expect(FakeSpeechRecognition.latest).toMatchObject({
      lang: 'sk-SK',
      continuous: true,
      interimResults: true,
    })
    expect(FakeSpeechRecognition.latest?.start).toHaveBeenCalledOnce()

    act(() => FakeSpeechRecognition.latest?.emit('Vytvor koncept zákazník'))
    const composer = screen.getByPlaceholderText('Opíšte, čo má viki pridať alebo zmeniť…')
    expect(composer).toHaveValue('Vytvor koncept zákazník')
    expect(screen.getByText('Počúvam po slovensky…')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Zastaviť hlasový vstup' }))
    expect(FakeSpeechRecognition.latest?.stop).toHaveBeenCalledOnce()
    await user.type(composer, ' pre zmluvu')
    expect(composer).toHaveValue('Vytvor koncept zákazník pre zmluvu')
    expect(mocks.api.sendAssistantMessage).not.toHaveBeenCalled()
  })

  it('submits an asynchronous turn and lets the user stop it', async () => {
    const user = userEvent.setup()
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    await user.click(screen.getByRole('button', { name: 'Otázky' }))
    await user.type(screen.getByPlaceholderText('Opýtajte sa viki…'), 'Čo potrebuje zákazník?')
    await user.click(screen.getByRole('button', { name: 'Odoslať' }))

    expect(mocks.api.sendAssistantMessage).toHaveBeenCalledWith('conversation-1', 'Čo potrebuje zákazník?', 'qa')
    expect(screen.getByText('Čo potrebuje zákazník?')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Zastaviť' }))
    expect(mocks.api.stopAssistantConversation).toHaveBeenCalledWith('conversation-1')
  })

  it('navigates an accepted Edit turn to its live progress page immediately', async () => {
    const user = userEvent.setup()
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')

    await user.type(screen.getByPlaceholderText('Opíšte, čo má viki pridať alebo zmeniť…'), 'Vytvor koncept zákazník')
    await user.click(screen.getByRole('button', { name: 'Odoslať' }))

    await waitFor(() => expect(mocks.api.sendAssistantMessage).toHaveBeenCalledWith('conversation-1', 'Vytvor koncept zákazník', 'edit'))
    await waitFor(() => expect(window.location.pathname).toBe('/assistant/turns/turn-new'))
  })

  it('projects safe stream events into activity, citations, drafts, and structured clarification controls', async () => {
    const user = userEvent.setup()
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')

    act(() => emit({ id: '18', type: 'activity', data: { turnId: 'turn-live', mode: 'edit', state: 'reading', label: 'raw tool label' } }))
    expect(screen.getByText('Hľadám vo viki…')).toBeInTheDocument()
    expect(screen.queryByText('raw tool label')).not.toBeInTheDocument()

    act(() => emit({ id: '19', type: 'message_delta', data: { turnId: 'turn-live', mode: 'edit', delta: 'Pripravujem zmenu.' } }))
    act(() => emit({ id: '20', type: 'citation', data: {
      turnId: 'turn-live',
      mode: 'edit',
      citation: {
        revisionId: 'revision-approved-4',
        pageId: 'page-contract',
        pageTitle: 'Zmluva pre domácnosť',
        draft: false,
      },
    } }))
    act(() => emit({ id: '21', type: 'draft_created', data: { turnId: 'turn-live', mode: 'edit', draft } }))
    expect(screen.getByText('Pripravujem zmenu.')).toBeInTheDocument()
    expect(screen.getAllByRole('link', { name: /Zmluva pre domácnosť.*verzia revision/i })).toHaveLength(2)
    expect(screen.getAllByRole('link', { name: /Draft vytvorený.*Nový koncept.*verzia revision/i })).toHaveLength(2)

    act(() => emit({ id: '22', type: 'clarification', data: {
      turnId: 'turn-live',
      mode: 'edit',
      requestId: 'clarification-1',
      message: 'Má sa pravidlo týkať domácností alebo firiem?',
      choices: ['Domácnosť', 'Firma'],
    } }))
    expect(screen.getByText('Má sa pravidlo týkať domácností alebo firiem?')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Domácnosť' }))
    expect(screen.getByLabelText('Vaša odpoveď')).toHaveValue('Domácnosť')
    await user.click(screen.getByRole('button', { name: 'Pokračovať' }))
    await waitFor(() => expect(mocks.api.respondToAssistantClarification).toHaveBeenCalledWith(
      'conversation-1',
      'clarification-1',
      'Domácnosť',
    ))
  })

  it('retains only safe progress states, summary text, and draft receipts for the live Edit screen', async () => {
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))

    act(() => emit({ id: 'progress-1', type: 'activity', data: { turnId: 'turn-progress', mode: 'edit', state: 'submitted', label: 'must not be retained' } }))
    act(() => emit({ id: 'progress-2', type: 'activity', data: { turnId: 'turn-progress', mode: 'edit', state: 'searching', label: 'also hidden' } }))
    act(() => emit({ id: 'progress-3', type: 'activity', data: { turnId: 'turn-progress', mode: 'edit', state: 'searching', label: 'duplicate' } }))
    act(() => emit({ id: 'progress-4', type: 'message_delta', data: { turnId: 'turn-progress', mode: 'edit', delta: 'Pripravujem zmeny.' } }))
    act(() => emit({ id: 'progress-5', type: 'draft_created', data: { turnId: 'turn-progress', mode: 'edit', draft } }))
    act(() => emit({ id: 'progress-6', type: 'draft_created', data: { turnId: 'turn-progress', mode: 'edit', draft } }))
    act(() => emit({ id: 'progress-7', type: 'completed', data: { turnId: 'turn-progress', mode: 'edit' } }))

    expect(assistantProbe?.turns['turn-progress']).toEqual({
      id: 'turn-progress',
      mode: 'edit',
      status: 'completed',
      activities: ['submitted', 'searching'],
      summary: 'Pripravujem zmeny.',
      drafts: [draft],
    })
  })

  it('initializes progress from first receipt and error events while ignoring unbound errors', async () => {
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))

    act(() => emit({ id: 'first-draft', type: 'draft_created', data: { turnId: 'turn-first-draft', mode: 'edit', draft } }))
    act(() => emit({ id: 'first-error', type: 'error', data: { turnId: 'turn-first-error', mode: 'edit', code: 'failed', message: 'Turn failed.' } }))
    act(() => emit({ id: 'missing-mode', type: 'error', data: { turnId: 'turn-missing-mode', code: 'failed', message: 'Ignored binding.' } }))
    act(() => emit({ id: 'missing-turn', type: 'error', data: { code: 'failed', message: 'General failure.' } }))

    expect(assistantProbe?.turns['turn-first-draft']?.drafts).toEqual([draft])
    expect(assistantProbe?.turns['turn-first-error']).toMatchObject({ status: 'error', error: 'Turn failed.' })
    expect(assistantProbe?.turns['turn-missing-mode']).toBeUndefined()
  })

  it('always includes drafts without a scope control and rejects Hermes management commands', async () => {
    const user = userEvent.setup()
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    await user.click(screen.getByRole('button', { name: 'Otázky' }))
    expect(screen.queryByRole('checkbox', { name: 'Zahrnúť drafty' })).not.toBeInTheDocument()

    await user.type(screen.getByPlaceholderText('Opýtajte sa viki…'), '/model tajny-model')
    await user.click(screen.getByRole('button', { name: 'Odoslať' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Príkazy na správu asistenta nie sú vo viki povolené.')
    expect(mocks.api.sendAssistantMessage).not.toHaveBeenCalled()
  })

  it('keeps wiki access usable and clearly reports when Hermes is unavailable', async () => {
    mocks.api.assistantStatus.mockResolvedValue({
      available: false,
      qa: { mode: 'qa', connected: false, configured: false, ready: false },
      edit: { mode: 'edit', connected: false, configured: false, ready: false },
    })
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [] })
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )

    expect(await screen.findByText('Asistent je momentálne nedostupný.')).toBeInTheDocument()
    expect(screen.getByText('Viki môžete naďalej používať bez asistenta.')).toBeInTheDocument()
    expect(screen.getByText(/Koncepty, funkcie a scenáre zostávajú dostupné\./)).toBeInTheDocument()
    expect(mocks.api.createAssistantConversation).not.toHaveBeenCalled()
  })

  it('shows reconnect state without exposing transport details', async () => {
    const user = userEvent.setup()
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    act(() => changeConnection('reconnecting'))

    expect(screen.getByText('Spojenie s asistentom sa obnovuje…')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Pripojiť znova' }))
    await waitFor(() => expect(mocks.openAssistantEventStream).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(mocks.api.assistantConversation).toHaveBeenCalledTimes(2))
    expect(screen.queryByText(/WebSocket|JSON-RPC|Hermes session/i)).not.toBeInTheDocument()
  })

  it('creates only one initial conversation in Edit mode under React Strict Mode', async () => {
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [] })
    mocks.api.createAssistantConversation.mockResolvedValue({ ...conversation, messages: [] })
    render(
      <StrictMode>
        <Router>
          <AssistantProvider>
            <AssistantPanel />
          </AssistantProvider>
        </Router>
      </StrictMode>,
    )

    expect(await screen.findByText('Čo potrebujete zachytiť?')).toBeInTheDocument()
    expect(mocks.api.createAssistantConversation).toHaveBeenCalledTimes(1)
    expect(mocks.api.createAssistantConversation).toHaveBeenCalledWith('edit')
  })

  it('loads an idle existing conversation with Edit selected by default', async () => {
    const qaConversation = { ...conversation, lastMode: 'qa' as const }
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [qaConversation] })
    mocks.api.assistantConversation.mockResolvedValue(qaConversation)

    render(<Router><AssistantProvider><AssistantPanel /></AssistantProvider></Router>)

    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    expect(screen.getByRole('button', { name: 'Úpravy' })).toHaveClass('active')
    expect(screen.getByPlaceholderText('Opíšte, čo má viki pridať alebo zmeniť…')).toBeEnabled()
  })

  it('creates a conversation from the panel action', async () => {
    const user = userEvent.setup()
    mocks.api.createAssistantConversation.mockResolvedValue({ ...secondConversation, messages: [] })
    render(<Router><AssistantProvider><AssistantPanel /></AssistantProvider></Router>)
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    await user.click(screen.getByRole('button', { name: 'Nový rozhovor' }))
    await waitFor(() => expect(mocks.api.createAssistantConversation).toHaveBeenCalledWith('edit'))
  })

  it('keeps a ready profile usable when the other profile is unavailable', async () => {
    mocks.api.assistantStatus.mockResolvedValue({
      available: false,
      qa: { mode: 'qa', connected: true, configured: true, ready: true },
      edit: { mode: 'edit', connected: false, configured: false, ready: false },
    })
    const qaConversation = { ...conversation, lastMode: 'qa' as const }
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [qaConversation] })
    mocks.api.assistantConversation.mockResolvedValue(qaConversation)
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )

    expect(await screen.findByPlaceholderText('Opýtajte sa viki…')).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Nový rozhovor' })).toBeEnabled()
    expect(screen.queryByText('Asistent je momentálne nedostupný.')).not.toBeInTheDocument()
  })

  it('continues a Q&A clarification in the Q&A profile', async () => {
    const user = userEvent.setup()
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
          <ActivityModeProbe />
        </AssistantProvider>
      </Router>,
    )
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    act(() => emit({ id: '31', type: 'clarification', data: {
      turnId: 'turn-qa',
      mode: 'qa',
      requestId: 'clarification-qa',
      message: 'Ktorú oblasť máte na mysli?',
      choices: ['Zmluvy'],
    } }))
    await user.click(screen.getByRole('button', { name: 'Zmluvy' }))
    await user.click(screen.getByRole('button', { name: 'Pokračovať' }))

    expect(screen.getByLabelText('Režim aktivity')).toHaveTextContent('qa')
  })

  it('keeps history after status failure and reports conversation-list failures', async () => {
    mocks.api.assistantStatus.mockRejectedValueOnce(new Error('status offline'))
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))
    expect(assistantProbe?.status?.available).toBe(false)
    expect(assistantProbe?.conversation?.id).toBe(conversation.id)

    mocks.api.assistantConversations.mockRejectedValueOnce(new Error('history offline'))
    await act(async () => { await assistantProbe?.refresh() })
    expect(assistantProbe?.error).toBe('history offline')

    mocks.api.assistantConversations.mockRejectedValueOnce('failure')
    await act(async () => { await assistantProbe?.refresh() })
    expect(assistantProbe?.error).toBe('Rozhovory sa nepodarilo načítať.')
  })

  it('normalizes incomplete history and recovers action failures without losing input', async () => {
    const incomplete = {
      ...conversation,
      messages: [{ ...conversation.messages[0], citations: undefined, drafts: undefined }],
    } as unknown as AssistantConversation
    mocks.api.assistantConversation.mockResolvedValue(incomplete)
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))
    expect(assistantProbe?.conversation?.messages[0]).toMatchObject({ citations: [], drafts: [] })

    const calls = mocks.api.assistantConversation.mock.calls.length
    await act(async () => { await assistantProbe?.selectConversation(conversation.id) })
    expect(mocks.api.assistantConversation).toHaveBeenCalledTimes(calls)

    mocks.api.assistantConversation.mockRejectedValueOnce(new Error('selection failed'))
    await act(async () => { await assistantProbe?.selectConversation('other') })
    expect(assistantProbe?.error).toBe('selection failed')
    mocks.api.assistantConversation.mockRejectedValueOnce('failure')
    await act(async () => { await assistantProbe?.selectConversation('other') })
    expect(assistantProbe?.error).toBe('Rozhovor sa nepodarilo načítať.')

    mocks.api.createAssistantConversation.mockRejectedValueOnce(new Error('create failed'))
    await act(async () => { await assistantProbe?.createConversation() })
    expect(assistantProbe?.error).toBe('create failed')
    mocks.api.createAssistantConversation.mockRejectedValueOnce('failure')
    await act(async () => { await assistantProbe?.createConversation() })
    expect(assistantProbe?.error).toBe('Nový rozhovor sa nepodarilo vytvoriť.')
  })

  it('normalizes a conversation whose history omits the messages array', async () => {
    mocks.api.assistantConversation.mockResolvedValue({
      ...conversation,
      messages: undefined,
    } as unknown as AssistantConversation)

    renderProbe()

    await waitFor(() => expect(assistantProbe?.loading).toBe(false))
    expect(assistantProbe?.conversation?.messages).toEqual([])
  })

  it('ignores late stream events from a conversation that is no longer selected', async () => {
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))
    await waitFor(() => expect(mocks.openAssistantEventStream).toHaveBeenCalled())
    const emitOldConversation = emit
    mocks.api.assistantConversation.mockResolvedValueOnce(secondConversation)

    await act(async () => { await assistantProbe?.selectConversation(secondConversation.id) })
    expect(assistantProbe?.conversation?.id).toBe(secondConversation.id)
    mocks.api.assistantConversation.mockResolvedValue(secondConversation)

    const lateEvents: AssistantStreamEvent[] = [
      { id: 'late-activity', type: 'activity', data: { turnId: 'late', mode: 'qa', state: 'thinking', label: '' } },
      { id: 'late-delta', type: 'message_delta', data: { turnId: 'late', mode: 'qa', delta: 'late' } },
      { id: 'late-citation', type: 'citation', data: { turnId: 'late', mode: 'qa', citation: conversation.messages[0].citations[0] } },
      { id: 'late-draft', type: 'draft_created', data: { turnId: 'late', mode: 'edit', draft } },
      { id: 'late-clarification', type: 'clarification', data: { turnId: 'late', mode: 'qa', requestId: 'late', message: 'late' } },
      { id: 'late-error', type: 'error', data: { turnId: 'late', mode: 'qa', code: 'late', message: 'late' } },
      { id: 'late-stopped', type: 'stopped', data: { turnId: 'late', mode: 'qa' } },
    ]
    act(() => lateEvents.forEach(emitOldConversation))

    await waitFor(() => expect(assistantProbe?.conversation?.id).toBe(secondConversation.id))
    expect(assistantProbe?.conversation?.messages).toEqual([])
  })

  it('rolls back failed sends and restores failed clarification responses', async () => {
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))

    await act(async () => { await assistantProbe?.send() })
    expect(mocks.api.sendAssistantMessage).not.toHaveBeenCalled()
    act(() => assistantProbe?.setComposer('  Keep this message  '))
    mocks.api.sendAssistantMessage.mockRejectedValueOnce(new Error('send failed'))
    await act(async () => { await assistantProbe?.send() })
    expect(assistantProbe?.composer).toBe('Keep this message')
    expect(assistantProbe?.conversation?.messages.some((message) => message.id.startsWith('local-'))).toBe(false)
    expect(assistantProbe?.error).toBe('send failed')

    mocks.api.sendAssistantMessage.mockRejectedValueOnce('failure')
    await act(async () => { await assistantProbe?.send() })
    expect(assistantProbe?.error).toBe('Správu sa nepodarilo odoslať.')

    act(() => emit({ id: 'clarify-failure', type: 'clarification', data: { turnId: 'turn-c', mode: 'qa', requestId: 'request-c', message: 'Which one?' } }))
    act(() => assistantProbe?.setClarificationResponse('  Answer  '))
    mocks.api.respondToAssistantClarification.mockRejectedValueOnce(new Error('clarification failed'))
    await act(async () => { await assistantProbe?.respondToClarification() })
    expect(assistantProbe?.clarification?.requestId).toBe('request-c')
    expect(assistantProbe?.clarificationResponse).toBe('Answer')
    expect(assistantProbe?.error).toBe('clarification failed')

    mocks.api.respondToAssistantClarification.mockRejectedValueOnce('failure')
    await act(async () => { await assistantProbe?.respondToClarification() })
    expect(assistantProbe?.error).toBe('Doplnenie sa nepodarilo odoslať.')
  })

  it('does not let a stale send callback mutate a newly selected conversation', async () => {
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))
    act(() => assistantProbe?.setComposer('Message from the old conversation'))
    const staleSend = assistantProbe!.send
    mocks.api.assistantConversation.mockResolvedValueOnce(secondConversation)
    await act(async () => { await assistantProbe?.selectConversation(secondConversation.id) })
    mocks.api.sendAssistantMessage.mockRejectedValueOnce(new Error('late send failed'))

    await act(async () => { await staleSend() })

    expect(mocks.api.sendAssistantMessage).toHaveBeenCalledWith(
      conversation.id,
      'Message from the old conversation',
      'edit',
    )
    expect(assistantProbe?.conversation?.id).toBe(secondConversation.id)
    expect(assistantProbe?.conversation?.messages).toEqual([])
  })

  it('projects draft receipts and terminal stream events without duplicate evidence', async () => {
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))
    await waitFor(() => expect(mocks.openAssistantEventStream).toHaveBeenCalled())
    const citation = conversation.messages[0].citations[0]
    act(() => emit({ id: 'delta-1', type: 'message_delta', data: { turnId: 'turn-stream', mode: 'edit', delta: 'One' } }))
    act(() => emit({ id: 'delta-2', type: 'message_delta', data: { turnId: 'turn-stream', mode: 'edit', delta: ' two' } }))
    act(() => emit({ id: 'citation-1', type: 'citation', data: { turnId: 'turn-stream', mode: 'edit', citation } }))
    act(() => emit({ id: 'citation-2', type: 'citation', data: { turnId: 'turn-stream', mode: 'edit', citation } }))
    act(() => emit({ id: 'draft-1', type: 'draft_created', data: { turnId: 'turn-stream', mode: 'edit', draft } }))
    act(() => emit({ id: 'draft-2', type: 'draft_created', data: { turnId: 'turn-stream', mode: 'edit', draft } }))
    const streamed = assistantProbe?.conversation?.messages.find((message) => message.id === 'turn-turn-stream')
    expect(streamed).toMatchObject({ content: 'One two' })
    expect(streamed?.citations).toHaveLength(1)
    expect(streamed?.drafts).toHaveLength(1)

    expect(mocks.reloadPages).toHaveBeenCalledTimes(2)

    act(() => emit({ id: 'error', type: 'error', data: { turnId: 'turn-stream', mode: 'edit', code: 'failed', message: 'stream failed' } }))
    expect(assistantProbe?.conversation?.state).toBe('error')
    expect(assistantProbe?.error).toBe('stream failed')
    act(() => emit({ id: 'stopped', type: 'stopped', data: { turnId: 'turn-stream', mode: 'edit' } }))
    await waitFor(() => expect(mocks.api.assistantConversation).toHaveBeenCalled())
    act(() => emit({ id: 'completed', type: 'completed', data: { turnId: 'turn-stream', mode: 'edit' } }))
    await waitFor(() => expect(mocks.reloadPages).toHaveBeenCalledTimes(3))
  })

  it('fails closed when stream creation fails', async () => {
    mocks.openAssistantEventStream.mockImplementationOnce(() => { throw new Error('stream failed') })
    renderProbe()
    await waitFor(() => expect(assistantProbe?.connection).toBe('disconnected'))
  })

  it('blocks mode changes during active turns and reports stop errors', async () => {
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))
    act(() => assistantProbe?.setMode('edit'))
    expect(assistantProbe?.mode).toBe('edit')
    act(() => emit({ id: 'running', type: 'activity', data: { turnId: 'turn', mode: 'edit', state: 'thinking', label: '' } }))
    const mode = assistantProbe?.mode
    const createCalls = mocks.api.createAssistantConversation.mock.calls.length
    await act(async () => { await assistantProbe?.createConversation() })
    expect(mocks.api.createAssistantConversation).toHaveBeenCalledTimes(createCalls)
    act(() => assistantProbe?.setMode('qa'))
    expect(assistantProbe?.mode).toBe(mode)

    mocks.api.stopAssistantConversation.mockRejectedValueOnce(new Error('stop failed'))
    await act(async () => { await assistantProbe?.stop() })
    expect(assistantProbe?.error).toBe('stop failed')
    mocks.api.stopAssistantConversation.mockRejectedValueOnce('failure')
    await act(async () => { await assistantProbe?.stop() })
    expect(assistantProbe?.error).toBe('Asistenta sa nepodarilo zastaviť.')
  })

  it('creates a new conversation, keeps the selected one on refresh, and guards inactive actions', async () => {
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))
    const created = { ...secondConversation, id: 'created-conversation', lastMode: 'qa' as const }
    mocks.api.createAssistantConversation.mockResolvedValue(created)
    act(() => assistantProbe?.setComposer('discard me'))
    await act(async () => { await assistantProbe?.createConversation() })
    expect(assistantProbe?.conversation?.id).toBe(created.id)
    expect(assistantProbe?.composer).toBe('')

    mocks.api.assistantConversations.mockResolvedValue({ conversations: [created, conversation] })
    mocks.api.assistantConversation.mockResolvedValue(created)
    await act(async () => { await assistantProbe?.refresh() })
    expect(mocks.api.assistantConversation).toHaveBeenLastCalledWith(created.id)

    const stopCalls = mocks.api.stopAssistantConversation.mock.calls.length
    await act(async () => { await assistantProbe?.stop() })
    expect(mocks.api.stopAssistantConversation).toHaveBeenCalledTimes(stopCalls)
    const clarificationCalls = mocks.api.respondToAssistantClarification.mock.calls.length
    await act(async () => { await assistantProbe?.respondToClarification() })
    expect(mocks.api.respondToAssistantClarification).toHaveBeenCalledTimes(clarificationCalls)
  })

  it('moves to reconnecting when terminal reconciliation or connection recovery fails', async () => {
    renderProbe()
    await waitFor(() => expect(assistantProbe?.loading).toBe(false))
    mocks.api.assistantConversation.mockRejectedValueOnce(new Error('terminal history failed'))
    act(() => emit({ id: 'stop-failed-history', type: 'stopped', data: { turnId: 'turn', mode: 'edit' } }))
    await waitFor(() => expect(assistantProbe?.connection).toBe('reconnecting'))

    act(() => changeConnection('reconnecting'))
    mocks.api.assistantConversation.mockRejectedValueOnce(new Error('reconnect history failed'))
    act(() => changeConnection('connected'))
    await waitFor(() => expect(assistantProbe?.connection).toBe('reconnecting'))
  })

  it('covers panel keyboard, refresh, voice-submit, activity, and untitled conversation fallbacks', async () => {
    const user = userEvent.setup()
    const originalScrollTo = HTMLElement.prototype.scrollTo
    HTMLElement.prototype.scrollTo = vi.fn()
    mocks.api.assistantStatus.mockResolvedValue({
      available: false,
      qa: { mode: 'qa', connected: false, configured: false, ready: false },
      edit: { mode: 'edit', connected: false, configured: false, ready: false },
    })
    const untitled = { ...conversation, title: undefined, createdAt: 'invalid-date' }
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [untitled] })
    mocks.api.assistantConversation.mockResolvedValue(untitled)
    const view = render(<Router><AssistantProvider><AssistantPanel /></AssistantProvider></Router>)
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    expect(screen.getByRole('button', { name: 'Rozhovor: Rozhovor 1' })).toBeVisible()
    mocks.api.assistantStatus.mockResolvedValue(status)
    await user.click(screen.getByRole('button', { name: 'Skontrolovať spojenie' }))
    expect(mocks.api.assistantStatus).toHaveBeenCalledTimes(2)

    await user.click(screen.getByRole('button', { name: 'Úpravy' }))
    act(() => emit({ id: 'drafting', type: 'activity', data: { turnId: 'turn', mode: 'edit', state: 'drafting', label: '' } }))
    expect(screen.getByText('Vytváram drafty…')).toBeVisible()
    act(() => emit({ id: 'awaiting', type: 'activity', data: { turnId: 'turn', mode: 'edit', state: 'awaiting_approval', label: '' } }))
    expect(screen.getByText('Drafty čakajú na schválenie…')).toBeVisible()
    act(() => emit({ id: 'activity-complete', type: 'stopped', data: { turnId: 'turn', mode: 'edit' } }))
    await screen.findByRole('button', { name: 'Začať hlasový vstup' })

    const composer = view.container.querySelector('.assistant-composer textarea') as HTMLTextAreaElement
    const requestSubmit = vi.fn()
    Object.defineProperty(composer.form, 'requestSubmit', { configurable: true, value: requestSubmit })
    fireEvent.keyDown(composer, { key: 'Enter' })
    expect(requestSubmit).toHaveBeenCalledOnce()
    fireEvent.keyDown(composer, { key: 'Enter', shiftKey: true })
    expect(requestSubmit).toHaveBeenCalledOnce()

    await user.click(screen.getByRole('button', { name: 'Začať hlasový vstup' }))
    fireEvent.submit(composer.form!)
    expect(mocks.api.sendAssistantMessage).not.toHaveBeenCalled()
    HTMLElement.prototype.scrollTo = originalScrollTo
  })

  it('uses dated titles and accepts typed clarification text', async () => {
    const user = userEvent.setup()
    const untitled = { ...conversation, title: ' ', createdAt: '2026-07-30T10:00:00Z' }
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [untitled] })
    mocks.api.assistantConversation.mockResolvedValue(untitled)
    render(<Router><AssistantProvider><AssistantPanel /></AssistantProvider></Router>)
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    expect(screen.getByRole('button', { name: /Rozhovor: Rozhovor · 30\. 7\. 2026/ })).toBeVisible()

    act(() => emit({ id: 'typed-clarification', type: 'clarification', data: { turnId: 'turn', mode: 'edit', requestId: 'typed', message: 'Explain' } }))
    await user.type(screen.getByLabelText('Vaša odpoveď'), 'Typed answer')
    expect(screen.getByLabelText('Vaša odpoveď')).toHaveValue('Typed answer')
  })

  it('derives activity from active conversation state when no stream activity exists', async () => {
    const running = { ...conversation, state: 'running' as const, lastMode: 'edit' as const }
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [running] })
    mocks.api.assistantConversation.mockResolvedValue(running)
    const first = render(<Router><AssistantProvider><AssistantPanel /></AssistantProvider></Router>)
    expect(await screen.findByText('Premýšľam nad úpravou…')).toBeVisible()
    first.unmount()

    const clarifying = { ...conversation, state: 'awaiting_clarification' as const, lastMode: 'qa' as const }
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [clarifying] })
    mocks.api.assistantConversation.mockResolvedValue(clarifying)
    render(<Router><AssistantProvider><AssistantPanel /></AssistantProvider></Router>)
    expect(await screen.findByText('Čakám na doplnenie…')).toBeVisible()
  })

  it('shows recognition failures and the unsupported voice title', async () => {
    const user = userEvent.setup()
    const first = render(<Router><AssistantProvider><AssistantPanel /></AssistantProvider></Router>)
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    await user.click(screen.getByRole('button', { name: 'Začať hlasový vstup' }))
    act(() => FakeSpeechRecognition.latest?.onerror?.({ error: 'not-allowed' }))
    expect(screen.getByText('Povoľte prístup k mikrofónu a skúste to znova.')).toHaveClass('error')
    first.unmount()

    Object.defineProperty(window, 'webkitSpeechRecognition', { configurable: true, value: undefined })
    render(<Router><AssistantProvider><AssistantPanel /></AssistantProvider></Router>)
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    expect(screen.getByRole('button', { name: 'Začať hlasový vstup' })).toHaveAttribute('title', 'Tento prehliadač nepodporuje hlasový vstup')
  })

  it('labels draft citations and fallback Q&A activity', async () => {
    render(<Router><AssistantProvider><AssistantPanel /></AssistantProvider></Router>)
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    act(() => emit({ id: 'draft-citation', type: 'citation', data: { turnId: 'draft-citation', mode: 'qa', citation: { ...conversation.messages[0].citations[0], revisionId: 'draft-revision', draft: true } } }))
    expect(await screen.findByText('Draft', { selector: '.citation-link em' })).toBeVisible()
    act(() => emit({ id: 'unknown-activity', type: 'activity', data: { turnId: 'turn', mode: 'qa', state: 'unknown', label: '' } }))
    expect(screen.getByText('Hľadám odpoveď…')).toBeVisible()
  })

  it('formats untitled conversation dates in English', async () => {
    const user = userEvent.setup()
    const untitled = { ...conversation, title: '', createdAt: '2026-07-30T10:00:00Z' }
    mocks.api.assistantConversations.mockResolvedValue({ conversations: [untitled] })
    mocks.api.assistantConversation.mockResolvedValue(untitled)
    render(<I18nProvider><LanguageSwitcher /><Router><AssistantProvider><AssistantPanel /></AssistantProvider></Router></I18nProvider>)
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')
    await user.click(screen.getByRole('switch', { name: 'Jazyk' }))
    expect(screen.getByRole('button', { name: /Conversation: Conversation · 30\/07\/2026/ })).toBeVisible()
  })

  it('throws when the assistant hook is used outside its provider', () => {
    const Outside = () => { useAssistant(); return null }
    expect(() => render(<Outside />)).toThrow('useAssistant must be used inside AssistantProvider')
  })
})
