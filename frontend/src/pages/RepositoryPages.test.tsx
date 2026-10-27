import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { Router } from '../router'
import type { Page, PageDetail, RepositorySource, Revision, WorkspaceSyncStatus } from '../api/types'
import { LibraryPage } from './LibraryPage'
import { PagePage } from './PagePage'
import { StatusBadge } from '../components/ui'

const mocks = vi.hoisted(() => ({ page: vi.fn(), revision: vi.fn(), openNewPage: vi.fn(), pages: [] as Page[], loading: false, status: null as WorkspaceSyncStatus | null }))
vi.mock('../api/client', () => ({ api: { page: mocks.page, revision: mocks.revision } }))
vi.mock('../workspace', () => ({ useWorkspace: () => ({ pages: mocks.pages, loadingPages: mocks.loading, syncStatus: mocks.status, readOnly: true, openNewPage: mocks.openNewPage }) }))
const timestamp = '2026-10-26T12:00:00Z'
const source: RepositorySource = { externalId: 'VOZ-SC-CONSENT-002', repository: 'https://github.com/matk0/vozokany', commit: '1'.repeat(40), contentHash: 'sha256:' + 'a'.repeat(64), path: 'specs/business/consent.feature', content: 'Feature: Exact English source\n', runId: '10', runAttempt: 1, verifiedCommit: '1'.repeat(40), result: 'passed', importedAt: timestamp }
const page: Page = { id: 'page', kind: 'scenario', parentId: 'feature', title: 'Preserve withdrawal', slug: 'VOZ-SC-CONSENT-002', approved: false, hasDraft: false, unresolvedObjections: 0, repositoryRevisionId: 'current', repositorySource: source, createdAt: timestamp, updatedAt: timestamp }
const version: Revision = { id: 'current', pageId: 'page', number: 1, title: page.title, status: 'repository_baseline', bodyMd: '<!-- business-id: VOZ-SC-CONSENT-002 -->\n<!-- relates-to: VOZ-CON-CONSENT -->\n# Preserve withdrawal\n\nOriginal English business prose.', steps: [{ keyword: 'given', text: 'a customer is withdrawn' }, { keyword: 'when', text: 'the email and group change' }, { keyword: 'then', text: 'the customer stays withdrawn' }], references: [{ targetPageId: 'concept', targetTitle: 'Consent', relation: 'uses' }], repositorySource: source, createdBy: { id: '', email: '', displayName: 'Repository baseline', createdAt: timestamp }, createdAt: timestamp }
const feature: Page = { ...page, id: 'feature', parentId: undefined, kind: 'feature', title: 'Customer consent' }
const concept: Page = { ...page, id: 'concept', parentId: undefined, kind: 'concept', title: 'Consent' }
const detail: PageDetail = { page, repositoryRevision: version, revisions: [version], comments: [], objections: [], children: [], reviewStates: [] }
beforeEach(() => {
  vi.clearAllMocks()
  mocks.status = { mode: 'git_managed', repository: source.repository, commit: source.commit, runId: '10', runAttempt: 1, pageCount: 3, scenarioCount: 1, stale: false }
  mocks.pages = [page, feature, concept]; mocks.loading = false
  mocks.page.mockResolvedValue(detail); mocks.revision.mockResolvedValue(version)
  window.history.replaceState({}, '', '/page/page')
})
it('uses original concept panels and feature cards with disabled writes and no repository widgets or approval claims', () => {
  const view = render(<Router><LibraryPage kind="concept" /></Router>)
  expect(screen.getByRole('link', { name: /^Consent/ })).toHaveAttribute('href', '/page/concept')
  expect(view.container.querySelector('.library-group')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Pridať koncept' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Filtrovať podľa stavu: Všetky' })).toBeDisabled()
  expect(view.container.querySelector('.status-badge')).toBeNull()
  expect(screen.queryByText(/Repository baseline|Canonical specifications|Passed at/)).toBeNull()
  fireEvent.change(screen.getByRole('textbox'), { target: { value: 'nothing' } })
  expect(screen.getByRole('heading', { name: 'Nič sa nenašlo' })).toBeVisible()
  view.unmount()
  const features = render(<Router><LibraryPage kind="feature" /></Router>)
  expect(screen.getByText('1 scenár')).toBeVisible()
  expect(screen.getByRole('link', { name: 'Preserve withdrawal' })).toHaveAttribute('href', '/page/page')
  expect(features.container.querySelector('.feature-card')).toBeVisible()
  expect(features.container.querySelector('.draft-dot, .status-badge')).toBeNull()
  expect(screen.getByRole('button', { name: 'Pridať funkciu' })).toBeDisabled()
})
it('uses the original document reader, Gherkin steps, relationships and history without technical metadata', async () => {
  const view = render(<Router><PagePage pageId="page" /></Router>)
  await screen.findByRole('heading', { name: page.title })
  expect(screen.getAllByRole('heading', { name: page.title })).toHaveLength(1)
  expect(screen.getByText('Original English business prose.')).toBeVisible()
  expect(view.container.querySelector('.document-page .bdd-steps')).toHaveTextContent('the customer stays withdrawn')
  expect(screen.getByRole('img', { name: 'Scenár · Len na čítanie' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Nová verzia' })).toBeDisabled()
  expect(screen.queryByRole('button', { name: 'Schváliť' })).toBeNull()
  expect(view.container.querySelector('.document-tools')).toBeNull()
  expect(screen.queryByText(/Repository baseline|Source SHA|business-id|Exact canonical source|Immutable business versions/)).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Súvisiace stránky' }))
  expect(screen.getByRole('link', { name: /Consent/ })).toHaveAttribute('href', '/page/concept')
  expect(screen.getByRole('link', { name: 'Customer consent' })).toHaveAttribute('href', '/page/feature')
  fireEvent.click(screen.getByRole('button', { name: 'História' }))
  expect(screen.getByRole('dialog', { name: 'História verzií' })).toBeVisible()
  expect(screen.queryByText('Repository baseline')).toBeNull()
  expect(screen.getAllByText('the customer stays withdrawn')).toHaveLength(2)
  fireEvent.click(screen.getByRole('button', { name: /Verzia #1/ }))
  await waitFor(() => expect(mocks.revision).toHaveBeenCalledWith('current'))
  fireEvent.click(screen.getByRole('button', { name: 'Zavrieť' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(version.bodyMd).toContain('<!-- business-id: VOZ-SC-CONSENT-002 -->')
})
it('retains feature children, archived records and minimal imported prose without granting write access', async () => {
  mocks.page.mockResolvedValue({ ...detail, page: { ...feature, archived: true }, repositoryRevision: { ...version, pageId: 'feature', title: feature.title, bodyMd: 'Feature prose.', steps: [], references: [] }, children: [page] })
  render(<Router><PagePage pageId="feature" /></Router>)
  await screen.findByRole('heading', { name: feature.title })
  expect(screen.getByRole('status')).toHaveTextContent('Táto stránka je archivovaná.')
  expect(screen.getByRole('link', { name: /Preserve withdrawal/ })).toHaveAttribute('href', '/page/page')
  expect(screen.getByRole('button', { name: 'Pridať scenár' })).toBeDisabled()
  expect(screen.getByText('Feature prose.')).toBeVisible()
})
it('opens an exact Q&A citation and rejects a revision belonging to another page', async () => {
  window.history.replaceState({}, '', '/page/page?revision=historic')
  mocks.revision.mockResolvedValue({ ...version, id: 'historic', title: 'Historic title' })
  const view = render(<Router><PagePage pageId="page" /></Router>)
  await screen.findByRole('heading', { name: 'Historic title' })
  expect(mocks.revision).toHaveBeenCalledWith('historic')
  view.unmount()
  mocks.revision.mockResolvedValue({ ...version, pageId: 'other' })
  render(<Router><PagePage pageId="page" /></Router>)
  await screen.findByRole('heading', { name: 'Stránku sa nepodarilo načítať.' })
  expect(screen.queryByRole('heading', { name: page.title })).toBeNull()
})
it('refreshes the current source after automatic sync and ignores superseded page responses and errors', async () => {
  let resolve!: (value: PageDetail) => void
  let reject!: (reason: Error) => void
  mocks.page.mockImplementationOnce(() => new Promise<PageDetail>(done => { resolve = done }))
  const view = render(<Router><PagePage pageId="page" /></Router>)
  mocks.page.mockResolvedValue({ ...detail, page: { ...page, id: 'other' }, repositoryRevision: { ...version, pageId: 'other', title: 'New page' } })
  view.rerender(<Router><PagePage pageId="other" /></Router>)
  await screen.findByRole('heading', { name: 'New page' })
  await act(async () => resolve(detail))
  expect(screen.getByRole('heading', { name: 'New page' })).toBeVisible()
  mocks.page.mockImplementationOnce(() => new Promise<PageDetail>((_, fail) => { reject = fail }))
  mocks.status = { ...mocks.status!, runId: '11' }
  view.rerender(<Router><PagePage pageId="other" /></Router>)
  mocks.status = { ...mocks.status!, runId: '12' }
  view.rerender(<Router><PagePage pageId="other" /></Router>)
  await screen.findByRole('heading', { name: 'New page' })
  await act(async () => reject(new Error('late failure')))
  expect(screen.getByRole('heading', { name: 'New page' })).toBeVisible()
})
it('does not turn imported pages into draft or approved badges', () => {
  const view = render(<StatusBadge page={page} />)
  expect(view.container).toBeEmptyDOMElement()
  view.rerender(<StatusBadge status="repository_baseline" />)
  expect(view.container).toBeEmptyDOMElement()
})
