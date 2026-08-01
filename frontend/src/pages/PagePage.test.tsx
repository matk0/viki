import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PageDetail, Revision } from '../api/types'
import { Router } from '../router'
import styles from '../styles.css?raw'
import { PagePage } from './PagePage'

const mocks = vi.hoisted(() => ({
  page: vi.fn(),
  saveRevision: vi.fn(),
  revision: vi.fn(),
  vote: vi.fn(),
  comment: vi.fn(),
  resolveComment: vi.fn(),
  publish: vi.fn(),
  reloadPages: vi.fn(),
  pages: [] as PageDetail['page'][],
}))

vi.mock('../api/client', () => ({
  api: { page: mocks.page, saveRevision: mocks.saveRevision, revision: mocks.revision, vote: mocks.vote, comment: mocks.comment, resolveComment: mocks.resolveComment, publish: mocks.publish },
}))

vi.mock('../workspace', () => ({
  useWorkspace: () => ({ pages: mocks.pages, reloadPages: mocks.reloadPages }),
}))

const author = {
  id: 'user-1',
  email: 'matej@matejlukasik.com',
  displayName: 'Matej',
  createdAt: '2026-07-30T09:00:00Z',
}

function revision(id: string, number: number, status: Revision['status'], title: string): Revision {
  return {
    id,
    pageId: 'page-1',
    number,
    status,
    title,
    bodyMd: `${title} obsah`,
    aliases: [],
    steps: [],
    references: [],
    createdBy: author,
    createdAt: '2026-07-30T10:00:00Z',
  }
}

const accepted = revision('accepted-revision', 1, 'accepted', 'Publikované pravidlo')
const draft = revision('draft-revision', 2, 'draft', 'Presný draft')
const detail: PageDetail = {
  page: {
    id: 'page-1',
    kind: 'concept',
    conceptKind: 'noun',
    slug: 'pravidlo',
    title: 'Pravidlo',
    acceptedRevisionId: accepted.id,
    latestDraftRevisionId: draft.id,
    accepted: true,
    hasDraft: true,
    unresolvedRejections: 0,
    createdAt: '2026-07-30T10:00:00Z',
    updatedAt: '2026-07-30T10:00:00Z',
  },
  acceptedRevision: accepted,
  draftRevision: draft,
  revisions: [],
  comments: [],
  votes: [],
  children: [],
}

describe('PagePage revision links', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.pages = []
    mocks.page.mockResolvedValue(detail)
    mocks.saveRevision.mockResolvedValue(undefined)
    mocks.revision.mockResolvedValue(accepted)
    mocks.reloadPages.mockResolvedValue(undefined)
    window.history.replaceState({}, '', '/page/page-1?revision=draft-revision')
  })

  it('opens the exact current revision named by an assistant citation', async () => {
    const { container } = render(<Router><PagePage pageId="page-1" /></Router>)

    const heading = await screen.findByRole('heading', { name: 'Presný draft' })
    expect(heading.closest('.document-title-main')?.querySelector('.document-icon')).not.toBeNull()
    expect(screen.getByRole('button', { name: 'Upraviť' }).matches('.document-header > .document-edit-button')).toBe(true)
    expect(screen.getByRole('tab', { name: 'Draft #2' })).toHaveAttribute('aria-selected', 'true')
    const voteBadges = container.querySelectorAll('.vote-counts span')
    expect(voteBadges).toHaveLength(2)
    expect(voteBadges[0]).toHaveTextContent('Súhlas: 0')
    expect(voteBadges[1]).toHaveTextContent('Nesúhlas: 0')
    for (const badge of voteBadges) expect(badge.querySelector('svg')).toBeNull()
  })

  it('does not expose illustrative metadata in the page or editor', async () => {
    mocks.page.mockResolvedValue({
      ...detail,
      draftRevision: { ...draft, illustrative: true } as Revision,
    })
    render(<Router><PagePage pageId="page-1" /></Router>)

    expect(await screen.findByRole('heading', { name: 'Presný draft' })).toBeInTheDocument()
    expect(screen.queryByText('Ilustračné pravidlá')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Upraviť' }))
    expect(screen.queryByText('Obsahuje ilustračné, neoverené pravidlá')).not.toBeInTheDocument()
  })

  it('links concept mentions inline and uses each related page kind icon', async () => {
    const customer = {
      ...detail.page,
      id: 'customer-page',
      kind: 'concept' as const,
      title: 'zakaznik',
      slug: 'zakaznik',
    }
    const availability = {
      ...detail.page,
      id: 'availability-page',
      kind: 'feature' as const,
      conceptKind: undefined,
      title: 'Overenie dostupnosti',
      slug: 'overenie-dostupnosti',
    }
    const scenarioRevision: Revision = {
      ...accepted,
      pageId: 'scenario-page',
      title: 'Rezervácia služby',
      bodyMd: 'Zákazník pokračuje podľa procesu overenia dostupnosti.',
      references: [
        { targetPageId: customer.id, targetTitle: 'zakaznik', relation: 'uses' },
        { targetPageId: availability.id, targetTitle: availability.title, relation: 'relates_to' },
      ],
    }
    mocks.pages = [customer, availability]
    mocks.page.mockResolvedValue({
      ...detail,
      page: { ...availability, id: 'scenario-page', title: scenarioRevision.title },
      acceptedRevision: scenarioRevision,
      draftRevision: undefined,
    })
    window.history.replaceState({}, '', '/page/scenario-page')

    const { container } = render(<Router><PagePage pageId="scenario-page" /></Router>)

    const customerMention = await screen.findByRole('link', { name: 'Zákazník' })
    expect(customerMention).toHaveAttribute('href', '/page/customer-page')
    expect(customerMention.closest('.document-body')).not.toBeNull()

    const customerReference = container.querySelector('.reference-list a[href="/page/customer-page"]')
    const scenarioReference = container.querySelector('.reference-list a[href="/page/availability-page"]')
    expect(customerReference?.querySelector('svg')).toHaveClass('lucide-box')
    expect(scenarioReference?.querySelector('svg')).toHaveClass('lucide-workflow')
    expect(container.querySelector('.document-icon.feature svg')).toHaveClass('lucide-workflow')
  })

  it('top-aligns the shared page icon and status with the document title', () => {
    expect(styles).toMatch(/\.document-title-main\s*\{[^}]*align-items:\s*flex-start;/s)
    expect(styles).toMatch(/\.document-title-row\s*>\s*\.status-badge\s*\{[^}]*align-self:\s*start;/s)
  })

  it('shows loading and localized error states', async () => {
    let reject!: (reason: unknown) => void
    mocks.page.mockReturnValue(new Promise((_resolve, nextReject) => { reject = nextReject }))
    const view = render(<Router><PagePage pageId="page-1" /></Router>)
    expect(screen.getByText('Načítavam stránku…')).toBeVisible()

    reject(new Error('offline'))
    expect(await screen.findByText('offline')).toBeVisible()
    view.unmount()

    mocks.page.mockRejectedValue('failure')
    render(<Router><PagePage pageId="page-2" /></Router>)
    expect(await screen.findByRole('heading', { name: 'Stránku sa nepodarilo načítať.' })).toBeVisible()
  })

  it('selects available versions and explains an unavailable page version', async () => {
    const user = userEvent.setup()
    window.history.replaceState({}, '', '/page/page-1?revision=accepted-revision')
    const view = render(<Router><PagePage pageId="page-1" /></Router>)
    expect(await screen.findByRole('heading', { name: 'Publikované pravidlo' })).toBeVisible()
    await user.click(screen.getByRole('tab', { name: 'Draft #2' }))
    expect(screen.getByRole('heading', { name: 'Presný draft' })).toBeVisible()
    await user.click(screen.getByRole('tab', { name: 'Publikované' }))
    expect(screen.getByRole('heading', { name: 'Publikované pravidlo' })).toBeVisible()
    view.unmount()

    mocks.page.mockResolvedValue({ ...detail, acceptedRevision: undefined, draftRevision: undefined })
    window.history.replaceState({}, '', '/page/page-1')
    render(<Router><PagePage pageId="page-1" /></Router>)
    expect(await screen.findByRole('heading', { name: 'Verzia nie je dostupná' })).toBeVisible()
  })

  it('falls back to the accepted revision for an unknown requested revision', async () => {
    window.history.replaceState({}, '', '/page/page-1?revision=unknown')
    render(<Router><PagePage pageId="page-1" /></Router>)
    expect(await screen.findByRole('heading', { name: 'Publikované pravidlo' })).toBeVisible()
  })

  it('defaults to a lone draft and renders aliases, Gherkin, references, and child scenarios', async () => {
    const missing = { targetPageId: 'missing-page', targetTitle: 'External rule', relation: 'custom' }
    const scenarioPage = { ...detail.page, id: 'scenario-page', kind: 'scenario' as const, parentId: 'feature-page', conceptKind: undefined, title: 'Scenario' }
    const scenarioDraft: Revision = {
      ...draft, pageId: scenarioPage.id, bodyMd: '', aliases: ['alias one'],
      steps: [{ stableId: 'step-1', keyword: 'given', text: 'condition' }, { stableId: 'step-2', keyword: 'then', text: 'result' }],
      references: [missing],
    }
    mocks.page.mockResolvedValue({ ...detail, page: scenarioPage, acceptedRevision: undefined, draftRevision: scenarioDraft, children: [{ ...detail.page, id: 'child', kind: 'scenario', parentId: scenarioPage.id, title: 'Child scenario' }] })
    window.history.replaceState({}, '', '/page/scenario-page')
    const { container } = render(<Router><PagePage pageId="scenario-page" /></Router>)

    expect(await screen.findByRole('heading', { name: 'Presný draft' })).toBeVisible()
    expect(screen.getByText('Táto stránka zatiaľ nemá opis.')).toBeVisible()
    expect(screen.getByText('alias one')).toBeVisible()
    expect(screen.getByText('Za predpokladu')).toBeVisible()
    expect(screen.getByText('Tak')).toBeVisible()
    expect(screen.getByRole('link', { name: /External rule/ })).toHaveAttribute('href', '/page/missing-page')
    expect(screen.getByRole('link', { name: /Child scenario/ })).toHaveAttribute('href', '/page/child')
    expect(container.querySelector('.reference-list .lucide-book-open')).toBeInTheDocument()
  })

  it('edits inline, saves through the current base revision, reloads data, and cancels editing', async () => {
    const user = userEvent.setup()
    render(<Router><PagePage pageId="page-1" /></Router>)
    await screen.findByRole('heading', { name: 'Presný draft' })

    await user.click(screen.getByRole('button', { name: 'Upraviť' }))
    await user.click(screen.getByRole('button', { name: 'Zrušiť' }))
    expect(screen.getByRole('button', { name: 'Upraviť' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Upraviť' }))
    fireEvent.submit(screen.getByRole('button', { name: 'Uložiť novú revíziu' }).closest('form')!)

    await waitFor(() => expect(mocks.saveRevision).toHaveBeenCalledWith('page-1', 'draft-revision', expect.any(Object)))
    await waitFor(() => expect(mocks.reloadPages).toHaveBeenCalledOnce())
    expect(mocks.page).toHaveBeenCalledTimes(2)
  })

  it('opens and closes immutable revision history', async () => {
    const user = userEvent.setup()
    mocks.page.mockResolvedValue({ ...detail, revisions: [{ id: accepted.id, number: 1, status: 'accepted', title: accepted.title, createdBy: author, createdAt: accepted.createdAt }] })
    render(<Router><PagePage pageId="page-1" /></Router>)
    await screen.findByRole('heading', { name: 'Presný draft' })
    await user.click(screen.getByRole('button', { name: 'História' }))
    expect(screen.getByRole('heading', { name: 'História revízií' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Zavrieť' }))
    expect(screen.queryByRole('heading', { name: 'História revízií' })).not.toBeInTheDocument()
  })
})
