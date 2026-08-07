import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, expect, it, vi } from 'vitest'
import { SearchPage } from './SearchPage'

const mocks = vi.hoisted(() => ({
  search: vi.fn(),
  navigate: vi.fn(),
  params: new URLSearchParams(),
}))

vi.mock('../api/client', () => ({ api: { search: mocks.search } }))
vi.mock('../router', () => ({
  useRouter: () => ({ search: mocks.params, navigate: mocks.navigate }),
  Link: ({ to, className, children }: { to: string; className?: string; children: ReactNode }) => <a href={to} className={className}>{children}</a>,
}))

beforeEach(() => {
  mocks.search.mockReset()
  mocks.navigate.mockReset()
  mocks.params = new URLSearchParams()
})

it('runs initial URL searches and renders draft results without a preview', async () => {
  mocks.params = new URLSearchParams('q=zmluva&kind=concept&includeDrafts=true')
  mocks.search.mockResolvedValue({ results: [{
    revisionId: 'revision-1', draft: true, excerpt: '', score: 1,
    page: { id: 'page-1', kind: 'concept', conceptKind: 'noun', slug: 'zmluva', title: 'Zmluva', approved: false, hasDraft: true, unresolvedObjections: 0, createdAt: '', updatedAt: '' },
  }] })
  render(<SearchPage />)
  expect(await screen.findByText('Zmluva')).toBeInTheDocument()
  expect(screen.getByText('Bez textového náhľadu.')).toBeInTheDocument()
  expect(screen.getByText('Draft')).toBeInTheDocument()
  expect(mocks.search).toHaveBeenCalledWith('zmluva', 'concept', true)
})

it('submits query, kind, and draft settings then shows the empty state', async () => {
  const user = userEvent.setup()
  mocks.search.mockResolvedValue({ results: [] })
  render(<SearchPage />)
  await user.clear(screen.getByPlaceholderText('Čo hľadáte?'))
  await user.type(screen.getByPlaceholderText('Čo hľadáte?'), 'internet')
  await user.click(screen.getByRole('button', { name: 'Typ stránky' }))
  await user.click(screen.getByRole('option', { name: 'Funkcie' }))
  await user.click(screen.getByRole('checkbox', { name: 'Zahrnúť drafty' }))
  await user.click(screen.getByRole('button', { name: 'Hľadať' }))
  await waitFor(() => expect(mocks.search).toHaveBeenCalledWith('internet', 'feature', true))
  expect(mocks.navigate).toHaveBeenCalledWith('/search?q=internet&includeDrafts=true&kind=feature', true)
  expect(await screen.findByText('Žiadne výsledky')).toBeInTheDocument()
})

it('searches without a page-kind filter', async () => {
  const user = userEvent.setup()
  mocks.search.mockResolvedValue({ results: [] })
  render(<SearchPage />)
  await user.type(screen.getByPlaceholderText('Čo hľadáte?'), 'all pages')
  await user.click(screen.getByRole('button', { name: 'Hľadať' }))
  await waitFor(() => expect(mocks.search).toHaveBeenCalledWith('all pages', undefined, false))
  expect(mocks.navigate).toHaveBeenCalledWith('/search?q=all+pages&includeDrafts=false', true)
})
