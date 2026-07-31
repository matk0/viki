import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { DraftsPage } from './DraftsPage'

const mocks = vi.hoisted(() => ({ draftProposals: vi.fn() }))

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, draftProposals: mocks.draftProposals } }
})

vi.mock('../router', () => ({
  Link: ({ to, children, ...props }: { to: string; children: React.ReactNode }) => <a href={to} {...props}>{children}</a>,
}))

it('uses Koncepty throughout the concepts index', async () => {
  mocks.draftProposals.mockResolvedValue({ proposals: [] })

  render(<DraftsPage />)

  expect(screen.getByRole('heading', { name: 'Koncepty' })).toBeInTheDocument()
  expect(await screen.findByText('Zatiaľ žiadne koncepty')).toBeInTheDocument()
  expect(screen.queryByText(/draft/i)).not.toBeInTheDocument()
})
