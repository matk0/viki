import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Page } from '../api/types'
import { Router } from '../router'
import { LibraryPage } from './LibraryPage'

const mocks = vi.hoisted(() => ({
  pages: [] as Page[],
  openNewPage: vi.fn(),
}))

vi.mock('../workspace', () => ({
  useWorkspace: () => ({
    pages: mocks.pages,
    loadingPages: false,
    openNewPage: mocks.openNewPage,
  }),
}))

const timestamp = '2026-07-31T10:00:00Z'
const pages: Page[] = [
  {
    id: 'customer', kind: 'primitive', primitiveKind: 'noun', slug: 'zakaznik', title: 'Zákazník',
    accepted: true, hasDraft: false, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp,
  },
  {
    id: 'account-number', kind: 'primitive', primitiveKind: 'noun', slug: 'cislo-uctu', title: 'Číslo účtu',
    accepted: true, hasDraft: true, unresolvedRejections: 0, createdAt: timestamp, updatedAt: timestamp,
  },
  {
    id: 'contract', kind: 'primitive', primitiveKind: 'noun', slug: 'zmluva', title: 'Zmluva',
    accepted: true, hasDraft: true, unresolvedRejections: 1, createdAt: timestamp, updatedAt: timestamp,
  },
]

describe('LibraryPage', () => {
  beforeEach(() => {
    mocks.pages = pages
    mocks.openNewPage.mockClear()
    window.history.replaceState({}, '', '/primitives')
  })

  it.each([
    ['primitive', 'Pridať pojem'],
    ['scenario', 'Pridať scenár'],
  ] as const)('opens a fixed %s creation flow', async (kind, buttonName) => {
    const user = userEvent.setup()
    render(<Router><LibraryPage kind={kind} /></Router>)

    await user.click(screen.getByRole('button', { name: buttonName }))

    expect(mocks.openNewPage).toHaveBeenCalledWith(kind)
  })

  it('matches titles regardless of case and Slovak diacritics', async () => {
    const user = userEvent.setup()
    render(<Router><LibraryPage kind="primitive" /></Router>)
    const search = screen.getByPlaceholderText('Hľadať v pojmoch…')

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
    render(<Router><LibraryPage kind="primitive" /></Router>)

    const trigger = screen.getByRole('button', { name: 'Filtrovať podľa stavu: Všetky' })
    await user.click(trigger)

    expect(screen.getByRole('listbox', { name: 'Filtrovať podľa stavu' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Všetky' })).toHaveAttribute('aria-selected', 'true')

    await user.click(screen.getByRole('option', { name: 'Koncept' }))

    expect(screen.getByRole('button', { name: 'Filtrovať podľa stavu: Koncept' })).toBeInTheDocument()
    expect(screen.queryByRole('listbox', { name: 'Filtrovať podľa stavu' })).not.toBeInTheDocument()
    expect(screen.getByText('Číslo účtu')).toBeInTheDocument()
    expect(screen.getByText('Zmluva')).toBeInTheDocument()
    expect(screen.queryByText('Zákazník')).not.toBeInTheDocument()
  })

  it('uses one text-only color-coded status badge per primitive row and no leading icon tile', () => {
    render(<Router><LibraryPage kind="primitive" /></Router>)

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
})
