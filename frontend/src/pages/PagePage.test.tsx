import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PageDetail, Revision } from '../api/types'
import { Router } from '../router'
import { PagePage } from './PagePage'

const mocks = vi.hoisted(() => ({
  page: vi.fn(),
  reloadPages: vi.fn(),
}))

vi.mock('../api/client', () => ({
  api: { page: mocks.page },
}))

vi.mock('../workspace', () => ({
  useWorkspace: () => ({ pages: [], reloadPages: mocks.reloadPages }),
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
const draft = revision('draft-revision', 2, 'draft', 'Presný koncept')
const detail: PageDetail = {
  page: {
    id: 'page-1',
    kind: 'primitive',
    primitiveKind: 'noun',
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
    mocks.page.mockResolvedValue(detail)
    window.history.replaceState({}, '', '/page/page-1?revision=draft-revision')
  })

  it('opens the exact current revision named by an assistant citation', async () => {
    render(<Router><PagePage pageId="page-1" /></Router>)

    const heading = await screen.findByRole('heading', { name: 'Presný koncept' })
    expect(heading.closest('.document-title-main')?.querySelector('.document-icon')).not.toBeNull()
    expect(screen.getByRole('button', { name: 'Upraviť' }).matches('.document-header > .document-edit-button')).toBe(true)
    expect(screen.getByRole('tab', { name: 'Koncept #2' })).toHaveAttribute('aria-selected', 'true')
  })

  it('does not expose illustrative metadata in the page or editor', async () => {
    mocks.page.mockResolvedValue({
      ...detail,
      draftRevision: { ...draft, illustrative: true } as Revision,
    })
    render(<Router><PagePage pageId="page-1" /></Router>)

    expect(await screen.findByRole('heading', { name: 'Presný koncept' })).toBeInTheDocument()
    expect(screen.queryByText('Ilustračné pravidlá')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Upraviť' }))
    expect(screen.queryByText('Obsahuje ilustračné, neoverené pravidlá')).not.toBeInTheDocument()
  })
})
