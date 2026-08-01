import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Comment, PageDetail, Revision, User } from '../../api/types'
import { ReviewPanel } from './ReviewPanel'

const mocks = vi.hoisted(() => ({
  comment: vi.fn(),
  publish: vi.fn(),
  resolveComment: vi.fn(),
  vote: vi.fn(),
}))

vi.mock('../../api/client', async () => {
  const actual = await vi.importActual<typeof import('../../api/client')>('../../api/client')
  return { ...actual, api: { ...actual.api, ...mocks } }
})

const author: User = { id: 'user-1', email: 'matej@example.com', displayName: 'Matej', createdAt: '2026-01-01T10:00:00Z' }
const revision: Revision = {
  id: 'revision-1', pageId: 'page-1', number: 2, status: 'draft', title: 'Scenario', bodyMd: 'Body', aliases: [], references: [],
  steps: [
    { stableId: 'step-1', keyword: 'given', text: 'condition' },
    { id: 'step-2', keyword: 'then', text: 'outcome' },
  ], createdBy: author, createdAt: '2026-01-02T10:00:00Z',
}

function comment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 'comment-1', pageId: 'page-1', revisionId: revision.id, body: 'Needs detail', blocking: true, author,
    createdAt: '2026-01-03T10:00:00Z', replies: [{
      id: 'reply-1', pageId: 'page-1', revisionId: revision.id, parentCommentId: 'comment-1', body: 'Reply', blocking: false,
      author, createdAt: '2026-01-03T11:00:00Z', replies: [],
    }],
    ...overrides,
  }
}

function detail(comments: Comment[] = []): PageDetail {
  return {
    page: { id: 'page-1', kind: 'scenario', parentId: 'feature-1', slug: 'scenario', title: 'Scenario', accepted: true, hasDraft: true, unresolvedRejections: comments.filter((entry) => entry.blocking && !entry.resolvedAt).length, createdAt: '', updatedAt: '' },
    acceptedRevision: { ...revision, id: 'accepted-1', number: 1, status: 'accepted' }, draftRevision: revision,
    revisions: [], comments, children: [],
    votes: [
      { revisionId: revision.id, value: 'approve', user: author, createdAt: '' },
      { revisionId: revision.id, value: 'reject', user: author, createdAt: '' },
      { revisionId: 'other-revision', value: 'approve', user: author, createdAt: '' },
    ],
  }
}

beforeEach(() => {
  Object.values(mocks).forEach((mock) => mock.mockReset().mockResolvedValue(undefined))
})

describe('ReviewPanel', () => {
  it('approves, rejects with a required reason, publishes, and refreshes after each action', async () => {
    const user = userEvent.setup()
    const onChanged = vi.fn().mockResolvedValue(undefined)
    render(<ReviewPanel detail={detail()} revision={revision} onChanged={onChanged} />)

    expect(screen.getByText('Súhlas: 1')).toBeVisible()
    expect(screen.getByText('Nesúhlas: 1')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Súhlasím' }))
    await waitFor(() => expect(mocks.vote).toHaveBeenCalledWith(revision.id, 'approve'))

    await user.click(screen.getByRole('button', { name: 'Nesúhlasím' }))
    const submit = screen.getByRole('button', { name: 'Odoslať námietku' })
    expect(submit).toBeDisabled()
    await user.type(screen.getByLabelText('Dôvod nesúhlasu'), 'Chýba podmienka')
    await user.click(submit)
    await waitFor(() => expect(mocks.vote).toHaveBeenCalledWith(revision.id, 'reject', 'Chýba podmienka'))
    expect(screen.queryByLabelText('Dôvod nesúhlasu')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Publikovať revíziu' }))
    await waitFor(() => expect(mocks.publish).toHaveBeenCalledWith(revision.id))
    expect(onChanged).toHaveBeenCalledTimes(3)
  })

  it('anchors comments, resolves blockers, and replies to threads', async () => {
    const user = userEvent.setup()
    const blocking = comment({ anchorKind: 'step', anchorId: 'step-1' })
    const onChanged = vi.fn().mockResolvedValue(undefined)
    const view = render(<ReviewPanel detail={detail([blocking])} revision={revision} onChanged={onChanged} />)

    expect(screen.getByText('1 otvorená námietka')).toBeVisible()
    expect(screen.getByText('Krok správania')).toBeVisible()
    expect(screen.getByText('Reply')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Publikovanie je zablokované' })).toBeDisabled()

    fireEvent.submit(view.container.querySelector('.comment-form')!)
    expect(mocks.comment).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Kotva komentára' }))
    await user.click(screen.getByRole('option', { name: 'Krok 1' }))
    await user.type(screen.getByPlaceholderText('Pridať komentár k revízii…'), '  New comment  ')
    await user.click(screen.getByRole('button', { name: 'Odoslať komentár' }))
    await waitFor(() => expect(mocks.comment).toHaveBeenCalledWith({ pageId: 'page-1', revisionId: revision.id, body: 'New comment', anchorKind: 'step', anchorId: 'step-1' }))
    expect(screen.getByPlaceholderText('Pridať komentár k revízii…')).toHaveValue('')

    await user.click(screen.getByRole('button', { name: 'Označiť ako vyriešené' }))
    await waitFor(() => expect(mocks.resolveComment).toHaveBeenCalledWith(blocking.id))

    await user.click(screen.getByRole('button', { name: 'Odpovedať' }))
    const replyInput = screen.getByPlaceholderText('Napíšte odpoveď…')
    fireEvent.submit(replyInput.closest('form')!)
    expect(mocks.comment).toHaveBeenCalledTimes(1)
    await user.type(replyInput, '  Thread reply  ')
    await user.click(screen.getByRole('button', { name: 'Odoslať' }))
    await waitFor(() => expect(mocks.comment).toHaveBeenLastCalledWith({ pageId: 'page-1', revisionId: revision.id, body: 'Thread reply', parentCommentId: blocking.id }))
    expect(view.container.querySelector('.reply-form')).not.toBeInTheDocument()
  })

  it('shows resolved and inherited comments, hides draft actions for accepted revisions, and reports failures', async () => {
    const user = userEvent.setup()
    const resolved = comment({ id: 'resolved', blocking: true, resolvedAt: '2026-01-04T10:00:00Z', anchorKind: 'field', anchorId: 'title' })
    const inherited = comment({ id: 'inherited', revisionId: 'older', body: 'Inherited blocker' })
    const accepted = { ...revision, status: 'accepted' as const }
    const onChanged = vi.fn().mockResolvedValue(undefined)
    const view = render(<ReviewPanel detail={detail([resolved, inherited])} revision={accepted} onChanged={onChanged} />)

    expect(screen.getByText('Publikovaná revízia')).toBeVisible()
    expect(screen.getByText('Vyriešené')).toBeVisible()
    expect(screen.getByText('Názov')).toBeVisible()
    expect(screen.getByText('Inherited blocker')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Súhlasím' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Publikovať revíziu' })).not.toBeInTheDocument()

    mocks.comment.mockRejectedValueOnce(new Error('offline'))
    await user.type(screen.getByPlaceholderText('Pridať komentár k revízii…'), 'Comment')
    fireEvent.submit(view.container.querySelector('.comment-form')!)
    expect(await screen.findByRole('alert')).toHaveTextContent('offline')

    mocks.comment.mockRejectedValueOnce('failure')
    fireEvent.submit(view.container.querySelector('.comment-form')!)
    expect(await screen.findByRole('alert')).toHaveTextContent('Akciu sa nepodarilo dokončiť.')
  })

  it('labels plural blockers and nonblocking body comments', () => {
    const first = comment({ id: 'first' })
    const second = comment({ id: 'second', body: 'Second blocker' })
    const note = comment({ id: 'note', body: 'Body note', blocking: false, anchorKind: 'field', anchorId: 'bodyMd' })
    render(<ReviewPanel detail={detail([first, second, note])} revision={revision} onChanged={vi.fn()} />)

    expect(screen.getByText('2 otvorených námietok')).toBeVisible()
    expect(screen.getByText('Body note').closest('article')).not.toHaveClass('blocking')
    expect(screen.getByText('Body note').closest('article')).toHaveTextContent('Opis')
  })
})
