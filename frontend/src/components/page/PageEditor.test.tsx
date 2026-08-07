import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'
import type { Page, Revision } from '../../api/types'
import { PageEditor } from './PageEditor'

const mocks = vi.hoisted(() => ({ saveRevision: vi.fn(), stepDefinitions: vi.fn() }))
vi.mock('../../api/client', async () => {
  const actual = await vi.importActual<typeof import('../../api/client')>('../../api/client')
  return { ...actual, api: { ...actual.api, saveRevision: mocks.saveRevision, stepDefinitions: mocks.stepDefinitions } }
})

const author = { id: 'user-1', email: 'a@b.c', displayName: 'Matej', createdAt: '' }
const page: Page = { id: 'page-1', kind: 'scenario', parentId: 'feature-1', slug: 'scenario', title: 'Scenario', approved: true, hasDraft: false, unresolvedObjections: 0, createdAt: '', updatedAt: '' }
const revision: Revision = {
  id: 'revision-1', pageId: page.id, number: 1, status: 'approved', title: 'Scenario', bodyMd: 'Body',
  steps: [{ stableId: 'step-1', keyword: 'given', text: 'condition' }],
  references: [{ targetPageId: 'page-2', targetTitle: 'Concept', relation: 'uses' }, { targetPageId: 'page-3', targetTitle: 'Action', relation: 'requires' }],
  createdBy: author, createdAt: '',
}

beforeEach(() => {
  mocks.saveRevision.mockReset()
  mocks.stepDefinitions.mockReset().mockResolvedValue({ definitions: [] })
})

it('loads approved step definitions for scenario editing', async () => {
  mocks.stepDefinitions.mockResolvedValue({ definitions: [{
    id: 'definition-1', expression: 'zákazník má zmluvu', role: 'context', approved: true, usageCount: 3,
  }] })
  render(<PageEditor page={page} revision={{
    ...revision,
    steps: [{ keyword: 'given', definitionId: 'definition-1', expression: 'zákazník má zmluvu', arguments: [], text: 'zákazník má zmluvu' }],
  }} onCancel={vi.fn()} onSaved={vi.fn()} />)

  expect(await screen.findByText('Použité v 3 scenároch')).toBeInTheDocument()
  expect(mocks.stepDefinitions).toHaveBeenCalledWith()
  expect(screen.queryByRole('textbox', { name: 'Text kroku 1' })).not.toBeInTheDocument()
})

it('does not show the structured relationships block in the page editor', () => {
  const { container } = render(<PageEditor page={{ ...page, kind: 'concept' }} revision={revision} onCancel={vi.fn()} onSaved={vi.fn()} />)

  expect(container.querySelector('.reference-editor')).not.toBeInTheDocument()
})

it('does not expose alias editing', () => {
  render(<PageEditor page={page} revision={revision} onCancel={vi.fn()} onSaved={vi.fn()} />)

  expect(screen.queryByLabelText('Alias názvy')).not.toBeInTheDocument()
})

it('edits structured content and saves an immutable revision', async () => {
  const user = userEvent.setup()
  let resolve!: () => void
  mocks.saveRevision.mockReturnValue(new Promise<void>((next) => { resolve = next }))
  const onSaved = vi.fn().mockResolvedValue(undefined)
  const onCancel = vi.fn()
  const { container } = render(<PageEditor page={page} revision={revision} onCancel={onCancel} onSaved={onSaved} />)

  expect(container.querySelector('.editor-header')).not.toBeInTheDocument()
  expect(screen.queryByText('Nová nemenná verzia')).not.toBeInTheDocument()
  expect(screen.queryByRole('heading', { name: 'Upraviť obsah' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Zavrieť editor' })).not.toBeInTheDocument()
  await user.clear(screen.getByDisplayValue('Scenario'))
  await user.type(screen.getByLabelText('Názov'), 'Updated scenario')
  await user.clear(screen.getByRole('textbox', { name: 'Obsah stránky' }))
  await user.type(screen.getByRole('textbox', { name: 'Obsah stránky' }), 'Updated body')
  fireEvent.change(screen.getByRole('textbox', { name: 'Nová definícia kroku 1' }), { target: { value: 'updated condition' } })

  await user.click(screen.getByRole('button', { name: 'Uložiť novú verziu' }))
  expect(screen.getByRole('button', { name: /Ukladám/ })).toBeDisabled()
  resolve()
  await waitFor(() => expect(onSaved).toHaveBeenCalledOnce())
  expect(mocks.saveRevision).toHaveBeenCalledWith('page-1', 'revision-1', expect.objectContaining({
    title: 'Updated scenario', bodyMd: 'Updated body',
    steps: [expect.objectContaining({ expression: 'updated condition', text: 'updated condition' })],
    references: revision.references,
  }))
  expect(container.querySelector('.page-editor')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Zrušiť' }))
  expect(onCancel).toHaveBeenCalledOnce()
})

it('reports conflict, errors, and fallback failures', async () => {
  const onSaved = vi.fn().mockResolvedValue(undefined)
  const view = render(<PageEditor page={{ ...page, kind: 'concept' }} revision={{ ...revision, steps: [], references: [] }} onCancel={vi.fn()} onSaved={onSaved} />)

  const { APIError } = await import('../../api/client')
  mocks.saveRevision.mockRejectedValueOnce(new APIError(409, 'revision_conflict', 'Conflict'))
  fireEvent.submit(view.container.querySelector('form')!)
  expect(await screen.findByRole('alert')).toHaveTextContent('Medzitým vznikla novšia verzia')

  mocks.saveRevision.mockRejectedValueOnce(new Error('offline'))
  fireEvent.submit(view.container.querySelector('form')!)
  expect(await screen.findByRole('alert')).toHaveTextContent('offline')

  mocks.saveRevision.mockRejectedValueOnce('failure')
  fireEvent.submit(view.container.querySelector('form')!)
  expect(await screen.findByRole('alert')).toHaveTextContent('Draft sa nepodarilo uložiť.')
  expect(onSaved).not.toHaveBeenCalled()
})

it('keeps scenario editing usable when the definition catalog is unavailable', async () => {
  mocks.stepDefinitions.mockRejectedValue(new Error('catalog offline'))
  render(<PageEditor page={page} revision={revision} onCancel={vi.fn()} onSaved={vi.fn()} />)

  await waitFor(() => expect(mocks.stepDefinitions).toHaveBeenCalledOnce())
  expect(screen.getByRole('textbox', { name: 'Nová definícia kroku 1' })).toHaveValue('condition')
})
