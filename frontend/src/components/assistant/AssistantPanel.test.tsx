import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StrictMode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AssistantConnectionState, AssistantConversation, AssistantDraftReceipt, AssistantStatus, AssistantStreamEvent } from '../../api/types'
import { Router } from '../../router'
import { AssistantProvider, useAssistant } from '../../assistant'
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
  pageTitle: 'Nový pojem',
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
        revisionId: 'revision-accepted-4',
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
      content: 'Pripravil som koncept.',
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
    expect(screen.getByRole('link', { name: /Zmluva pre domácnosť.*revízia revision/i })).toHaveAttribute(
      'href',
      '/page/page-contract?revision=revision-accepted-4',
    )
    expect(screen.getByRole('link', { name: /Koncept vytvorený.*Nový pojem.*revízia revision/i })).toHaveAttribute(
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

    act(() => FakeSpeechRecognition.latest?.emit('Vytvor pojem zákazník'))
    const composer = screen.getByPlaceholderText('Opíšte, čo má viki pridať alebo zmeniť…')
    expect(composer).toHaveValue('Vytvor pojem zákazník')
    expect(screen.getByText('Počúvam po slovensky…')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Zastaviť hlasový vstup' }))
    expect(FakeSpeechRecognition.latest?.stop).toHaveBeenCalledOnce()
    await user.type(composer, ' pre zmluvu')
    expect(composer).toHaveValue('Vytvor pojem zákazník pre zmluvu')
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

  it('navigates an accepted Edit turn to its live Draft page immediately', async () => {
    const user = userEvent.setup()
    render(
      <Router>
        <AssistantProvider>
          <AssistantPanel />
        </AssistantProvider>
      </Router>,
    )
    await screen.findByText('Zmluva vyžaduje identifikačné údaje.')

    await user.type(screen.getByPlaceholderText('Opíšte, čo má viki pridať alebo zmeniť…'), 'Vytvor pojem zákazník')
    await user.click(screen.getByRole('button', { name: 'Odoslať' }))

    await waitFor(() => expect(window.location.pathname).toBe('/drafts/turn-new'))
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
        revisionId: 'revision-accepted-4',
        pageId: 'page-contract',
        pageTitle: 'Zmluva pre domácnosť',
        draft: false,
      },
    } }))
    act(() => emit({ id: '21', type: 'draft_created', data: { turnId: 'turn-live', mode: 'edit', draft } }))
    expect(screen.getByText('Pripravujem zmenu.')).toBeInTheDocument()
    expect(screen.getAllByRole('link', { name: /Zmluva pre domácnosť.*revízia revision/i })).toHaveLength(2)
    expect(screen.getAllByRole('link', { name: /Koncept vytvorený.*Nový pojem.*revízia revision/i })).toHaveLength(2)

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

  it('always includes concepts without a scope control and rejects Hermes management commands', async () => {
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
    expect(screen.queryByRole('checkbox', { name: 'Zahrnúť koncepty' })).not.toBeInTheDocument()

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
    expect(screen.getByText(/Pojmy a scenáre zostávajú dostupné\./)).toBeInTheDocument()
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

  it('creates only one initial conversation under React Strict Mode', async () => {
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
})
