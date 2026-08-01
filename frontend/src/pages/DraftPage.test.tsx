import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '../api/client'
import type { AssistantDraftProposal } from '../api/types'
import { I18nProvider, LanguageSwitcher } from '../i18n'
import styles from '../styles.css?raw'
import { DraftPage } from './DraftPage'

const mocks = vi.hoisted(() => ({
  draftProposal: vi.fn(),
  approveDraftProposal: vi.fn(),
  discardDraftProposal: vi.fn(),
  reviewDraftProposalOperation: vi.fn(),
  reloadPages: vi.fn(),
  navigate: vi.fn(),
  setClarificationResponse: vi.fn(),
  respondToClarification: vi.fn(),
  pages: [{
    id: '4d678534-71a0-497f-a168-ae9ff307e55d',
    kind: 'concept',
    conceptKind: 'noun',
    slug: 'zakaznik',
    title: 'Zákazník',
    accepted: false,
    hasDraft: true,
    unresolvedRejections: 0,
    createdAt: '2026-07-31T09:00:00Z',
    updatedAt: '2026-07-31T09:00:00Z',
  }],
  assistantState: { proposals: {}, activity: { state: 'thinking', mode: 'edit' } } as Record<string, unknown>,
}))

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return {
    ...actual,
    api: {
      ...actual.api,
      draftProposal: mocks.draftProposal,
      approveDraftProposal: mocks.approveDraftProposal,
      discardDraftProposal: mocks.discardDraftProposal,
      reviewDraftProposalOperation: mocks.reviewDraftProposalOperation,
    },
  }
})

vi.mock('../assistant', () => ({ useAssistant: () => mocks.assistantState }))
vi.mock('../workspace', () => ({ useWorkspace: () => ({ reloadPages: mocks.reloadPages, pages: mocks.pages }) }))
vi.mock('../router', () => ({ useRouter: () => ({ navigate: mocks.navigate }) }))

const proposal: AssistantDraftProposal = {
  id: '5c47d253-9d32-4c36-a6de-e18d72a01011',
  conversationId: '5c47d253-9d32-4c36-a6de-e18d72a01012',
  turnId: '5c47d253-9d32-4c36-a6de-e18d72a01011',
  summary: 'Pridať koncept Zmluva',
  status: 'awaiting_approval',
  operations: [{
    operation: 'create',
    clientKey: 'zmluva',
    kind: 'concept',
    conceptKind: 'noun',
    slug: 'zmluva',
    content: {
      title: 'Zmluva',
      bodyMd: 'Dohoda medzi spoločnosťou a zákazníkom.',
      aliases: [],
      steps: [],
      references: [],
    },
  }],
  operationReviews: [],
  publishedRevisions: [],
  createdAt: '2026-07-31T10:00:00Z',
  updatedAt: '2026-07-31T10:00:00Z',
}

describe('DraftPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.assistantState = { proposals: {}, activity: { state: 'thinking', mode: 'edit' } }
    mocks.draftProposal.mockRejectedValue(Object.assign(new Error('not found'), { status: 404 }))
  })

  it('shows live generation before Hermes has staged the proposal', () => {
    render(<DraftPage proposalId={proposal.id} />)

    expect(screen.getByRole('heading', { name: 'Draft' })).toBeInTheDocument()
    expect(screen.getByText('Viki pripravuje návrh')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /publikovať/i })).not.toBeInTheDocument()
  })

  it('shows the Hermes clarification on the concept page instead of an endless generation state', async () => {
    mocks.assistantState = {
      proposals: {},
      activity: { state: 'clarifying', mode: 'edit' },
      conversation: {
        id: proposal.conversationId,
        state: 'awaiting_clarification',
        messages: [{ id: `${proposal.id}-user` }],
      },
      clarification: {
        turnId: proposal.id,
        requestId: 'clarify-1',
        mode: 'edit',
        message: 'Je televízia súčasťou jedného spoločného balíka?',
      },
      clarificationResponse: 'Áno, ide o jeden spoločný balík.',
      setClarificationResponse: mocks.setClarificationResponse,
      respondToClarification: mocks.respondToClarification,
      error: '',
      loading: false,
    }

    render(<DraftPage proposalId={proposal.id} />)

    expect(screen.getByRole('heading', { name: 'Potrebujem doplnenie' })).toBeInTheDocument()
    expect(screen.getByText('Je televízia súčasťou jedného spoločného balíka?')).toBeInTheDocument()
    expect(screen.queryByText('Viki pripravuje návrh')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Pokračovať' }))
    expect(mocks.respondToClarification).toHaveBeenCalledOnce()
  })

  it('stops polling and explains a failed turn when no proposal was created', async () => {
    mocks.assistantState = {
      proposals: {},
      activity: null,
      conversation: {
        id: proposal.conversationId,
        state: 'idle',
        messages: [
          { id: `${proposal.id}-user`, role: 'user', content: 'Priprav proces.' },
          { id: `${proposal.id}-assistant`, role: 'assistant', content: 'Operation interrupted.' },
        ],
      },
      clarification: null,
      error: '',
      loading: false,
    }

    render(<DraftPage proposalId={proposal.id} />)

    expect(await screen.findByRole('alert')).toHaveTextContent('Príprava draftu bola prerušená')
    expect(screen.queryByText('Viki pripravuje návrh')).not.toBeInTheDocument()
    expect(mocks.draftProposal).toHaveBeenCalledOnce()
  })

  it('previews operations and publishes accepted revisions after the last operation approval', async () => {
    mocks.assistantState = { proposals: { [proposal.id]: proposal }, activity: { state: 'awaiting_approval', mode: 'edit' } }
    mocks.draftProposal.mockResolvedValue(proposal)
    mocks.reviewDraftProposalOperation.mockResolvedValue({
      ...proposal,
      status: 'published',
      operationReviews: [{ operationKey: 'zmluva', value: 'approve', reviewedAt: '2026-07-31T10:01:00Z' }],
      publishedAt: '2026-07-31T10:01:00Z',
      publishedRevisions: [{
        ...proposal.operations[0].content,
        id: '5c47d253-9d32-4c36-a6de-e18d72a01013',
        pageId: '5c47d253-9d32-4c36-a6de-e18d72a01014',
        number: 1,
        status: 'accepted',
        createdBy: { id: 'user-1', email: 'matej@matejlukasik.com', displayName: 'Matej', createdAt: '2026-07-31T09:00:00Z' },
        createdAt: '2026-07-31T10:01:00Z',
      }],
    })

    render(<DraftPage proposalId={proposal.id} />)

    expect(screen.getByRole('heading', { name: 'Zmluva' })).toBeInTheDocument()
    expect(screen.getByText('Dohoda medzi spoločnosťou a zákazníkom.')).toBeInTheDocument()
    expect(screen.queryByText('Čaká na schválenie')).not.toBeInTheDocument()
    expect(mocks.reviewDraftProposalOperation).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: 'Schváliť Zmluva' }))

    expect(mocks.reviewDraftProposalOperation).toHaveBeenCalledWith(proposal.id, 'zmluva', 'approve', '', false)
    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledWith(`/page/5c47d253-9d32-4c36-a6de-e18d72a01014?revision=5c47d253-9d32-4c36-a6de-e18d72a01013`, true))
    expect(screen.queryByText('Publikované')).not.toBeInTheDocument()
    expect(mocks.reloadPages).toHaveBeenCalled()
  })

  it('redirects a published proposal URL to its accepted scenario page', async () => {
    const scenarioProposal: AssistantDraftProposal = {
      ...proposal,
      status: 'published',
      operations: [
        proposal.operations[0],
        {
          operation: 'create',
          clientKey: 'reservation',
          kind: 'feature',
          slug: 'rezervacia',
          content: { title: 'Rezervácia', bodyMd: '', aliases: [], steps: [], references: [] },
        },
      ],
      operationReviews: [
        { operationKey: 'zmluva', value: 'approve', reviewedAt: '2026-07-31T10:01:00Z' },
        { operationKey: 'reservation', value: 'approve', reviewedAt: '2026-07-31T10:01:00Z' },
      ],
      publishedRevisions: [
        {
          ...proposal.operations[0].content,
          id: 'concept-revision', pageId: 'concept-page', number: 1, status: 'accepted',
          createdBy: { id: 'user-1', email: 'matej@matejlukasik.com', displayName: 'Matej', createdAt: '2026-07-31T09:00:00Z' },
          createdAt: '2026-07-31T10:01:00Z',
        },
        {
          title: 'Rezervácia', bodyMd: '', aliases: [], steps: [], references: [],
          id: 'scenario-revision', pageId: 'scenario-page', number: 1, status: 'accepted',
          createdBy: { id: 'user-1', email: 'matej@matejlukasik.com', displayName: 'Matej', createdAt: '2026-07-31T09:00:00Z' },
          createdAt: '2026-07-31T10:01:00Z',
        },
      ],
    }
    mocks.assistantState = { proposals: { [proposal.id]: scenarioProposal }, activity: { state: 'awaiting_approval', mode: 'edit' } }

    render(<DraftPage proposalId={proposal.id} />)

    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledWith('/page/scenario-page?revision=scenario-revision', true))
  })

  it('requires a reason before rejecting an operation', async () => {
    const user = userEvent.setup()
    mocks.assistantState = { proposals: { [proposal.id]: proposal }, activity: { state: 'awaiting_approval', mode: 'edit' } }
    mocks.draftProposal.mockResolvedValue(proposal)
    mocks.reviewDraftProposalOperation.mockResolvedValue({
      ...proposal,
      status: 'discarded',
      operationReviews: [{ operationKey: 'zmluva', value: 'reject', reason: 'Chýba presný spôsob výpočtu ceny.', reviewedAt: '2026-07-31T10:01:00Z' }],
    })

    render(<DraftPage proposalId={proposal.id} />)

    await user.click(screen.getByRole('button', { name: 'Odmietnuť Zmluva' }))

    const dialog = screen.getByRole('dialog', { name: 'Odmietnuť „Zmluva“?' })
    const submit = screen.getByRole('button', { name: 'Odmietnuť koncept' })
    expect(dialog).toBeInTheDocument()
    expect(submit).toBeDisabled()
    expect(mocks.reviewDraftProposalOperation).not.toHaveBeenCalled()

    await user.type(screen.getByRole('textbox', { name: 'Dôvod odmietnutia' }), 'Chýba presný spôsob výpočtu ceny.')
    expect(submit).toBeEnabled()
    await user.click(submit)

    expect(mocks.reviewDraftProposalOperation).toHaveBeenCalledWith(proposal.id, 'zmluva', 'reject', 'Chýba presný spôsob výpočtu ceny.', false)
    expect(await screen.findByRole('heading', { name: 'Zmluva' })).toBeInTheDocument()
    expect(screen.queryByText('Návrh bol odmietnutý')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Odmietnuť Zmluva' })).not.toBeInTheDocument()
  })

  it('renders the scenario first with its new concepts nested underneath and linked inline', () => {
    const linkedProposal: AssistantDraftProposal = {
      ...proposal,
      summary: 'Pridať scenár a chýbajúci koncept Zmluva',
      operations: [
        {
          ...proposal.operations[0],
          clientKey: 'zmluva',
        },
        {
          operation: 'create',
          clientKey: 'podpis-zmluvy',
          kind: 'feature',
          slug: 'zakaznik-chce-podpisat-zmluvu',
          content: {
            title: 'Zákazník chce podpísať zmluvu',
            bodyMd: 'Scenár zachytáva zámer zákazníka podpísať zmluvu.',
            aliases: [],
            steps: [],
            references: [
              { targetPageId: mocks.pages[0].id, targetTitle: 'Zákazník', relation: 'používa' },
              { targetPageId: '', targetClientKey: 'zmluva', targetTitle: 'Zmluva', relation: 'používa' },
            ],
          },
        },
      ],
    }
    mocks.assistantState = { proposals: { [proposal.id]: linkedProposal }, activity: { state: 'awaiting_approval', mode: 'edit' } }

    render(<DraftPage proposalId={proposal.id} />)

    const customer = screen.getByRole('link', { name: 'zákazníka' })
    const contract = screen.getByRole('link', { name: 'zmluvu' })
    expect(customer).toHaveAttribute('href', `/page/${mocks.pages[0].id}`)
    expect(contract).toHaveAttribute('href', '#draft-operation-zmluva')
    expect(customer).toHaveClass('concept-reference')
    expect(contract).toHaveClass('concept-reference')
    expect(customer.closest('p')).toHaveTextContent('Scenár zachytáva zámer zákazníka podpísať zmluvu.')
    expect(contract.closest('p')).toBe(customer.closest('p'))
    expect(screen.queryByText('Použité koncepty')).not.toBeInTheDocument()

    const scenario = screen.getByRole('heading', { name: 'Zákazník chce podpísať zmluvu' }).closest('article')
    const concept = screen.getByRole('heading', { name: 'Zmluva' }).closest('article')
    const group = scenario?.closest('.proposal-operation-group')

    expect(scenario).not.toBeNull()
    expect(concept).not.toBeNull()
    expect(group).toContainElement(concept)
    expect(concept?.closest('.proposal-operation-children')).not.toBeNull()
    expect(within(scenario!).queryByText('1')).not.toBeInTheDocument()
    expect(within(concept!).queryByText('2')).not.toBeInTheDocument()
    expect(scenario!.compareDocumentPosition(concept!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('draws one continuous rail through nested proposal operations', () => {
    expect(styles).toMatch(/\.proposal-operation-child::before\s*\{[^}]*top:\s*-15px;[^}]*bottom:\s*-15px;[^}]*border-left:\s*2px solid #aaa;/s)
    expect(styles).toMatch(/\.proposal-operation-child:last-child::before\s*\{[^}]*bottom:\s*calc\(100% - 35px\);/s)
    expect(styles).toMatch(/\.proposal-operation-child::after\s*\{[^}]*left:\s*-27px;[^}]*width:\s*27px;[^}]*border-top:\s*2px solid #aaa;/s)
  })

  it('reviews each proposal operation from controls on that card instead of a global footer', async () => {
    const user = userEvent.setup()
    const individualProposal: AssistantDraftProposal = {
      ...proposal,
      summary: 'Pridať scenár a koncept Zmluva',
      operations: [
        {
          ...proposal.operations[0],
          clientKey: 'zmluva',
        },
        {
          operation: 'create',
          clientKey: 'podpis-zmluvy',
          kind: 'feature',
          slug: 'podpis-zmluvy',
          content: {
            title: 'Podpísanie zmluvy',
            bodyMd: 'Zákazník podpíše zmluvu.',
            aliases: [],
            steps: [],
            references: [{ targetPageId: '', targetClientKey: 'zmluva', targetTitle: 'Zmluva', relation: 'používa' }],
          },
        },
      ],
    }
    mocks.assistantState = { proposals: { [proposal.id]: individualProposal }, activity: { state: 'awaiting_approval', mode: 'edit' } }
    mocks.reviewDraftProposalOperation.mockResolvedValue(individualProposal)

    render(<DraftPage proposalId={proposal.id} />)

    const concept = screen.getByRole('heading', { name: 'Zmluva' }).closest('article')
    expect(concept).not.toBeNull()
    expect(screen.queryByText(/Schválenie vykoná všetky zmeny naraz/)).not.toBeInTheDocument()
    await user.click(within(concept!).getByRole('button', { name: 'Schváliť Zmluva' }))
    expect(mocks.reviewDraftProposalOperation).toHaveBeenCalledWith(proposal.id, 'zmluva', 'approve', '', false)

    await user.click(within(concept!).getByRole('button', { name: 'Odmietnuť Zmluva' }))
    expect(screen.getByRole('dialog', { name: 'Odmietnuť „Zmluva“?' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Odmietnuť koncept' })).toBeDisabled()
  })

  it('reviews a feature together with every proposed subitem in its tree', async () => {
    const user = userEvent.setup()
    const scenarioTree: AssistantDraftProposal = {
      ...proposal,
      summary: 'Pridať celý proces rezervácie',
      operations: [
        { ...proposal.operations[0], clientKey: 'zmluva' },
        {
          operation: 'create',
          clientKey: 'rezervacia',
          kind: 'feature',
          slug: 'rezervacia',
          content: {
            title: 'Rezervácia', bodyMd: 'Zákazník odošle rezerváciu.', aliases: [], steps: [],
            references: [{ targetPageId: '', targetClientKey: 'zmluva', targetTitle: 'Zmluva', relation: 'výstup' }],
          },
        },
        {
          operation: 'create',
          clientKey: 'uspesna-rezervacia',
          kind: 'scenario',
          parentClientKey: 'rezervacia',
          slug: 'uspesna-rezervacia',
          content: {
            title: 'Úspešná rezervácia', bodyMd: 'Úspešné spracovanie rezervácie.', aliases: [],
            steps: [
              { keyword: 'given', text: 'zákazník má údaje' },
              { keyword: 'when', text: 'odošle rezerváciu' },
              { keyword: 'then', text: 'systém pripraví zmluvu' },
            ],
            references: [{ targetPageId: '', targetClientKey: 'zmluva', targetTitle: 'Zmluva', relation: 'výstup' }],
          },
        },
      ],
    }
    mocks.assistantState = { proposals: { [proposal.id]: scenarioTree }, activity: { state: 'awaiting_approval', mode: 'edit' } }
    mocks.reviewDraftProposalOperation.mockResolvedValue(scenarioTree)

    render(<DraftPage proposalId={proposal.id} />)

    const feature = screen.getByRole('heading', { name: 'Rezervácia' }).closest('article')
    expect(feature).not.toBeNull()
    await user.click(within(feature!).getByRole('button', { name: 'Schváliť Rezervácia' }))
    expect(mocks.reviewDraftProposalOperation).toHaveBeenCalledWith(proposal.id, 'rezervacia', 'approve', '', true)

    await user.click(within(feature!).getByRole('button', { name: 'Odmietnuť Rezervácia' }))
    expect(screen.getByText('Rozhodnutie sa použije aj na scenáre a nové koncepty patriace pod túto funkciu.')).toBeInTheDocument()
    await user.type(screen.getByRole('textbox', { name: 'Dôvod odmietnutia' }), 'Celý proces treba prepracovať.')
    await user.click(screen.getByRole('button', { name: 'Odmietnuť funkcia' }))
    expect(mocks.reviewDraftProposalOperation).toHaveBeenLastCalledWith(proposal.id, 'rezervacia', 'reject', 'Celý proces treba prepracovať.', true)
  })

  it('quickly fades card review controls in on hover or keyboard focus', () => {
    expect(styles).toMatch(/\.proposal-operation-actions\s*\{[^}]*opacity:\s*0;[^}]*transition:\s*opacity 110ms ease-out, transform 110ms ease-out;/s)
    expect(styles).toMatch(/\.proposal-operation\.reviewable:hover \.proposal-operation-actions, \.proposal-operation\.reviewable:focus-within \.proposal-operation-actions\s*\{[^}]*opacity:\s*1;/s)
  })

  it('loads a persisted proposal when it is absent from the stream', async () => {
    mocks.assistantState = { proposals: {}, activity: null, conversation: { state: 'idle', messages: [] }, clarification: null, error: '', loading: false }
    mocks.draftProposal.mockResolvedValue(proposal)
    render(<DraftPage proposalId={proposal.id} />)

    expect(await screen.findByRole('heading', { name: 'Zmluva' })).toBeVisible()
    expect(mocks.draftProposal).toHaveBeenCalledWith(proposal.id)
  })

  it('polls a running turn until Hermes persists its proposal', async () => {
    mocks.assistantState = { proposals: {}, activity: { state: 'thinking', mode: 'edit' }, conversation: { state: 'running', messages: [] }, clarification: null, error: '', loading: false }
    mocks.draftProposal.mockRejectedValueOnce(Object.assign(new Error('not found'), { status: 404 })).mockResolvedValueOnce(proposal)
    render(<DraftPage proposalId={proposal.id} />)

    expect(await screen.findByRole('heading', { name: 'Zmluva' }, { timeout: 2000 })).toBeVisible()
    expect(mocks.draftProposal).toHaveBeenCalledTimes(2)
  })

  it('does not update state after a persisted-proposal load is cancelled', async () => {
    mocks.assistantState = { proposals: {}, activity: null, conversation: { state: 'idle', messages: [] }, clarification: null, error: '', loading: false }
    let reject!: (reason: unknown) => void
    mocks.draftProposal.mockReturnValue(new Promise((_resolve, nextReject) => { reject = nextReject }))
    const view = render(<DraftPage proposalId={proposal.id} />)
    view.unmount()
    await act(async () => { reject(new Error('late failure')); await Promise.resolve() })
  })

  it('does not apply a persisted proposal that resolves after unmount', async () => {
    mocks.assistantState = { proposals: {}, activity: null, conversation: { state: 'idle', messages: [] }, clarification: null, error: '', loading: false }
    let resolve!: (value: AssistantDraftProposal) => void
    mocks.draftProposal.mockReturnValue(new Promise((nextResolve) => { resolve = nextResolve }))
    const view = render(<DraftPage proposalId={proposal.id} />)
    view.unmount()

    await act(async () => { resolve(proposal); await Promise.resolve() })
  })

  it('reports proposal loading errors and terminal missing-draft states', async () => {
    mocks.assistantState = { proposals: {}, activity: null, conversation: { state: 'idle', messages: [] }, clarification: null, error: '', loading: false }
    mocks.draftProposal.mockRejectedValueOnce(new Error('database offline'))
    const first = render(<DraftPage proposalId={proposal.id} />)
    expect(await screen.findByRole('alert')).toHaveTextContent('database offline')
    first.unmount()

    mocks.draftProposal.mockRejectedValueOnce('failure')
    const second = render(<DraftPage proposalId={proposal.id} />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Návrh sa nepodarilo načítať.')
    second.unmount()

    for (const [state, expected] of [['error', 'Príprava draftu zlyhala'], ['stopped', 'Príprava draftu bola zastavená'], ['idle', 'Tento draft neexistuje']] as const) {
      mocks.assistantState = { proposals: {}, activity: null, conversation: { state, messages: [] }, clarification: null, error: '', loading: false }
      mocks.draftProposal.mockRejectedValueOnce(Object.assign(new Error('not found'), { status: 404 }))
      const view = render(<DraftPage proposalId={proposal.id} />)
      expect(await screen.findByRole('alert')).toHaveTextContent(expected)
      view.unmount()
    }
  })

  it('recognizes typed API 404 responses as missing drafts', async () => {
    mocks.assistantState = { proposals: {}, activity: null, conversation: { state: 'idle', messages: [] }, clarification: null, error: '', loading: false }
    mocks.draftProposal.mockRejectedValueOnce(new APIError(404, 'not_found', 'not found'))

    render(<DraftPage proposalId={proposal.id} />)

    expect(await screen.findByRole('alert')).toHaveTextContent('Tento draft neexistuje')
  })

  it('renders and edits clarification choices associated through conversation history', async () => {
    const user = userEvent.setup()
    mocks.assistantState = {
      proposals: {}, activity: null,
      conversation: { state: 'awaiting_clarification', messages: [{ id: `turn-${proposal.id}`, role: 'user' }] },
      clarification: { requestId: 'request', mode: 'edit', message: 'Choose', choices: ['Home', 'Business'] },
      clarificationResponse: '', setClarificationResponse: mocks.setClarificationResponse, respondToClarification: mocks.respondToClarification,
      error: '', loading: false,
    }
    render(<DraftPage proposalId={proposal.id} />)
    await user.click(screen.getByRole('button', { name: 'Home' }))
    expect(mocks.setClarificationResponse).toHaveBeenCalledWith('Home')
    await user.type(screen.getByLabelText('Vaša odpoveď'), 'Typed')
    expect(mocks.setClarificationResponse).toHaveBeenLastCalledWith('d')
  })

  it('reports decision failures and closes rejection without mutating the proposal', async () => {
    const user = userEvent.setup()
    mocks.assistantState = { proposals: { [proposal.id]: proposal }, activity: null }
    mocks.reviewDraftProposalOperation.mockRejectedValueOnce(new Error('review offline'))
    const view = render(<DraftPage proposalId={proposal.id} />)
    await user.click(screen.getByRole('button', { name: 'Schváliť Zmluva' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('review offline')

    mocks.reviewDraftProposalOperation.mockRejectedValueOnce('failure')
    await user.click(screen.getByRole('button', { name: 'Schváliť Zmluva' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Rozhodnutie sa nepodarilo uložiť.')

    await user.click(screen.getByRole('button', { name: 'Odmietnuť Zmluva' }))
    const dialog = screen.getByRole('dialog')
    fireEvent.submit(dialog)
    expect(mocks.reviewDraftProposalOperation).toHaveBeenCalledTimes(2)
    mocks.reviewDraftProposalOperation.mockRejectedValueOnce(new Error('reject offline'))
    await user.type(screen.getByRole('textbox', { name: 'Dôvod odmietnutia' }), 'Reason')
    await user.click(screen.getByRole('button', { name: 'Odmietnuť koncept' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('reject offline')
    expect(screen.getByRole('dialog')).toBeVisible()
    fireEvent.mouseDown(view.container.querySelector('.modal-backdrop')!)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('keeps a busy rejection dialog open until its decision finishes', async () => {
    const user = userEvent.setup()
    mocks.assistantState = { proposals: { [proposal.id]: proposal }, activity: null }
    let resolve!: (value: AssistantDraftProposal) => void
    mocks.reviewDraftProposalOperation.mockReturnValue(new Promise((next) => { resolve = next }))
    const view = render(<DraftPage proposalId={proposal.id} />)
    await user.click(screen.getByRole('button', { name: 'Odmietnuť Zmluva' }))
    await user.type(screen.getByRole('textbox', { name: 'Dôvod odmietnutia' }), 'Reason')
    await user.click(screen.getByRole('button', { name: 'Odmietnuť koncept' }))
    expect(screen.getByRole('button', { name: 'Odmietam…' })).toBeDisabled()
    fireEvent.mouseDown(view.container.querySelector('.modal-backdrop')!)
    expect(screen.getByRole('dialog')).toBeVisible()
    resolve({ ...proposal, status: 'discarded' })
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('shows assistant failures before a proposal exists', () => {
    mocks.assistantState = {
      proposals: {}, activity: null, conversation: { state: 'error', messages: [] }, clarification: null,
      error: 'assistant offline', loading: false,
    }

    render(<DraftPage proposalId={proposal.id} />)

    expect(screen.getByRole('alert')).toHaveTextContent('assistant offline')
  })

  it('localizes the rejection action in English', async () => {
    const user = userEvent.setup()
    mocks.assistantState = { proposals: { [proposal.id]: proposal }, activity: null }

    render(<I18nProvider><LanguageSwitcher /><DraftPage proposalId={proposal.id} /></I18nProvider>)
    await user.click(screen.getByRole('switch', { name: 'Jazyk' }))
    await user.click(screen.getByRole('button', { name: 'Reject Zmluva' }))

    expect(screen.getByRole('button', { name: 'Reject concept' })).toBeVisible()
  })

  it('renders reviewed edits, aliases, every BDD keyword, unresolved references, and published links', () => {
	  const rich: AssistantDraftProposal = {
      ...proposal,
      status: 'discarded',
      operations: [{
	      operation: 'revise', pageId: 'existing-page', kind: 'scenario', parentId: 'feature', slug: 'rich',
        content: {
          title: 'Rich scenario', bodyMd: 'Unknown concept and external concept.', aliases: ['alias'],
          steps: [
            { keyword: 'given', text: 'given' }, { keyword: 'when', text: 'when' }, { keyword: 'then', text: 'then' },
            { keyword: 'and', text: 'and' }, { keyword: 'but', text: 'but' },
          ],
          references: [
            { targetPageId: '', targetClientKey: 'missing-client', targetTitle: 'Unknown concept', relation: 'uses' },
            { targetPageId: 'missing-page', targetTitle: 'External concept', relation: 'uses' },
            { targetPageId: '', targetTitle: 'Nothing', relation: 'uses' },
          ],
        },
      }],
      operationReviews: [{ operationKey: 'existing-page', value: 'reject', reason: 'Reason', reviewedAt: '' }],
    }
    mocks.assistantState = { proposals: { [proposal.id]: rich }, activity: null }
    const { container } = render(<DraftPage proposalId={proposal.id} />)

    expect(screen.getByText('Upraviť · Scenár')).toBeVisible()
    expect(screen.getByText('Odmietnuté')).toBeVisible()
    expect(screen.getByText('alias')).toBeVisible()
    for (const keyword of ['Za predpokladu', 'Keď', 'Tak', 'A', 'Ale']) expect(screen.getByText(keyword)).toBeVisible()
    expect(container.querySelectorAll('.concept-reference')).toHaveLength(0)
    expect(screen.queryByRole('button', { name: 'Schváliť Rich scenario' })).not.toBeInTheDocument()

    const published = { ...rich, status: 'published' as const, operationReviews: [], publishedRevisions: [{
      ...proposal.operations[0].content, id: 'revision', pageId: 'page', number: 1, status: 'accepted' as const,
      createdBy: { id: 'user', email: '', displayName: 'Matej', createdAt: '' }, createdAt: '',
    }] }
    mocks.assistantState = { proposals: { [proposal.id]: published }, activity: null }
    const view = render(<DraftPage proposalId={proposal.id} />)
    expect(view.container.querySelector('.published-links a')).toHaveAttribute('href', '/page/page?revision=revision')
  })

  it('uses generated operation keys and inferred concept titles when receipts omit optional labels', () => {
    const inferred: AssistantDraftProposal = {
      ...proposal,
      operations: [
        {
          operation: 'create', clientKey: 'new-contract', kind: 'concept', conceptKind: 'noun', slug: 'new-contract',
          content: { title: 'New contract', bodyMd: '', aliases: [], steps: [], references: [] },
        },
        {
	      operation: 'create', kind: 'scenario', parentId: 'feature', slug: 'anonymous-scenario',
          content: {
            title: 'Anonymous scenario', bodyMd: 'New contract and Zákazník.', aliases: [], steps: [],
            references: [
              { targetClientKey: 'new-contract', targetTitle: '', relation: 'uses' },
              { targetPageId: mocks.pages[0].id, targetTitle: '', relation: 'uses' },
            ],
          },
        },
      ],
    }
    mocks.assistantState = { proposals: { [proposal.id]: inferred }, activity: null }

    render(<DraftPage proposalId={proposal.id} />)

    expect(screen.getByRole('link', { name: 'New contract' })).toHaveAttribute('href', '#draft-operation-new-contract')
    expect(screen.getByRole('link', { name: 'Zákazník' })).toHaveAttribute('href', `/page/${mocks.pages[0].id}`)
    expect(screen.getByRole('button', { name: 'Schváliť Anonymous scenario' })).toBeVisible()
  })

  it('falls back to the drafts index when a published proposal has no accepted revision', async () => {
    const emptyPublished: AssistantDraftProposal = {
      ...proposal,
      status: 'published',
      operations: [],
      operationReviews: [],
      publishedRevisions: [],
    }
    mocks.assistantState = { proposals: { [proposal.id]: emptyPublished }, activity: null }

    render(<DraftPage proposalId={proposal.id} />)

    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledWith('/drafts', true))
  })

  it('uses the first accepted revision when the preferred operation has no parallel receipt', async () => {
    const featureOperation = {
      operation: 'create' as const, clientKey: 'feature', kind: 'feature' as const, slug: 'feature',
      content: { title: 'Feature', bodyMd: '', aliases: [], steps: [], references: [] },
    }
    const sparsePublished: AssistantDraftProposal = {
      ...proposal,
      status: 'published',
      operations: [proposal.operations[0], featureOperation],
      operationReviews: [
        { operationKey: 'zmluva', value: 'approve', reviewedAt: '' },
        { operationKey: 'feature', value: 'approve', reviewedAt: '' },
      ],
      publishedRevisions: [{
        ...proposal.operations[0].content,
        id: 'only-revision', pageId: 'only-page', number: 1, status: 'accepted',
        createdBy: { id: 'user', email: '', displayName: 'Matej', createdAt: '' }, createdAt: '',
      }],
    }
    mocks.assistantState = { proposals: { [proposal.id]: sparsePublished }, activity: null }

    render(<DraftPage proposalId={proposal.id} />)

    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledWith('/page/only-page?revision=only-revision', true))
  })

  it.each([
    ['submitting', 'Odovzdávam zadanie'], ['thinking', 'Rozumiem požiadavke'], ['searching', 'Hľadám súvislosti vo viki'],
    ['reading', 'Čítam existujúce pravidlá'], ['drafting', 'Skladám návrh zmien'], ['editing', 'Skladám návrh zmien'],
    ['writing', 'Skladám návrh zmien'], ['applying', 'Kontrolujem návrh'], ['awaiting_approval', 'Čaká na vaše schválenie'], ['unknown', 'Viki pripravuje návrh'],
  ])('labels generation activity %s', (state, expected) => {
    mocks.assistantState = { proposals: {}, activity: { state, mode: 'edit' }, conversation: { state: 'running', messages: [] }, clarification: null, error: '', loading: true }
    render(<DraftPage proposalId={proposal.id} />)
    expect(screen.getByText(`${expected}…`)).toBeVisible()
  })
})
