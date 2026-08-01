import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'
import type { Page, Revision } from '../../api/types'
import { PageEditor } from './PageEditor'

const mocks = vi.hoisted(() => ({ saveRevision: vi.fn(), pages: [] as Page[] }))
vi.mock('../../api/client', async () => {
  const actual = await vi.importActual<typeof import('../../api/client')>('../../api/client')
  return { ...actual, api: { ...actual.api, saveRevision: mocks.saveRevision } }
})
vi.mock('../../workspace', () => ({ useWorkspace: () => ({ pages: mocks.pages }) }))

const author = { id: 'user-1', email: 'a@b.c', displayName: 'Matej', createdAt: '' }
const page: Page = { id: 'page-1', kind: 'scenario', parentId: 'feature-1', slug: 'scenario', title: 'Scenario', accepted: true, hasDraft: false, unresolvedRejections: 0, createdAt: '', updatedAt: '' }
const revision: Revision = {
  id: 'revision-1', pageId: page.id, number: 1, status: 'accepted', title: 'Scenario', bodyMd: 'Body', aliases: ['old'],
  steps: [{ stableId: 'step-1', keyword: 'given', text: 'condition' }],
  references: [{ targetPageId: 'page-2', targetTitle: 'Concept', relation: 'uses' }, { targetPageId: 'page-3', targetTitle: 'Action', relation: 'requires' }],
  createdBy: author, createdAt: '',
}

beforeEach(() => {
  mocks.saveRevision.mockReset()
  mocks.pages = [page, { ...page, id: 'page-2', kind: 'concept', conceptKind: 'noun', title: 'Concept' }, { ...page, id: 'page-3', kind: 'concept', conceptKind: 'verb', title: 'Action' }]
})

it('edits structured content and saves an immutable revision', async () => {
  const user = userEvent.setup()
  let resolve!: () => void
  mocks.saveRevision.mockReturnValue(new Promise<void>((next) => { resolve = next }))
  const onSaved = vi.fn().mockResolvedValue(undefined)
  const onCancel = vi.fn()
  const { container } = render(<PageEditor page={page} revision={revision} onCancel={onCancel} onSaved={onSaved} />)

  await user.clear(screen.getByDisplayValue('Scenario'))
  await user.type(screen.getByLabelText('Názov'), 'Updated scenario')
  await user.clear(screen.getByRole('textbox', { name: 'Obsah stránky' }))
  await user.type(screen.getByRole('textbox', { name: 'Obsah stránky' }), 'Updated body')
  fireEvent.change(screen.getByRole('textbox', { name: 'Text kroku 1' }), { target: { value: 'updated condition' } })
  await user.clear(screen.getByRole('textbox', { name: 'Typ vzťahu 1' }))
  await user.type(screen.getByRole('textbox', { name: 'Typ vzťahu 1' }), 'requires')
  await user.click(screen.getByRole('button', { name: 'Cieľ vzťahu 1' }))
  await user.click(screen.getByRole('option', { name: 'Action' }))
  await user.clear(screen.getByLabelText('Alias názvy'))
  await user.type(screen.getByLabelText('Alias názvy'), 'one, two, , three')
  await user.click(screen.getByRole('button', { name: 'Pridať vzťah' }))
  expect(screen.getAllByRole('button', { name: 'Odstrániť vzťah' })).toHaveLength(3)
  await user.click(screen.getAllByRole('button', { name: 'Odstrániť vzťah' })[0])

  await user.click(screen.getByRole('button', { name: 'Uložiť novú revíziu' }))
  expect(screen.getByRole('button', { name: /Ukladám/ })).toBeDisabled()
  resolve()
  await waitFor(() => expect(onSaved).toHaveBeenCalledOnce())
  expect(mocks.saveRevision).toHaveBeenCalledWith('page-1', 'revision-1', expect.objectContaining({
    title: 'Updated scenario', bodyMd: 'Updated body', aliases: ['one', 'two', 'three'],
    steps: [expect.objectContaining({ text: 'updated condition' })],
  }))
  expect(container.querySelector('.page-editor')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Zavrieť editor' }))
  await user.click(screen.getByRole('button', { name: 'Zrušiť' }))
  expect(onCancel).toHaveBeenCalledTimes(2)
})

it('disables adding relationships without targets and reports conflict, errors, and fallback failures', async () => {
  const user = userEvent.setup()
  mocks.pages = [page]
  const onSaved = vi.fn().mockResolvedValue(undefined)
  const view = render(<PageEditor page={{ ...page, kind: 'concept' }} revision={{ ...revision, steps: [], references: [] }} onCancel={vi.fn()} onSaved={onSaved} />)
  expect(screen.getByRole('button', { name: 'Pridať vzťah' })).toBeDisabled()

  const { APIError } = await import('../../api/client')
  mocks.saveRevision.mockRejectedValueOnce(new APIError(409, 'revision_conflict', 'Conflict'))
  fireEvent.submit(view.container.querySelector('form')!)
  expect(await screen.findByRole('alert')).toHaveTextContent('Medzitým vznikla novšia revízia')

  mocks.saveRevision.mockRejectedValueOnce(new Error('offline'))
  fireEvent.submit(view.container.querySelector('form')!)
  expect(await screen.findByRole('alert')).toHaveTextContent('offline')

  mocks.saveRevision.mockRejectedValueOnce('failure')
  fireEvent.submit(view.container.querySelector('form')!)
  expect(await screen.findByRole('alert')).toHaveTextContent('Draft sa nepodarilo uložiť.')
  expect(onSaved).not.toHaveBeenCalled()
})
