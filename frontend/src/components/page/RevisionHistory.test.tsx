import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import type { PageDetail, Revision } from '../../api/types'
import { RevisionHistory } from './RevisionHistory'

const mocks = vi.hoisted(() => ({ revision: vi.fn() }))
vi.mock('../../api/client', () => ({ api: mocks }))

const author = { id: 'user-1', email: 'a@b.c', displayName: 'Matej', createdAt: '2026-07-31T09:00:00Z' }
const accepted: Revision = { id: 'accepted', pageId: 'page-1', number: 1, status: 'accepted', title: 'Published', bodyMd: '', aliases: [], steps: [], references: [], createdBy: author, createdAt: '2026-07-31T10:00:00Z' }
const draft: Revision = { ...accepted, id: 'draft', number: 2, status: 'draft', title: 'Draft', bodyMd: 'Draft body' }
const detail: PageDetail = {
  page: { id: 'page-1', kind: 'concept', conceptKind: 'noun', slug: 'page', title: 'Page', accepted: true, hasDraft: true, unresolvedRejections: 0, createdAt: '', updatedAt: '' },
  acceptedRevision: accepted, draftRevision: draft,
  revisions: [
    { id: 'draft', number: 2, status: 'draft', title: 'Draft', createdBy: author, createdAt: draft.createdAt },
    { id: 'accepted', number: 1, status: 'accepted', title: 'Published', createdBy: author, createdAt: accepted.createdAt },
  ],
  comments: [], votes: [], children: [],
}

it('compares the selected draft, loads another revision, closes, and restores body scrolling', async () => {
  const user = userEvent.setup()
  let resolve!: (revision: Revision) => void
  mocks.revision.mockReturnValue(new Promise((next) => { resolve = next }))
  const onClose = vi.fn()
  const view = render(<RevisionHistory detail={detail} onClose={onClose} />)
  expect(document.body).toHaveClass('modal-open')
  expect(screen.getByText('Porovnávate s publikovanou revíziou #1')).toBeInTheDocument()
  expect(screen.getAllByText('Bez opisu')).not.toHaveLength(0)

  await user.click(screen.getByRole('button', { name: /Revízia #1/ }))
  expect(document.querySelector('.spinner')).toBeInTheDocument()
  await act(async () => resolve(accepted))
  expect(await screen.findByRole('heading', { name: 'Published' })).toBeInTheDocument()
  expect(document.querySelector('.compare-note')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Zavrieť' }))
  expect(onClose).toHaveBeenCalledOnce()
  view.unmount()
  expect(document.body).not.toHaveClass('modal-open')
})

it('starts empty when no accepted or draft revision exists', () => {
  render(<RevisionHistory detail={{ ...detail, acceptedRevision: undefined, draftRevision: undefined, revisions: [] }} onClose={vi.fn()} />)
  expect(document.querySelector('.revision-preview-heading')).not.toBeInTheDocument()
})
