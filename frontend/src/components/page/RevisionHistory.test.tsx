import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import type { PageDetail, Revision } from '../../api/types'
import styles from '../../styles.css?raw'
import { RevisionHistory } from './RevisionHistory'

const mocks = vi.hoisted(() => ({ revision: vi.fn() }))
vi.mock('../../api/client', () => ({ api: mocks }))

const author = { id: 'user-1', email: 'a@b.c', displayName: 'Matej', createdAt: '2026-07-31T09:00:00Z' }
const approved: Revision = { id: 'approved', pageId: 'page-1', number: 1, status: 'approved', title: 'Approved', bodyMd: '', steps: [], references: [], createdBy: author, createdAt: '2026-07-31T10:00:00Z' }
const draft: Revision = { ...approved, id: 'draft', number: 2, status: 'draft', title: 'Draft', bodyMd: 'Draft body' }
const detail: PageDetail = {
  page: { id: 'page-1', kind: 'concept', conceptKind: 'noun', slug: 'page', title: 'Page', approved: true, hasDraft: true, unresolvedObjections: 0, createdAt: '', updatedAt: '' },
  approvedRevision: approved, draftRevision: draft,
  revisions: [
    { id: 'draft', number: 2, status: 'draft', title: 'Draft', createdBy: author, createdAt: draft.createdAt },
    { id: 'approved', number: 1, status: 'approved', title: 'Approved', createdBy: author, createdAt: approved.createdAt },
  ],
  comments: [], objections: [], children: [],
  reviewStates: [
    { revisionId: approved.id, state: 'approved', blockers: [] },
    { revisionId: draft.id, state: 'ready', blockers: [] },
  ],
}

it('compares the selected draft, loads another revision, closes, and restores body scrolling', async () => {
  const user = userEvent.setup()
  let resolve!: (revision: Revision) => void
  mocks.revision.mockReturnValue(new Promise((next) => { resolve = next }))
  const onClose = vi.fn()
  const view = render(<RevisionHistory detail={detail} onClose={onClose} />)
  expect(document.body).toHaveClass('modal-open')
  expect(screen.getByText('Porovnávate so schválenou verziou #1')).toBeInTheDocument()
  expect(screen.getByText('Vybraná verzia')).toBeInTheDocument()
  expect(screen.getAllByText('Bez opisu')).not.toHaveLength(0)

  await user.click(screen.getByRole('button', { name: /Verzia #1/ }))
  expect(document.querySelector('.spinner')).toBeInTheDocument()
  await act(async () => resolve(approved))
  expect(await screen.findByRole('heading', { name: 'Approved' })).toBeInTheDocument()
  expect(document.querySelector('.compare-note')).not.toBeInTheDocument()
  expect(screen.queryByText('Vybraná verzia')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Zavrieť' }))
  expect(onClose).toHaveBeenCalledOnce()
  view.unmount()
  expect(document.body).not.toHaveClass('modal-open')
})

it('starts empty when no approved or draft revision exists', () => {
  render(<RevisionHistory detail={{ ...detail, approvedRevision: undefined, draftRevision: undefined, revisions: [] }} onClose={vi.fn()} />)
  expect(document.querySelector('.revision-preview-heading')).not.toBeInTheDocument()
})

it('shows the revision history title without a header eyebrow', () => {
  render(<RevisionHistory detail={detail} onClose={vi.fn()} />)

  const header = screen.getByRole('heading', { name: 'História verzií' }).closest('header')
  expect(header?.querySelector('.eyebrow')).not.toBeInTheDocument()
})

it('keeps slight spacing between revision buttons', () => {
  expect(styles).toMatch(/\.history-layout\s*>\s*aside\s+button\s*\+\s*button\s*\{[^}]*margin-top:\s*4px;/s)
})

it('keeps imported history read-only, rejects other pages, and ignores superseded history responses', async () => {
  const user = userEvent.setup()
  const baseline: Revision = { ...approved, status: 'repository_baseline', title: 'First business version', bodyMd: '# First business version\n\nBusiness prose.' }
  const newer: Revision = { ...draft, status: 'repository_baseline', title: 'Second business version' }
  const imported: PageDetail = { ...detail, approvedRevision: undefined, draftRevision: undefined, repositoryRevision: baseline, revisions: [baseline, newer] }
  let resolve!: (revision: Revision) => void
  let reject!: (error: Error) => void
  const view = render(<RevisionHistory detail={imported} onClose={vi.fn()} />)
  expect(screen.getByRole('heading', { name: 'First business version' })).toBeVisible()
  expect(screen.queryByText('Repository baseline')).toBeNull()
  mocks.revision.mockImplementationOnce(() => new Promise<Revision>(done => { resolve = done }))
  await user.click(screen.getByRole('button', { name: /Verzia #1/ }))
  mocks.revision.mockResolvedValueOnce(newer)
  await user.click(screen.getByRole('button', { name: /Verzia #2/ }))
  await screen.findByRole('heading', { name: 'Second business version' })
  await act(async () => resolve(baseline))
  expect(screen.getByRole('heading', { name: 'Second business version' })).toBeVisible()
  mocks.revision.mockImplementationOnce(() => new Promise<Revision>((_, fail) => { reject = fail }))
  await user.click(screen.getByRole('button', { name: /Verzia #1/ }))
  mocks.revision.mockResolvedValueOnce(newer)
  await user.click(screen.getByRole('button', { name: /Verzia #2/ }))
  await screen.findByRole('heading', { name: 'Second business version' })
  await act(async () => reject(new Error('superseded failure')))
  expect(screen.queryByRole('alert')).toBeNull()
  mocks.revision.mockResolvedValueOnce({ ...baseline, pageId: 'wrong-page' })
  await user.click(screen.getByRole('button', { name: /Verzia #1/ }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Stránku sa nepodarilo načítať.')
  mocks.revision.mockRejectedValueOnce(new Error('offline'))
  await user.click(screen.getByRole('button', { name: /Verzia #1/ }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Stránku sa nepodarilo načítať.')
  mocks.revision.mockImplementationOnce(() => new Promise<Revision>(done => { resolve = done }))
  await user.click(screen.getByRole('button', { name: /Verzia #1/ }))
  view.unmount()
  await act(async () => resolve(baseline))
})
