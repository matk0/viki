import { render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import type { AssistantDraftProposal } from '../api/types'
import { DraftsPage } from './DraftsPage'

const mocks = vi.hoisted(() => ({ draftProposals: vi.fn() }))

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, draftProposals: mocks.draftProposals } }
})

vi.mock('../router', () => ({
  Link: ({ to, children, ...props }: { to: string; children: React.ReactNode }) => <a href={to} {...props}>{children}</a>,
}))

beforeEach(() => { mocks.draftProposals.mockReset() })

it('uses Drafty throughout the drafts index', async () => {
  mocks.draftProposals.mockResolvedValue({ proposals: [] })

  render(<DraftsPage />)

  expect(screen.getByRole('heading', { name: 'Drafty' })).toBeInTheDocument()
  expect(await screen.findByText('Zatiaľ žiadne drafty')).toBeInTheDocument()
})

it('shows only proposals that still need a decision', async () => {
  const pending = proposalFixture('pending', 'awaiting_approval', 'Čakajúci draft')
  const published = proposalFixture('published', 'published', 'Už publikovaný návrh')
  mocks.draftProposals.mockResolvedValue({ proposals: [pending, published] })

  render(<DraftsPage />)

  expect(await screen.findByText('Čakajúci draft')).toBeInTheDocument()
  expect(screen.queryByText('Už publikovaný návrh')).not.toBeInTheDocument()
  expect(screen.queryByText('Vybavené')).not.toBeInTheDocument()
})

it('shows loading and provider or fallback errors', async () => {
  let reject!: (reason: unknown) => void
  mocks.draftProposals.mockReturnValue(new Promise((_resolve, nextReject) => { reject = nextReject }))
  const view = render(<DraftsPage />)
  expect(screen.getByText('Načítavam drafty…')).toBeVisible()
  reject(new Error('offline'))
  expect(await screen.findByText('offline')).toBeVisible()
  view.unmount()

  mocks.draftProposals.mockReturnValue({
    then: () => ({ catch: (onRejected: (reason: unknown) => void) => onRejected('failure') }),
  })
  render(<DraftsPage />)
  expect(await screen.findByText('Drafty sa nepodarilo načítať.')).toBeVisible()
})

it('uses localized change counts and a fallback proposal title', async () => {
  const one = { ...proposalFixture('one', 'awaiting_approval', ''), operations: [{}] as AssistantDraftProposal['operations'] }
  const few = { ...proposalFixture('few', 'awaiting_approval', 'Three'), operations: [{}, {}, {}] as AssistantDraftProposal['operations'] }
  const many = { ...proposalFixture('many', 'awaiting_approval', 'Five'), operations: [{}, {}, {}, {}, {}] as AssistantDraftProposal['operations'] }
  mocks.draftProposals.mockResolvedValue({ proposals: [one, few, many] })
  render(<DraftsPage />)

  expect(await screen.findByText('Návrh zmien')).toBeVisible()
  expect(screen.getByText(/1 zmena/)).toBeVisible()
  expect(screen.getByText(/3 zmeny/)).toBeVisible()
  expect(screen.getByText(/5 zmien/)).toBeVisible()
})

function proposalFixture(id: string, status: AssistantDraftProposal['status'], summary: string): AssistantDraftProposal {
  return {
    id,
    conversationId: 'conversation-1',
    turnId: id,
    summary,
    status,
    operations: [],
    operationReviews: [],
    publishedRevisions: [],
    createdAt: '2026-07-31T10:00:00Z',
    updatedAt: '2026-07-31T10:00:00Z',
  }
}
