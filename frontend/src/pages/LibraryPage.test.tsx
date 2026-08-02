import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Page } from '../api/types'
import { Router } from '../router'
import { LibraryPage } from './LibraryPage'

const mocks = vi.hoisted(() => ({
  pages: [] as Page[],
  openNewPage: vi.fn(),
  loadingPages: false,
}))

vi.mock('../workspace', () => ({
  useWorkspace: () => ({
    pages: mocks.pages,
    loadingPages: mocks.loadingPages,
    openNewPage: mocks.openNewPage,
  }),
}))

const timestamp = '2026-07-31T10:00:00Z'
const pages: Page[] = [
  {
    id: 'customer', kind: 'concept', conceptKind: 'noun', slug: 'zakaznik', title: 'Zákazník',
    approved: true, hasDraft: false, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp,
  },
  {
    id: 'account-number', kind: 'concept', conceptKind: 'noun', slug: 'cislo-uctu', title: 'Číslo účtu',
    approved: true, hasDraft: true, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp,
  },
  {
    id: 'contract', kind: 'concept', conceptKind: 'noun', slug: 'zmluva', title: 'Zmluva',
    approved: true, hasDraft: true, unresolvedObjections: 1, createdAt: timestamp, updatedAt: timestamp,
  },
]

describe('LibraryPage', () => {
  beforeEach(() => {
    mocks.pages = pages
    mocks.loadingPages = false
    mocks.openNewPage.mockClear()
    window.history.replaceState({}, '', '/concepts')
  })

  it.each([
    ['concept', 'Pridať koncept'],
    ['feature', 'Pridať funkciu'],
  ] as const)('opens a fixed %s creation flow', async (kind, buttonName) => {
    const user = userEvent.setup()
    render(<Router><LibraryPage kind={kind} /></Router>)

    await user.click(screen.getByRole('button', { name: buttonName }))

    expect(mocks.openNewPage).toHaveBeenCalledWith(kind)
  })

  it('matches titles regardless of case and Slovak diacritics', async () => {
    const user = userEvent.setup()
    render(<Router><LibraryPage kind="concept" /></Router>)
    const search = screen.getByPlaceholderText('Hľadať v konceptoch…')

    await user.type(search, 'ZAKAZNIK')
    expect(screen.getByText('Zákazník')).toBeVisible()
    expect(screen.queryByText('Číslo účtu')).not.toBeInTheDocument()

    await user.clear(search)
    await user.type(search, 'cislo uctu')
    expect(screen.getByText('Číslo účtu')).toBeVisible()
    expect(screen.queryByText('Zákazník')).not.toBeInTheDocument()
  })

  it('filters through an accessible status menu with styled options', async () => {
    const user = userEvent.setup()
    render(<Router><LibraryPage kind="concept" /></Router>)

    const trigger = screen.getByRole('button', { name: 'Filtrovať podľa stavu: Všetky' })
    await user.click(trigger)

    expect(screen.getByRole('listbox', { name: 'Filtrovať podľa stavu' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Všetky' })).toHaveAttribute('aria-selected', 'true')

    await user.click(screen.getByRole('option', { name: 'Draft' }))

    expect(screen.getByRole('button', { name: 'Filtrovať podľa stavu: Draft' })).toBeInTheDocument()
    expect(screen.queryByRole('listbox', { name: 'Filtrovať podľa stavu' })).not.toBeInTheDocument()
    expect(screen.getByText('Číslo účtu')).toBeInTheDocument()
    expect(screen.getByText('Zmluva')).toBeInTheDocument()
    expect(screen.queryByText('Zákazník')).not.toBeInTheDocument()
  })

  it('shows approved and draft availability as independent text-only pills', () => {
    mocks.pages = [
      { id: 'approved-only', kind: 'concept', conceptKind: 'noun', slug: 'approved', title: 'Approved only', approved: true, hasDraft: false, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'draft-only', kind: 'concept', conceptKind: 'noun', slug: 'draft', title: 'Draft only', approved: false, hasDraft: true, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'approved-with-draft', kind: 'concept', conceptKind: 'noun', slug: 'both', title: 'Approved with draft', approved: true, hasDraft: true, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
    ]
    render(<Router><LibraryPage kind="concept" /></Router>)

    const approvedRow = screen.getByRole('link', { name: /^Approved only/ })
    const draftRow = screen.getByRole('link', { name: /^Draft only/ })
    const approvedWithDraftRow = screen.getByRole('link', { name: /^Approved with draft/ })

    expect(approvedRow.querySelector('.page-icon-box')).toBeNull()
    expect(draftRow.querySelector('.page-icon-box')).toBeNull()
    expect(approvedWithDraftRow.querySelector('.page-icon-box')).toBeNull()
    expect([...approvedRow.querySelectorAll('.status-badge')].map((badge) => badge.textContent)).toEqual(['Schválené'])
    expect([...draftRow.querySelectorAll('.status-badge')].map((badge) => badge.textContent)).toEqual(['Draft'])
    expect([...approvedWithDraftRow.querySelectorAll('.status-badge')].map((badge) => badge.textContent)).toEqual(['Schválené', 'Draft'])
    for (const badge of document.querySelectorAll('.status-badge')) {
      expect(badge.querySelector('svg, i')).toBeNull()
    }
  })

  it('shows loading and empty states without rendering stale rows', () => {
    mocks.loadingPages = true
    const loading = render(<Router><LibraryPage kind="concept" /></Router>)
    expect(loading.container.querySelector('.skeleton-list')).toBeInTheDocument()
    loading.unmount()

    mocks.loadingPages = false
    mocks.pages = []
    render(<Router><LibraryPage kind="concept" /></Router>)
    expect(screen.getByRole('heading', { name: 'Nič sa nenašlo' })).toBeVisible()
  })

  it('renders feature scenario counts, children, and every icon state', () => {
    const featurePages: Page[] = [
      { id: 'feature-one', kind: 'feature', slug: 'one', title: 'First feature', approved: true, hasDraft: false, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'feature-many', kind: 'feature', slug: 'many', title: 'Second feature', approved: false, hasDraft: true, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'feature-rejected', kind: 'feature', slug: 'rejected', title: 'Rejected feature', approved: true, hasDraft: true, unresolvedObjections: 1, createdAt: timestamp, updatedAt: timestamp },
      { id: 'feature-new', kind: 'feature', slug: 'new', title: 'New feature', approved: false, hasDraft: false, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'scenario-one', kind: 'scenario', parentId: 'feature-one', slug: 'scenario-one', title: 'Only scenario', approved: true, hasDraft: false, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'scenario-two', kind: 'scenario', parentId: 'feature-many', slug: 'scenario-two', title: 'Draft scenario', approved: false, hasDraft: true, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'scenario-three', kind: 'scenario', parentId: 'feature-many', slug: 'scenario-three', title: 'Other scenario', approved: true, hasDraft: false, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'orphan', kind: 'scenario', slug: 'orphan', title: 'Orphan scenario', approved: true, hasDraft: false, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
    ]
    mocks.pages = featurePages
    const { container } = render(<Router><LibraryPage kind="feature" /></Router>)

    expect(screen.getByText('1 scenár')).toBeVisible()
    expect(screen.getByText('2 scenárov')).toBeVisible()
    expect(screen.getAllByText('0 scenárov')).toHaveLength(2)
    expect(screen.getByRole('link', { name: /Draft scenario/ }).querySelector('.draft-dot')).toBeInTheDocument()
    expect(container.querySelector('.page-icon-box.approved')).toBeInTheDocument()
    expect(container.querySelector('.page-icon-box.draft')).toBeInTheDocument()
    expect(screen.queryByText('Orphan scenario')).not.toBeInTheDocument()
  })

  it('filters approved pages separately from drafts', async () => {
    const user = userEvent.setup()
    render(<Router><LibraryPage kind="concept" /></Router>)
    await user.click(screen.getByRole('button', { name: 'Filtrovať podľa stavu: Všetky' }))
    await user.click(screen.getByRole('option', { name: 'Schválené' }))

    expect(screen.getByText('Zákazník')).toBeVisible()
    expect(screen.getByText('Číslo účtu')).toBeVisible()
    expect(screen.getByText('Zmluva')).toBeVisible()
  })

  it('renders the revision selected by each status filter when approved and draft titles differ', async () => {
    const user = userEvent.setup()
    mocks.pages = [{
      id: 'renamed-contract', kind: 'concept', conceptKind: 'noun', slug: 'contract', title: 'Draft contract title',
      approvedRevisionId: 'approved-revision', latestDraftRevisionId: 'draft-revision',
      approvedRevisionTitle: 'Approved contract title', draftRevisionTitle: 'Draft contract title',
      approved: true, hasDraft: true, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp,
    }]
    render(<Router><LibraryPage kind="concept" /></Router>)

    const allRow = screen.getByRole('link', { name: /^Draft contract title/ })
    expect([...allRow.querySelectorAll('.status-badge')].map((badge) => badge.textContent)).toEqual(['Schválené', 'Draft'])
    expect(allRow).toHaveAttribute('href', '/page/renamed-contract?revision=draft-revision')

    await user.click(screen.getByRole('button', { name: 'Filtrovať podľa stavu: Všetky' }))
    await user.click(screen.getByRole('option', { name: 'Schválené' }))

    const approvedRow = screen.getByRole('link', { name: /^Approved contract title/ })
    expect([...approvedRow.querySelectorAll('.status-badge')].map((badge) => badge.textContent)).toEqual(['Schválené', 'Draft'])
    expect(approvedRow).toHaveAttribute('href', '/page/renamed-contract')
    expect(screen.queryByText('Draft contract title')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Filtrovať podľa stavu: Schválené' }))
    await user.click(screen.getByRole('option', { name: 'Draft' }))

    const draftRow = screen.getByRole('link', { name: /^Draft contract title/ })
    expect([...draftRow.querySelectorAll('.status-badge')].map((badge) => badge.textContent)).toEqual(['Schválené', 'Draft'])
    expect(draftRow).toHaveAttribute('href', '/page/renamed-contract?revision=draft-revision')
    expect(screen.queryByText('Approved contract title')).not.toBeInTheDocument()
  })

  it('navigates a draft-labelled concept row to its draft revision', async () => {
    const user = userEvent.setup()
    mocks.pages = [{
      id: 'renamed-contract', kind: 'concept', conceptKind: 'noun', slug: 'contract', title: 'Draft contract title',
      approvedRevisionId: 'approved-revision', latestDraftRevisionId: 'draft-revision',
      approvedRevisionTitle: 'Approved contract title', draftRevisionTitle: 'Draft contract title',
      approved: true, hasDraft: true, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp,
    }]
    render(<Router><LibraryPage kind="concept" /></Router>)

    const draftRow = screen.getByRole('link', { name: /^Draft contract title/ })
    expect(draftRow).toHaveAttribute('href', '/page/renamed-contract?revision=draft-revision')

    await user.click(draftRow)

    expect(window.location.pathname + window.location.search).toBe('/page/renamed-contract?revision=draft-revision')
  })

  it('selects revision-specific titles for nested scenarios without hiding their feature context', async () => {
    const user = userEvent.setup()
    mocks.pages = [
      { id: 'feature', kind: 'feature', slug: 'contracts', title: 'Contracts', approvedRevisionTitle: 'Contracts', approved: true, hasDraft: false, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      {
        id: 'scenario', kind: 'scenario', parentId: 'feature', slug: 'sign-contract', title: 'Draft scenario title',
        approvedRevisionTitle: 'Approved scenario title', draftRevisionTitle: 'Draft scenario title',
        approved: true, hasDraft: true, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp,
      },
    ]
    const view = render(<Router><LibraryPage kind="feature" /></Router>)

    expect(screen.getByText('Draft scenario title')).toBeVisible()
    expect(view.container.querySelector('.feature-card-heading .status-badge')).toHaveTextContent('Schválené')

    await user.click(screen.getByRole('button', { name: 'Filtrovať podľa stavu: Všetky' }))
    await user.click(screen.getByRole('option', { name: 'Schválené' }))

    expect(screen.getByText('Contracts')).toBeVisible()
    expect(screen.getByText('Approved scenario title')).toBeVisible()
    expect(screen.queryByText('Draft scenario title')).not.toBeInTheDocument()
    expect(view.container.querySelector('.feature-children .draft-dot')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Filtrovať podľa stavu: Schválené' }))
    await user.click(screen.getByRole('option', { name: 'Draft' }))

    expect(screen.getByText('Contracts')).toBeVisible()
    expect(screen.getByText('Draft scenario title')).toBeVisible()
    expect(screen.queryByText('Approved scenario title')).not.toBeInTheDocument()
    expect(view.container.querySelector('.feature-children .draft-dot')).toBeInTheDocument()
  })

  it('keeps scenarios independently filterable beneath their feature', async () => {
    const user = userEvent.setup()
    mocks.pages = [
      { id: 'feature', kind: 'feature', slug: 'contracts', title: 'Contracts', approved: true, hasDraft: false, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'draft-scenario', kind: 'scenario', parentId: 'feature', slug: 'draft-scenario', title: 'Draft scenario', approved: false, hasDraft: true, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'approved-scenario', kind: 'scenario', parentId: 'feature', slug: 'approved-scenario', title: 'Approved scenario', approved: true, hasDraft: false, unresolvedObjections: 0, createdAt: timestamp, updatedAt: timestamp },
    ]
    const view = render(<Router><LibraryPage kind="feature" /></Router>)

    await user.click(screen.getByRole('button', { name: 'Filtrovať podľa stavu: Všetky' }))
    await user.click(screen.getByRole('option', { name: 'Draft' }))

    expect(screen.getByText('Contracts')).toBeVisible()
    expect(screen.getByText('Draft scenario')).toBeVisible()
    expect(screen.queryByText('Approved scenario')).not.toBeInTheDocument()
    expect(screen.getByText('1 scenár')).toBeVisible()
    expect(view.container.querySelector('.page-icon-box.approved')).toBeInTheDocument()
  })
})
