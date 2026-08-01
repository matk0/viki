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
    accepted: true, hasDraft: false, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp,
  },
  {
    id: 'account-number', kind: 'concept', conceptKind: 'noun', slug: 'cislo-uctu', title: 'Číslo účtu',
    accepted: true, hasDraft: true, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp,
  },
  {
    id: 'contract', kind: 'concept', conceptKind: 'noun', slug: 'zmluva', title: 'Zmluva',
    accepted: true, hasDraft: true, unresolvedRejections: 1, createdAt: timestamp, updatedAt: timestamp,
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

  it('uses one text-only color-coded status badge per concept row and no leading icon tile', () => {
    render(<Router><LibraryPage kind="concept" /></Router>)

    const acceptedRow = screen.getByRole('link', { name: /^Zákazník/ })
    const draftRow = screen.getByRole('link', { name: /^Číslo účtu/ })
    const rejectedRow = screen.getByRole('link', { name: /^Zmluva/ })
    const acceptedBadge = acceptedRow.querySelector('.status-badge')
    const draftBadge = draftRow.querySelector('.status-badge')
    const rejectedBadge = rejectedRow.querySelector('.status-badge')

    expect(acceptedRow.querySelector('.page-icon-box')).toBeNull()
    expect(draftRow.querySelector('.page-icon-box')).toBeNull()
    expect(rejectedRow.querySelector('.page-icon-box')).toBeNull()
    expect(acceptedBadge).toHaveClass('accepted')
    expect(draftBadge).toHaveClass('draft')
    expect(rejectedBadge).toHaveClass('rejected')
    expect(rejectedBadge).toHaveTextContent('Odmietnuté')
    for (const badge of [acceptedBadge, draftBadge, rejectedBadge]) {
      expect(badge?.querySelector('svg, i')).toBeNull()
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
      { id: 'feature-one', kind: 'feature', slug: 'one', title: 'First feature', accepted: true, hasDraft: false, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'feature-many', kind: 'feature', slug: 'many', title: 'Second feature', accepted: false, hasDraft: true, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'feature-rejected', kind: 'feature', slug: 'rejected', title: 'Rejected feature', accepted: true, hasDraft: true, unresolvedRejections: 1, createdAt: timestamp, updatedAt: timestamp },
      { id: 'feature-new', kind: 'feature', slug: 'new', title: 'New feature', accepted: false, hasDraft: false, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'scenario-one', kind: 'scenario', parentId: 'feature-one', slug: 'scenario-one', title: 'Only scenario', accepted: true, hasDraft: false, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'scenario-two', kind: 'scenario', parentId: 'feature-many', slug: 'scenario-two', title: 'Draft scenario', accepted: false, hasDraft: true, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'scenario-three', kind: 'scenario', parentId: 'feature-many', slug: 'scenario-three', title: 'Other scenario', accepted: true, hasDraft: false, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp },
      { id: 'orphan', kind: 'scenario', slug: 'orphan', title: 'Orphan scenario', accepted: true, hasDraft: false, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp },
    ]
    mocks.pages = featurePages
    const { container } = render(<Router><LibraryPage kind="feature" /></Router>)

    expect(screen.getByText('1 scenár')).toBeVisible()
    expect(screen.getByText('2 scenárov')).toBeVisible()
    expect(screen.getAllByText('0 scenárov')).toHaveLength(2)
    expect(screen.getByRole('link', { name: /Draft scenario/ }).querySelector('.draft-dot')).toBeInTheDocument()
    expect(container.querySelector('.page-icon-box.approved')).toBeInTheDocument()
    expect(container.querySelector('.page-icon-box.draft')).toBeInTheDocument()
    expect(container.querySelector('.page-icon-box.rejected')).toBeInTheDocument()
    expect(screen.queryByText('Orphan scenario')).not.toBeInTheDocument()
  })

  it('filters accepted pages separately from drafts', async () => {
    const user = userEvent.setup()
    render(<Router><LibraryPage kind="concept" /></Router>)
    await user.click(screen.getByRole('button', { name: 'Filtrovať podľa stavu: Všetky' }))
    await user.click(screen.getByRole('option', { name: 'Publikované' }))

    expect(screen.getByText('Zákazník')).toBeVisible()
    expect(screen.getByText('Číslo účtu')).toBeVisible()
    expect(screen.getByText('Zmluva')).toBeVisible()
  })
})
