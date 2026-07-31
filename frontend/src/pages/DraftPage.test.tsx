import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AssistantDraftProposal } from '../api/types'
import { DraftPage } from './DraftPage'

const mocks = vi.hoisted(() => ({
  draftProposal: vi.fn(),
  approveDraftProposal: vi.fn(),
  discardDraftProposal: vi.fn(),
  reloadPages: vi.fn(),
  pages: [{
    id: '4d678534-71a0-497f-a168-ae9ff307e55d',
    kind: 'primitive',
    primitiveKind: 'noun',
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
    },
  }
})

vi.mock('../assistant', () => ({ useAssistant: () => mocks.assistantState }))
vi.mock('../workspace', () => ({ useWorkspace: () => ({ reloadPages: mocks.reloadPages, pages: mocks.pages }) }))

const proposal: AssistantDraftProposal = {
  id: '5c47d253-9d32-4c36-a6de-e18d72a01011',
  conversationId: '5c47d253-9d32-4c36-a6de-e18d72a01012',
  turnId: '5c47d253-9d32-4c36-a6de-e18d72a01011',
  summary: 'Pridať pojem Zmluva',
  status: 'awaiting_approval',
  operations: [{
    operation: 'create',
    clientKey: 'zmluva',
    kind: 'primitive',
    primitiveKind: 'noun',
    slug: 'zmluva',
    content: {
      title: 'Zmluva',
      bodyMd: 'Dohoda medzi spoločnosťou a zákazníkom.',
      aliases: [],
      steps: [],
      references: [],
    },
  }],
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

  it('previews operations and publishes accepted revisions only after approval', async () => {
    mocks.assistantState = { proposals: { [proposal.id]: proposal }, activity: { state: 'awaiting_approval', mode: 'edit' } }
    mocks.draftProposal.mockResolvedValue(proposal)
    mocks.approveDraftProposal.mockResolvedValue({
      ...proposal,
      status: 'published',
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
    expect(mocks.approveDraftProposal).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: 'Schváliť a publikovať' }))

    expect(mocks.approveDraftProposal).toHaveBeenCalledWith(proposal.id)
    expect(await screen.findByText('otvoriť prijatú revíziu →')).toBeInTheDocument()
    expect(screen.queryByText('Publikované')).not.toBeInTheDocument()
    expect(mocks.reloadPages).toHaveBeenCalled()
  })

  it('requires a reason before rejecting a proposal', async () => {
    const user = userEvent.setup()
    mocks.assistantState = { proposals: { [proposal.id]: proposal }, activity: { state: 'awaiting_approval', mode: 'edit' } }
    mocks.draftProposal.mockResolvedValue(proposal)
    mocks.discardDraftProposal.mockResolvedValue({
      ...proposal,
      status: 'discarded',
      rejectionReason: 'Chýba presný spôsob výpočtu ceny.',
    })

    render(<DraftPage proposalId={proposal.id} />)

    await user.click(screen.getByRole('button', { name: 'Odmietnuť' }))

    const dialog = screen.getByRole('dialog', { name: 'Odmietnuť návrh?' })
    const submit = screen.getByRole('button', { name: 'Odmietnuť návrh' })
    expect(dialog).toBeInTheDocument()
    expect(submit).toBeDisabled()
    expect(mocks.discardDraftProposal).not.toHaveBeenCalled()

    await user.type(screen.getByRole('textbox', { name: 'Dôvod odmietnutia' }), 'Chýba presný spôsob výpočtu ceny.')
    expect(submit).toBeEnabled()
    await user.click(submit)

    expect(mocks.discardDraftProposal).toHaveBeenCalledWith(proposal.id, 'Chýba presný spôsob výpočtu ceny.')
    expect(await screen.findByRole('heading', { name: 'Zmluva' })).toBeInTheDocument()
    expect(screen.queryByText('Návrh bol odmietnutý')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Odmietnuť' })).not.toBeInTheDocument()
  })

  it('renders the scenario first with its new primitives nested underneath and linked inline', () => {
    const linkedProposal: AssistantDraftProposal = {
      ...proposal,
      summary: 'Pridať scenár a chýbajúci pojem Zmluva',
      operations: [
        {
          ...proposal.operations[0],
          clientKey: 'zmluva',
        },
        {
          operation: 'create',
          clientKey: 'podpis-zmluvy',
          kind: 'scenario',
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
    expect(customer).toHaveClass('primitive-reference')
    expect(contract).toHaveClass('primitive-reference')
    expect(customer.closest('p')).toHaveTextContent('Scenár zachytáva zámer zákazníka podpísať zmluvu.')
    expect(contract.closest('p')).toBe(customer.closest('p'))
    expect(screen.queryByText('Použité pojmy')).not.toBeInTheDocument()

    const scenario = screen.getByRole('heading', { name: 'Zákazník chce podpísať zmluvu' }).closest('article')
    const primitive = screen.getByRole('heading', { name: 'Zmluva' }).closest('article')
    const group = scenario?.closest('.proposal-operation-group')

    expect(scenario).not.toBeNull()
    expect(primitive).not.toBeNull()
    expect(group).toContainElement(primitive)
    expect(primitive?.closest('.proposal-operation-children')).not.toBeNull()
    expect(within(scenario!).queryByText('1')).not.toBeInTheDocument()
    expect(within(primitive!).queryByText('2')).not.toBeInTheDocument()
    expect(scenario!.compareDocumentPosition(primitive!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})
