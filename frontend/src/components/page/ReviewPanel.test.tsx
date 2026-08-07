import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Comment, Objection, ObjectionReviewBlocker, PageDetail, Revision, User } from '../../api/types'
import { DiscussionPanel, ReviewPanel } from './ReviewPanel'

const mocks = vi.hoisted(() => ({
  approve: vi.fn(),
  comment: vi.fn(),
  raiseObjection: vi.fn(),
  resolveObjection: vi.fn(),
}))

vi.mock('../../api/client', async () => {
  const actual = await vi.importActual<typeof import('../../api/client')>('../../api/client')
  return { ...actual, api: { ...actual.api, ...mocks } }
})

const author: User = { id: 'user-1', email: 'matej@example.com', displayName: 'Matej', createdAt: '2026-01-01T10:00:00Z' }
const revision: Revision = {
  id: 'revision-1', pageId: 'page-1', number: 2, status: 'draft', title: 'Scenario', bodyMd: 'Body', references: [],
  steps: [
    { stableId: 'step-1', keyword: 'given', text: 'condition' },
    { id: 'step-2', keyword: 'then', text: 'outcome' },
  ], createdBy: author, createdAt: '2026-01-02T10:00:00Z',
}

function comment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 'comment-1', pageId: 'page-1', revisionId: revision.id, body: 'Discussion note', author,
    createdAt: '2026-01-03T10:00:00Z', replies: [{
      id: 'reply-1', pageId: 'page-1', revisionId: revision.id, parentCommentId: 'comment-1', body: 'Reply',
      author, createdAt: '2026-01-03T11:00:00Z', replies: [],
    }],
    ...overrides,
  }
}

function objection(overrides: Partial<ObjectionReviewBlocker> = {}): ObjectionReviewBlocker {
  return {
    id: 'objection-1', type: 'objection', sourceRevisionId: revision.id, sourceRevisionNumber: revision.number,
    body: 'Needs detail', author, ...overrides,
  }
}

function detail(comments: Comment[] = [], blockers: ObjectionReviewBlocker[] = [], objections: Objection[] = []): PageDetail {
  const recordedObjections = objections.length > 0 ? objections : blockers.map((blocker) => ({
    id: blocker.id, pageId: 'page-1', revisionId: blocker.sourceRevisionId, revisionNumber: blocker.sourceRevisionNumber,
    body: blocker.body, author: blocker.author, createdAt: '2026-01-03T10:00:00Z',
  }))
  return {
    page: { id: 'page-1', kind: 'scenario', parentId: 'feature-1', slug: 'scenario', title: 'Scenario', approved: true, hasDraft: true, unresolvedObjections: blockers.length, createdAt: '', updatedAt: '' },
    approvedRevision: { ...revision, id: 'approved-1', number: 1, status: 'approved' }, draftRevision: revision,
    revisions: [], comments, objections: recordedObjections, children: [],
    reviewStates: [{ revisionId: revision.id, state: blockers.length ? 'blocked' : 'ready', blockers }],
  }
}

beforeEach(() => {
  Object.values(mocks).forEach((mock) => mock.mockReset().mockResolvedValue(undefined))
})

describe('ReviewPanel', () => {
  it('lists every review state and keeps status-changing actions inside the review card', () => {
    const view = render(<ReviewPanel detail={detail()} revision={revision} onChanged={vi.fn()} />)

    const reviewPanel = view.container.querySelector<HTMLElement>('.review-panel')!
    const statusList = within(reviewPanel).getByRole('list', { name: 'Možné stavy' })
    const statusItems = within(statusList).getAllByRole('listitem')
    expect(statusItems.map((item) => item.textContent)).toEqual([
      'Zablokované',
      'Draft',
      'Schválené',
    ])
    expect(statusItems[0].querySelector('svg')).toHaveClass('lucide-ban')
    expect(statusItems[1].querySelector('svg')).toHaveClass('lucide-pencil')
    expect(statusItems[2].querySelector('svg')).toHaveClass('lucide-badge-check')
    expect(within(statusList).queryByText('Aktuálny stav')).not.toBeInTheDocument()
    expect(within(statusList).getByText('Draft').closest('[role="listitem"]')).toHaveAttribute('aria-current', 'step')

    const approve = screen.getByRole('button', { name: 'Schváliť' })
    const object = screen.getByRole('button', { name: 'Vzniesť námietku' })
    expect(reviewPanel).toContainElement(approve)
    expect(reviewPanel).toContainElement(object)
    expect(object).toBe(view.container.querySelector('.review-status-actions')?.firstElementChild)
    expect(object.querySelector('svg')).toHaveClass('lucide-chevron-up')
    expect(approve.querySelector('svg')).toHaveClass('lucide-chevron-down')
  })

  it('fails fast when a draft has no server-derived review state', () => {
    const withoutReviewState = { ...detail(), reviewStates: [] }

    expect(() => render(<ReviewPanel detail={withoutReviewState} revision={revision} onChanged={vi.fn()} />))
      .toThrow(`Missing review state for draft revision ${revision.id}`)
  })

  it('does not render an empty-discussion placeholder', () => {
    const view = render(<DiscussionPanel detail={detail()} revision={revision} onChanged={vi.fn()} />)

    expect(screen.queryByText('K tejto verzii zatiaľ nie sú komentáre.')).not.toBeInTheDocument()
    expect(view.container.querySelector('.comment-list')).toBeEmptyDOMElement()
    expect(view.container.querySelector('.comment-form')).toBeInTheDocument()
  })

  it.each([
    ['draft', revision],
    ['approved', { ...revision, status: 'approved' as const }],
  ])('shows a labeled comment composer with an explicitly disabled action on %s revisions', async (_status, selectedRevision) => {
    const user = userEvent.setup()
    render(<DiscussionPanel detail={detail()} revision={selectedRevision} onChanged={vi.fn()} />)

    await user.click(screen.getByText('Diskusia (0)'))
    const textarea = screen.getByLabelText('Komentár')
    const addButton = screen.getByRole('button', { name: 'Pridať komentár' })
    expect(addButton).toBeDisabled()

    await user.type(textarea, 'Nová poznámka')
    expect(addButton).toBeEnabled()
  })

  it('submits a comment with Enter while preserving Shift+Enter for a newline', async () => {
    const user = userEvent.setup()
    render(<DiscussionPanel detail={detail()} revision={revision} onChanged={vi.fn().mockResolvedValue(undefined)} />)

    await user.click(screen.getByText('Diskusia (0)'))
    const textarea = screen.getByLabelText('Komentár')
    await user.type(textarea, 'Komentár cez Enter')
    await user.keyboard('{Enter}')

    await waitFor(() => expect(mocks.comment).toHaveBeenCalledWith({ pageId: 'page-1', revisionId: revision.id, body: 'Komentár cez Enter' }))
    expect(textarea).toHaveValue('')

    await user.type(textarea, 'Prvý riadok')
    await user.keyboard('{Shift>}{Enter}{/Shift}')
    expect(mocks.comment).toHaveBeenCalledTimes(1)
    expect(textarea).toHaveValue('Prvý riadok\n')
  })

  it('keeps server-derived draft actions outside the card and raises one objection', async () => {
    const user = userEvent.setup()
    const onChanged = vi.fn().mockResolvedValue(undefined)
    const view = render(<ReviewPanel detail={detail()} revision={revision} onChanged={onChanged} />)

    expect(screen.queryByText('Pripravené na schválenie')).not.toBeInTheDocument()
    expect(screen.queryByText(/Súhlas:/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Nesúhlas:/)).not.toBeInTheDocument()
    const decision = screen.getByRole('group', { name: 'Schválenie draftu' })
    await user.click(within(decision).getByRole('button', { name: 'Schváliť' }))
    await waitFor(() => expect(mocks.approve).toHaveBeenCalledWith(revision.id))

    await user.click(within(decision).getByRole('button', { name: 'Vzniesť námietku' }))
    const dialog = screen.getByRole('dialog', { name: 'Vzniesť námietku' })
    expect(dialog).toBeVisible()
    expect(view.container.querySelector('.review-panel .review-decision')).not.toBeInTheDocument()
    expect(within(dialog).getByLabelText('Dôvod námietky')).toBeVisible()
    await user.click(within(dialog).getByRole('button', { name: 'Zrušiť' }))
    expect(screen.queryByRole('dialog', { name: 'Vzniesť námietku' })).not.toBeInTheDocument()

    await user.click(within(decision).getByRole('button', { name: 'Vzniesť námietku' }))
    const submit = screen.getByRole('button', { name: 'Odoslať námietku' })
    expect(submit).toBeDisabled()
    await user.type(screen.getByLabelText('Dôvod námietky'), 'Chýba podmienka')
    await user.click(submit)
    await waitFor(() => expect(mocks.raiseObjection).toHaveBeenCalledWith(revision.id, 'Chýba podmienka'))
    expect(screen.queryByLabelText('Dôvod námietky')).not.toBeInTheDocument()

    expect(onChanged).toHaveBeenCalledTimes(2)
  })

  it('submits an objection with Enter while preserving Shift+Enter for a newline', async () => {
    const user = userEvent.setup()
    render(<ReviewPanel detail={detail()} revision={revision} onChanged={vi.fn().mockResolvedValue(undefined)} />)

    await user.click(screen.getByRole('button', { name: 'Vzniesť námietku' }))
    const textarea = screen.getByLabelText('Dôvod námietky')
    await user.type(textarea, 'Námietka cez Enter')
    await user.keyboard('{Enter}')

    await waitFor(() => expect(mocks.raiseObjection).toHaveBeenCalledWith(revision.id, 'Námietka cez Enter'))
    expect(screen.queryByLabelText('Dôvod námietky')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Vzniesť námietku' }))
    const multilineTextarea = screen.getByLabelText('Dôvod námietky')
    await user.type(multilineTextarea, 'Prvý riadok')
    await user.keyboard('{Shift>}{Enter}{/Shift}')
    expect(mocks.raiseObjection).toHaveBeenCalledTimes(1)
    expect(multilineTextarea).toHaveValue('Prvý riadok\n')
  })

  it('styles a submittable objection action like the comment submit action', async () => {
    const user = userEvent.setup()
    const panelDetail = detail()
    render(<><ReviewPanel detail={panelDetail} revision={revision} onChanged={vi.fn()} /><DiscussionPanel detail={panelDetail} revision={revision} onChanged={vi.fn()} /></>)

    await user.click(screen.getByRole('button', { name: 'Vzniesť námietku' }))
    await user.type(screen.getByLabelText('Dôvod námietky'), 'Chýba detail')
    await user.click(screen.getByText('Diskusia (0)'))
    await user.type(screen.getByLabelText('Komentár'), 'Poznámka')

    const objectionSubmit = screen.getByRole('button', { name: 'Odoslať námietku' })
    const commentSubmit = screen.getByRole('button', { name: 'Pridať komentár' })
    expect(objectionSubmit).toBeEnabled()
    expect(commentSubmit).toBeEnabled()
    expect(objectionSubmit.className).toBe(commentSubmit.className)
  })

  it('renders blockers once while comments remain a separate discussion', async () => {
    const user = userEvent.setup()
    const blocker = objection()
    const discussion = comment({ id: 'discussion', body: 'Nonblocking note' })
    const onChanged = vi.fn().mockResolvedValue(undefined)
    const panelDetail = detail([discussion], [blocker])
    const view = render(<><ReviewPanel detail={panelDetail} revision={revision} onChanged={onChanged} /><DiscussionPanel detail={panelDetail} revision={revision} onChanged={onChanged} /></>)

    expect(screen.queryByText('Schválenie je zablokované')).not.toBeInTheDocument()
    expect(screen.queryByText('1 otvorená námietka')).not.toBeInTheDocument()
    const blockedAccordion = screen.getByText('Zablokované').closest('details')
    expect(blockedAccordion).not.toHaveAttribute('open')
    expect(blockedAccordion?.querySelector('.review-decision')).toContainElement(screen.getByText('Needs detail'))
    expect(screen.getAllByText('Needs detail')).toHaveLength(1)
    expect(screen.getByRole('button', { name: 'Schváliť' })).toBeDisabled()

    await user.click(screen.getByText('Zablokované'))
    expect(blockedAccordion).toHaveAttribute('open')
    expect(screen.getByText('Needs detail')).toBeVisible()

    await user.click(screen.getByText('Diskusia (1)'))
    expect(screen.getByText('Nonblocking note')).toBeVisible()
    fireEvent.submit(view.container.querySelector('.comment-form')!)
    expect(mocks.comment).not.toHaveBeenCalled()
    expect(screen.queryByRole('button', { name: 'Kotva komentára' })).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('Komentár'), '  New comment  ')
    await user.click(screen.getByRole('button', { name: 'Pridať komentár' }))
    await waitFor(() => expect(mocks.comment).toHaveBeenCalledWith({ pageId: 'page-1', revisionId: revision.id, body: 'New comment' }))
    expect(screen.getByLabelText('Komentár')).toHaveValue('')

    await user.click(screen.getByRole('button', { name: 'Označiť ako vyriešené' }))
    await waitFor(() => expect(mocks.resolveObjection).toHaveBeenCalledWith(blocker.id))

    await user.click(screen.getByRole('button', { name: 'Odpovedať' }))
    const replyInput = screen.getByPlaceholderText('Napíšte odpoveď…')
    fireEvent.submit(replyInput.closest('form')!)
    expect(mocks.comment).toHaveBeenCalledTimes(1)
    await user.type(replyInput, '  Thread reply  ')
    await user.click(screen.getByRole('button', { name: 'Odoslať' }))
    await waitFor(() => expect(mocks.comment).toHaveBeenLastCalledWith({ pageId: 'page-1', revisionId: revision.id, body: 'Thread reply', parentCommentId: discussion.id }))
    expect(view.container.querySelector('.reply-form')).not.toBeInTheDocument()
  })

  it('hides review decisions and objections for approved revisions and reports comment failures', async () => {
    const user = userEvent.setup()
    const inherited = objection({ id: 'inherited', sourceRevisionId: 'older', sourceRevisionNumber: 1, body: 'Inherited blocker' })
    const note = comment({ id: 'note', body: 'Approved discussion' })
    const approved = { ...revision, status: 'approved' as const }
    const onChanged = vi.fn().mockResolvedValue(undefined)
    const view = render(<DiscussionPanel detail={detail([note], [inherited])} revision={approved} onChanged={onChanged} />)

    const discussion = screen.getByText('Diskusia (1)')
    expect(discussion).toBeVisible()
    expect(screen.getByText('Approved discussion')).not.toBeVisible()
    expect(screen.queryByText('Inherited blocker')).not.toBeInTheDocument()
    expect(screen.queryByText('Vyriešené')).not.toBeInTheDocument()
    expect(screen.queryByRole('group', { name: 'Schválenie draftu' })).not.toBeInTheDocument()
    expect(screen.queryByText(/Schválenie|Súhlas|Nesúhlas/)).not.toBeInTheDocument()

    await user.click(discussion)
    expect(screen.getByText('Approved discussion')).toBeVisible()

    mocks.comment.mockRejectedValueOnce(new Error('offline'))
    await user.type(screen.getByLabelText('Komentár'), 'Comment')
    fireEvent.submit(view.container.querySelector('.comment-form')!)
    expect(await screen.findByRole('alert')).toHaveTextContent('offline')

    mocks.comment.mockRejectedValueOnce('failure')
    fireEvent.submit(view.container.querySelector('.comment-form')!)
    expect(await screen.findByRole('alert')).toHaveTextContent('Akciu sa nepodarilo dokončiť.')
  })

  it('shows an inherited objection without a redundant source-revision heading', async () => {
    const inherited = objection({ id: 'inherited', sourceRevisionId: 'older-revision', sourceRevisionNumber: 1, body: 'Inherited blocker' })
    render(<ReviewPanel detail={detail([], [inherited])} revision={revision} onChanged={vi.fn().mockResolvedValue(undefined)} />)

    fireEvent.click(screen.getByText('Zablokované'))
    expect(screen.queryByText('Námietka z verzie #1')).not.toBeInTheDocument()
    expect(screen.getByText('Inherited blocker')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Odpovedať' })).not.toBeInTheDocument()
  })

  it('keeps a resolved objection visible without letting it block approval again', () => {
    const resolved: Objection = {
      id: 'resolved-objection', pageId: 'page-1', revisionId: revision.id, revisionNumber: revision.number,
      body: 'Resolved concern', author, createdAt: '2026-01-03T10:00:00Z',
      resolvedAt: '2026-01-04T10:00:00Z', resolvedBy: author,
    }
    render(<ReviewPanel detail={detail([], [], [resolved])} revision={revision} onChanged={vi.fn()} />)

    fireEvent.click(screen.getByText('Zablokované'))
    expect(screen.getByText('Resolved concern')).toBeVisible()
    expect(screen.getByText('Vyriešená')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Označiť ako vyriešené' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Schváliť' })).toBeEnabled()
  })

  it('renders every blocker without a redundant count and keeps nonblocking comments unadorned', () => {
    const first = objection({ id: 'first' })
    const second = objection({ id: 'second', body: 'Second blocker' })
    const note = comment({ id: 'note', body: 'Body note' })
    const panelDetail = detail([note], [first, second])
    render(<><ReviewPanel detail={panelDetail} revision={revision} onChanged={vi.fn()} /><DiscussionPanel detail={panelDetail} revision={revision} onChanged={vi.fn()} /></>)

    expect(screen.queryByText('2 otvorených námietok')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('Zablokované'))
    expect(screen.getByText('Needs detail')).toBeVisible()
    expect(screen.getByText('Second blocker')).toBeVisible()
    fireEvent.click(screen.getByText('Diskusia (1)'))
    expect(screen.getByText('Body note').closest('article')).not.toHaveClass('blocking')
    expect(screen.getByText('Body note').closest('article')).toHaveTextContent('Body note')
  })

  it('reports review action failures', async () => {
    const user = userEvent.setup()
    render(<ReviewPanel detail={detail()} revision={revision} onChanged={vi.fn()} />)

    mocks.approve.mockRejectedValueOnce(new Error('offline'))
    await user.click(screen.getByRole('button', { name: 'Schváliť' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('offline')

    mocks.approve.mockRejectedValueOnce('failure')
    await user.click(screen.getByRole('button', { name: 'Schváliť' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Akciu sa nepodarilo dokončiť.')
  })
})
