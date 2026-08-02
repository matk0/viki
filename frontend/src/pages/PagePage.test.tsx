import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PageDetail, Revision } from '../api/types'
import { Router } from '../router'
import styles from '../styles.css?raw'
import { PagePage } from './PagePage'

const mocks = vi.hoisted(() => ({
  page: vi.fn(),
  saveRevision: vi.fn(),
  revision: vi.fn(),
  comment: vi.fn(),
  raiseObjection: vi.fn(),
  resolveObjection: vi.fn(),
  approve: vi.fn(),
  reloadPages: vi.fn(),
  openNewPage: vi.fn(),
  pages: [] as PageDetail['page'][],
}))

vi.mock('../api/client', () => ({
  api: { page: mocks.page, saveRevision: mocks.saveRevision, revision: mocks.revision, comment: mocks.comment, raiseObjection: mocks.raiseObjection, resolveObjection: mocks.resolveObjection, approve: mocks.approve },
}))

vi.mock('../workspace', () => ({
  useWorkspace: () => ({ pages: mocks.pages, reloadPages: mocks.reloadPages, openNewPage: mocks.openNewPage }),
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
        steps: [],
    references: [],
    createdBy: author,
    createdAt: '2026-07-30T10:00:00Z',
  }
}

const approved = revision('approved-revision', 1, 'approved', 'Schválené pravidlo')
const draft = revision('draft-revision', 2, 'draft', 'Presný draft')
const detail: PageDetail = {
  page: {
    id: 'page-1',
    kind: 'concept',
    conceptKind: 'noun',
    slug: 'pravidlo',
    title: 'Pravidlo',
    approvedRevisionId: approved.id,
    latestDraftRevisionId: draft.id,
    approved: true,
    hasDraft: true,
    unresolvedObjections: 0,
    createdAt: '2026-07-30T10:00:00Z',
    updatedAt: '2026-07-30T10:00:00Z',
  },
  approvedRevision: approved,
  draftRevision: draft,
  revisions: [],
  comments: [],
  objections: [],
  children: [],
  reviewStates: [
    { revisionId: approved.id, state: 'approved', blockers: [] },
    { revisionId: draft.id, state: 'ready', blockers: [] },
  ],
}

describe('PagePage revision links', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.pages = []
    mocks.page.mockResolvedValue(detail)
    mocks.saveRevision.mockResolvedValue(undefined)
    mocks.revision.mockResolvedValue(approved)
    mocks.reloadPages.mockResolvedValue(undefined)
    window.history.replaceState({}, '', '/page/page-1?revision=draft-revision')
  })

  it('opens the exact current revision named by an assistant citation', async () => {
    const { container } = render(<Router><PagePage pageId="page-1" /></Router>)

    const heading = await screen.findByRole('heading', { name: 'Presný draft' })
    expect(heading.closest('.document-title-main')?.querySelector('.document-icon')).toBeNull()
    expect(heading.closest('.document-title-row')?.querySelector(':scope > .document-icon')).not.toBeNull()
    expect(screen.getByRole('button', { name: 'Zmeniť' }).matches('.document-header-actions > .document-edit-button')).toBe(true)
    expect(screen.getByRole('tab', { name: 'Draft #2' })).toHaveAttribute('aria-selected', 'true')
    expect(container.querySelector('.review-summary')).not.toBeInTheDocument()
    expect(container.querySelector('.vote-counts')).not.toBeInTheDocument()
    expect(screen.queryByText('Pripravené na schválenie')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Vzniesť námietku' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Schváliť' })).toBeEnabled()
  })

  it('edits the current draft but prevents creating a second draft from the approved version', async () => {
    const user = userEvent.setup()
    const { container } = render(<Router><PagePage pageId="page-1" /></Router>)

    await screen.findByRole('heading', { name: 'Presný draft' })
    const draftAction = within(container.querySelector<HTMLElement>('.document-header-actions')!).getByRole('button', { name: 'Zmeniť' })
    expect(draftAction).toBeEnabled()
    expect(draftAction.querySelector('svg')).toHaveClass('lucide-pencil')

    await user.click(screen.getByRole('tab', { name: 'Schválené' }))

    const approvedAction = within(container.querySelector<HTMLElement>('.document-header-actions')!).getByRole('button', { name: 'Nová verzia' })
    expect(approvedAction).toBeDisabled()
    const reviewPanel = container.querySelector<HTMLElement>('.review-panel')!
    const approvedStatus = within(reviewPanel).getByText('Schválené').closest<HTMLElement>('[role="listitem"]')!
    expect(within(approvedStatus).queryByRole('button', { name: 'Nová verzia' })).not.toBeInTheDocument()
  })

  it('merges page kind and revision status into one state-colored header icon', async () => {
    const user = userEvent.setup()
    render(<Router><PagePage pageId="page-1" /></Router>)

    const draftHeading = await screen.findByRole('heading', { name: 'Presný draft' })
    const draftRow = draftHeading.closest<HTMLElement>('.document-title-row')!
    const draftIcon = within(draftRow).getByRole('img', { name: 'Koncept · Draft' })
    expect(draftIcon).toHaveClass('document-icon', 'concept', 'draft')
    expect(draftIcon.parentElement).toBe(draftRow)
    expect(draftIcon.querySelector('svg')).toHaveClass('lucide-box')
    expect(within(draftRow).queryByText('Draft')).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Schválené' }))
    const approvedHeading = screen.getByRole('heading', { name: 'Schválené pravidlo' })
    const approvedRow = approvedHeading.closest<HTMLElement>('.document-title-row')!
    const approvedIcon = within(approvedRow).getByRole('img', { name: 'Koncept · Schválené' })
    expect(approvedIcon).toHaveClass('document-icon', 'concept', 'approved')
    expect(within(approvedRow).queryByText('Schválené')).not.toBeInTheDocument()
  })

  it('keeps the review panel visible with the current approved status', async () => {
    const user = userEvent.setup()
    const { container } = render(<Router><PagePage pageId="page-1" /></Router>)
    await screen.findByRole('heading', { name: 'Presný draft' })

    await user.click(screen.getByRole('tab', { name: 'Schválené' }))
    const tools = container.querySelector<HTMLElement>('.document-tools')!
    const reviewPanel = tools.querySelector<HTMLElement>('.review-panel')!
    const discussionPanel = tools.querySelector<HTMLElement>('.discussion-panel')!
    expect(Array.from(tools.children)).toEqual([reviewPanel, discussionPanel])
    expect(within(reviewPanel).getByRole('heading', { name: 'Kontrola' })).toBeVisible()
    expect(within(reviewPanel).queryByText('Aktuálny stav')).not.toBeInTheDocument()
    expect(within(reviewPanel).getByText('Zablokované').closest('[role="listitem"]')).toHaveAttribute('aria-disabled', 'true')
    expect(within(reviewPanel).getByText('Draft').closest('[role="listitem"]')).toHaveAttribute('aria-disabled', 'true')
    expect(styles).toMatch(/\.review-status-line\.disabled\s*\{[^}]*background:\s*var\(--soft\);[^}]*color:\s*var\(--faint\);/s)
    const approvedStatus = within(reviewPanel).getByText('Schválené').closest<HTMLElement>('[role="listitem"]')!
    expect(approvedStatus).toHaveAttribute('aria-current', 'step')
    const headerVersionButton = screen.getByRole('button', { name: 'Nová verzia' })
    expect(headerVersionButton).toBeDisabled()
    expect(within(approvedStatus).queryByRole('button', { name: 'Nová verzia' })).not.toBeInTheDocument()
    expect(within(discussionPanel).getByText('Diskusia (0)')).toBeVisible()
    expect(discussionPanel.querySelector('details')).not.toHaveAttribute('open')
  })

  it('offers a new version from both approved-page actions when no draft exists', async () => {
    const user = userEvent.setup()
    mocks.page.mockResolvedValue({
      ...detail,
      page: { ...detail.page, latestDraftRevisionId: undefined, hasDraft: false },
      draftRevision: undefined,
      reviewStates: [{ revisionId: approved.id, state: 'approved', blockers: [] }],
    })
    window.history.replaceState({}, '', '/page/page-1?revision=approved-revision')
    const { container } = render(<Router><PagePage pageId="page-1" /></Router>)

    await screen.findByRole('heading', { name: 'Schválené pravidlo' })
    const headerVersionButton = within(container.querySelector<HTMLElement>('.document-header-actions')!).getByRole('button', { name: 'Nová verzia' })
    expect(headerVersionButton).toBeEnabled()
    expect(headerVersionButton.querySelector('svg')).toHaveClass('lucide-plus')
    const approvedStatus = within(container.querySelector<HTMLElement>('.review-panel')!).getByText('Schválené').closest<HTMLElement>('[role="listitem"]')!
    const statusVersionButton = within(approvedStatus).getByRole('button', { name: 'Nová verzia' })
    expect(statusVersionButton).toHaveClass('review-new-version')
    expect(styles).toMatch(/\.review-new-version\s*\{[^}]*opacity:\s*0;[^}]*transform:\s*translateX\(5px\);[^}]*pointer-events:\s*none;/s)
    expect(styles).toMatch(/\.review-new-version\s*\{[^}]*cursor:\s*pointer;/s)
    expect(styles).toMatch(/\.review-status-line\.approved\.current:hover \.review-new-version,[^{]*\{[^}]*opacity:\s*1;[^}]*transform:\s*translateX\(0\);[^}]*pointer-events:\s*auto;/s)

    await user.click(statusVersionButton)
    expect(screen.getByRole('button', { name: 'Uložiť novú verziu' })).toBeVisible()
  })

  it('keeps draft status actions inside the review card before the closed discussion panel', async () => {
    const { container } = render(<Router><PagePage pageId="page-1" /></Router>)
    await screen.findByRole('heading', { name: 'Presný draft' })

    const tools = container.querySelector<HTMLElement>('.document-tools')!
    const panels = Array.from(tools.children)
    expect(panels).toHaveLength(2)
    expect(panels[0]).toHaveClass('panel', 'review-panel')
    expect(panels[1]).toHaveClass('panel', 'discussion-panel')
    const actions = panels[0].querySelector<HTMLElement>('.review-status-actions')!
    expect(within(panels[0] as HTMLElement).getByRole('heading', { name: 'Kontrola' }).querySelector('svg')).toHaveClass('lucide-clipboard-check')
    expect(panels[0].querySelector('.panel-heading p')).not.toBeInTheDocument()
    expect(panels[0]).not.toHaveTextContent('Námietky a schválenie.')
    expect(panels[0]).not.toHaveTextContent('Komentáre')
    expect(within(panels[0] as HTMLElement).queryByLabelText('Komentár')).not.toBeInTheDocument()
    expect(within(actions).getByRole('button', { name: 'Schváliť' })).toBeVisible()
    expect(within(actions).getByRole('button', { name: 'Vzniesť námietku' })).toBeVisible()
    expect(within(panels[1] as HTMLElement).getByText('Diskusia (0)')).toBeVisible()
    expect(panels[1].querySelector('details')).not.toHaveAttribute('open')
    expect(within(panels[1] as HTMLElement).getByLabelText('Komentár')).not.toBeVisible()
  })

  it('places matching history and edit controls together in the page header', async () => {
    render(<Router><PagePage pageId="page-1" /></Router>)
    await screen.findByRole('heading', { name: 'Presný draft' })

    const historyButton = screen.getByRole('button', { name: 'História' })
    const versionButton = screen.getByRole('button', { name: 'Zmeniť' })
    const actions = historyButton.closest('.document-header-actions')

    expect(actions).not.toBeNull()
    expect(versionButton.parentElement).toBe(actions)
    expect(Array.from(actions!.children)).toEqual([historyButton, versionButton])
    expect(historyButton).toHaveClass('secondary-button', 'document-header-button')
    expect(versionButton).toHaveClass('secondary-button', 'document-header-button')
    expect(historyButton.querySelector('svg')).not.toBeInTheDocument()
    expect(versionButton.querySelector('.lucide-pencil')).toBeInTheDocument()
    expect(document.querySelector('.document-tools .document-toolbar')).not.toBeInTheDocument()
  })

  it('does not expose illustrative metadata in the page or editor', async () => {
    mocks.page.mockResolvedValue({
      ...detail,
      draftRevision: { ...draft, illustrative: true } as Revision,
    })
    render(<Router><PagePage pageId="page-1" /></Router>)

    expect(await screen.findByRole('heading', { name: 'Presný draft' })).toBeInTheDocument()
    expect(screen.queryByText('Ilustračné pravidlá')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Zmeniť' }))
    expect(screen.queryByText('Obsahuje ilustračné, neoverené pravidlá')).not.toBeInTheDocument()
  })

  it('links concept mentions inline and keeps related pages in a closed accordion', async () => {
    const customer = {
      ...detail.page,
      id: 'customer-page',
      kind: 'concept' as const,
      title: 'zakaznik',
      slug: 'zakaznik',
    }
    const availability = {
      ...detail.page,
      id: 'availability-page',
      kind: 'feature' as const,
      conceptKind: undefined,
      title: 'Overenie dostupnosti',
      slug: 'overenie-dostupnosti',
    }
    const scenarioRevision: Revision = {
      ...approved,
      pageId: 'scenario-page',
      title: 'Rezervácia služby',
      bodyMd: 'Zákazník pokračuje podľa procesu overenia dostupnosti.',
      references: [
        { targetPageId: customer.id, targetTitle: 'zakaznik', relation: 'uses' },
        { targetPageId: availability.id, targetTitle: availability.title, relation: 'relates_to' },
      ],
    }
    mocks.pages = [customer, availability]
    mocks.page.mockResolvedValue({
      ...detail,
      page: { ...availability, id: 'scenario-page', title: scenarioRevision.title },
      approvedRevision: scenarioRevision,
      draftRevision: undefined,
    })
    window.history.replaceState({}, '', '/page/scenario-page')

    const { container } = render(<Router><PagePage pageId="scenario-page" /></Router>)

    const customerMention = await screen.findByRole('link', { name: 'Zákazník' })
    expect(customerMention).toHaveAttribute('href', '/page/customer-page')
    expect(customerMention.closest('.document-body')).not.toBeNull()

    const relatedToggle = screen.getByRole('button', { name: 'Súvisiace stránky' })
    expect(relatedToggle).toHaveAttribute('aria-expanded', 'false')
    expect(container.querySelector('.reference-list')).not.toBeVisible()

    await userEvent.click(relatedToggle)
    expect(relatedToggle).toHaveAttribute('aria-expanded', 'true')
    expect(container.querySelector('.reference-list')).toBeVisible()

    const customerReference = container.querySelector('.reference-list a[href="/page/customer-page"]')
    const scenarioReference = container.querySelector('.reference-list a[href="/page/availability-page"]')
    expect(customerReference?.querySelector('svg')).toHaveClass('lucide-box')
    expect(scenarioReference?.querySelector('svg')).toHaveClass('lucide-workflow')
    expect(container.querySelector('.document-icon.feature svg')).toHaveClass('lucide-workflow')
  })

  it.each([
    { status: 'queued' as const, label: 'Čaká na vývoj', icon: 'lucide-clock-3' },
    { status: 'running' as const, label: 'Vo vývoji', icon: 'lucide-hammer' },
    { status: 'developed' as const, label: 'Vyvinuté', icon: 'lucide-circle-check' },
    { status: 'blocked' as const, label: 'Vyžaduje zásah', icon: 'lucide-triangle-alert' },
  ])('shows the approved scenario $status development status without changing its revision status', async ({ status, label, icon }) => {
    const scenarioPage = { ...detail.page, id: 'scenario-page', kind: 'scenario' as const, parentId: 'feature-page', conceptKind: undefined, title: 'Podpis zmluvy' }
    const scenarioRevision = { ...approved, id: 'scenario-revision', pageId: scenarioPage.id, title: scenarioPage.title }
    mocks.page.mockResolvedValue({
      ...detail,
      page: { ...scenarioPage, approvedRevisionId: scenarioRevision.id, latestDraftRevisionId: undefined, hasDraft: false },
      approvedRevision: scenarioRevision,
      draftRevision: undefined,
      development: { revisionId: scenarioRevision.id, status, detail: '', updatedAt: '2026-08-02T10:00:00Z' },
      reviewStates: [{ revisionId: scenarioRevision.id, state: 'approved', blockers: [] }],
    })
    window.history.replaceState({}, '', '/page/scenario-page')

    const { container } = render(<Router><PagePage pageId="scenario-page" /></Router>)

    const developmentState = await screen.findByText(label)
    const statusList = container.querySelector<HTMLElement>('.review-status-list')!
    expect(statusList).toContainElement(developmentState)
    expect(statusList.lastElementChild).toHaveTextContent(label)
    expect(statusList.lastElementChild?.querySelector('svg')).toHaveClass(icon)
    expect(container.querySelector('.document-header .development-status')).not.toBeInTheDocument()
    expect(container.querySelector('.document-icon.approved')).toBeInTheDocument()
  })

  it('places related pages at the bottom of a feature article', async () => {
    const featurePage = { ...detail.page, id: 'feature-page', kind: 'feature' as const, conceptKind: undefined, title: 'Zmluvy' }
    const scenarioPage = { ...detail.page, id: 'scenario-page', kind: 'scenario' as const, parentId: featurePage.id, conceptKind: undefined, title: 'Podpis zmluvy' }
    const conceptPage = { ...detail.page, id: 'contract-page', kind: 'concept' as const, title: 'Zmluva', slug: 'zmluva' }
    const featureRevision: Revision = {
      ...approved,
      pageId: featurePage.id,
      title: featurePage.title,
      references: [{ targetPageId: conceptPage.id, targetTitle: conceptPage.title, relation: 'uses' }],
    }
    mocks.pages = [featurePage, scenarioPage, conceptPage]
    mocks.page.mockResolvedValue({
      ...detail,
      page: featurePage,
      approvedRevision: featureRevision,
      draftRevision: undefined,
      children: [scenarioPage],
    })
    window.history.replaceState({}, '', '/page/feature-page')

    const { container } = render(<Router><PagePage pageId="feature-page" /></Router>)

    const relatedSection = (await screen.findByRole('button', { name: 'Súvisiace stránky' })).closest('section')
    const scenarioSection = screen.getByRole('heading', { name: 'Scenáre' }).closest('section')
    const article = container.querySelector('.document-page')
    expect(article?.lastElementChild).toBe(relatedSection)
    expect(relatedSection?.previousElementSibling).toBe(scenarioSection)
  })

  it('top-aligns the shared page icon and status with the document title', () => {
    expect(styles).toMatch(/\.document-title-row\s*>\s*\.document-icon\s*\{[^}]*align-self:\s*start;/s)
  })

  it('uses the document card top edge for the stacked sidebar panels', () => {
    expect(styles).toMatch(/\.document-tools\s*\{[^}]*top:\s*var\(--document-top\);[^}]*gap:\s*16px;[^}]*padding-top:\s*0;/s)
    expect(styles).toMatch(/\.document-layout\s*\{[^}]*--document-top:\s*48px;[^}]*padding:\s*var\(--document-top\)\s+0\s+90px;/s)
  })

  it('places a full-height vertical status rail inside the review card content row', () => {
    expect(styles).toMatch(/\.review-panel-body\.has-actions\s*\{[^}]*display:\s*grid;[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)\s+42px;[^}]*align-items:\s*stretch;[^}]*gap:\s*9px;/s)
    expect(styles).toMatch(/\.review-status-actions\s*\{[^}]*align-self:\s*stretch;[^}]*flex-direction:\s*column;/s)
    expect(styles).toMatch(/\.review-status-action\s*\{[^}]*height:\s*auto;[^}]*flex:\s*1;/s)
  })

  it('makes enabled status controls clickable and restores their stronger accent on hover', () => {
    expect(styles).toMatch(/--approval-control-accent:\s*#c7ed89;/i)
    expect(styles).toMatch(/--rejection-control-accent:\s*#ff8585;/i)
    expect(styles).toMatch(/\.review-status-action\s*\{[^}]*cursor:\s*pointer;/s)
    expect(styles).toMatch(/\.review-status-action\.approve:not\(:disabled\):hover\s*\{[^}]*background:\s*var\(--approval-accent\);/s)
    expect(styles).toMatch(/\.review-status-action\.object:not\(:disabled\):hover\s*\{[^}]*background:\s*var\(--rejection-accent\);/s)
    expect(styles).toMatch(/\.review-status-action:disabled\s*\{[^}]*cursor:\s*default;/s)
  })

  it('does not draw a divider below the draft review panel heading', () => {
    expect(styles).toMatch(/\.review-panel\s*>\s*\.panel-heading\.compact\s*\{[^}]*border-bottom:\s*0;/s)
  })

  it('does not retain the old embedded comments heading', () => {
    expect(styles).not.toMatch(/\.comments-heading\s*\{/)
  })

  it('does not add top margin to the inline page editor', () => {
    const inlineEditorRule = styles.match(/\.document-page\s*>\s*\.page-editor\s*\{([^}]*)\}/s)?.[1] ?? ''
    expect(inlineEditorRule).not.toMatch(/margin-top\s*:/)
  })

  it('shows loading and localized error states', async () => {
    let reject!: (reason: unknown) => void
    mocks.page.mockReturnValue(new Promise((_resolve, nextReject) => { reject = nextReject }))
    const view = render(<Router><PagePage pageId="page-1" /></Router>)
    expect(screen.getByText('Načítavam stránku…')).toBeVisible()

    reject(new Error('offline'))
    expect(await screen.findByText('offline')).toBeVisible()
    view.unmount()

    mocks.page.mockRejectedValue('failure')
    render(<Router><PagePage pageId="page-2" /></Router>)
    expect(await screen.findByRole('heading', { name: 'Stránku sa nepodarilo načítať.' })).toBeVisible()
  })

  it('selects available versions and explains an unavailable page version', async () => {
    const user = userEvent.setup()
    window.history.replaceState({}, '', '/page/page-1?revision=approved-revision')
    const view = render(<Router><PagePage pageId="page-1" /></Router>)
    expect(await screen.findByRole('heading', { name: 'Schválené pravidlo' })).toBeVisible()
    await user.click(screen.getByRole('tab', { name: 'Draft #2' }))
    expect(screen.getByRole('heading', { name: 'Presný draft' })).toBeVisible()
    await user.click(screen.getByRole('tab', { name: 'Schválené' }))
    expect(screen.getByRole('heading', { name: 'Schválené pravidlo' })).toBeVisible()
    view.unmount()

    mocks.page.mockResolvedValue({ ...detail, approvedRevision: undefined, draftRevision: undefined })
    window.history.replaceState({}, '', '/page/page-1')
    render(<Router><PagePage pageId="page-1" /></Router>)
    expect(await screen.findByRole('heading', { name: 'Verzia nie je dostupná' })).toBeVisible()
  })

  it('falls back to the approved revision for an unknown requested revision', async () => {
    window.history.replaceState({}, '', '/page/page-1?revision=unknown')
    render(<Router><PagePage pageId="page-1" /></Router>)
    expect(await screen.findByRole('heading', { name: 'Schválené pravidlo' })).toBeVisible()
  })

  it('defaults to a lone draft and renders Gherkin and references', async () => {
    const missing = { targetPageId: 'missing-page', targetTitle: 'External rule', relation: 'custom' }
    const scenarioPage = { ...detail.page, id: 'scenario-page', kind: 'scenario' as const, parentId: 'feature-page', conceptKind: undefined, title: 'Scenario' }
    const scenarioDraft: Revision = {
      ...draft, pageId: scenarioPage.id, bodyMd: '',
      steps: [{ stableId: 'step-1', keyword: 'given', text: 'condition' }, { stableId: 'step-2', keyword: 'then', text: 'result' }],
      references: [missing],
    }
    mocks.page.mockResolvedValue({
      ...detail,
      page: scenarioPage,
      approvedRevision: undefined,
      draftRevision: scenarioDraft,
      children: [],
      reviewStates: [{ revisionId: scenarioDraft.id, state: 'ready', blockers: [] }],
    })
    window.history.replaceState({}, '', '/page/scenario-page')
    const { container } = render(<Router><PagePage pageId="scenario-page" /></Router>)

    expect(await screen.findByRole('heading', { name: 'Presný draft' })).toBeVisible()
    expect(screen.getByText('Táto stránka zatiaľ nemá opis.')).toBeVisible()
    expect(screen.getByText('Pokiaľ')).toBeVisible()
    expect(screen.getByText('Potom')).toBeVisible()
    expect(screen.queryByRole('link', { name: /External rule/ })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Súvisiace stránky' }))
    expect(screen.getByRole('link', { name: /External rule/ })).toHaveAttribute('href', '/page/missing-page')
    expect(container.querySelector('.reference-list .lucide-book-open')).toBeInTheDocument()
  })

  it('links a scenario back to the feature it belongs to', async () => {
    const featureTitle = 'Overenie súhlasu zákazníka so spracovaním osobných údajov pri telefonáte'
    const featurePage = { ...detail.page, id: 'feature-page', kind: 'feature' as const, conceptKind: undefined, title: featureTitle }
    const scenarioPage = { ...detail.page, id: 'scenario-page', kind: 'scenario' as const, parentId: featurePage.id, conceptKind: undefined, title: 'Podpis zmluvy' }
    mocks.pages = [featurePage, scenarioPage]
    mocks.page.mockResolvedValue({ ...detail, page: scenarioPage, children: [] })
    window.history.replaceState({}, '', '/page/scenario-page')

    render(<Router><PagePage pageId="scenario-page" /></Router>)

    const parentLink = await screen.findByRole('link', { name: featureTitle })
    expect(parentLink).toHaveAttribute('href', '/page/feature-page')
    expect(parentLink).toHaveAttribute('title', featureTitle)
    expect(parentLink).toHaveClass('breadcrumb-parent')
    expect(styles).toMatch(/\.breadcrumb-parent\s*\{[^}]*flex:\s*1;[^}]*min-width:\s*0;/s)
    expect(styles).toMatch(/\.breadcrumb-parent-title\s*\{[^}]*overflow:\s*hidden;[^}]*text-overflow:\s*ellipsis;[^}]*white-space:\s*nowrap;/s)
  })

  it('explains why a scenario draft cannot be approved before its parent feature', async () => {
    const featurePage = { ...detail.page, id: 'feature-page', kind: 'feature' as const, conceptKind: undefined, title: 'Zmluvy', approved: false, hasDraft: true }
    const scenarioPage = { ...detail.page, id: 'scenario-page', kind: 'scenario' as const, parentId: featurePage.id, conceptKind: undefined, title: 'Podpis zmluvy', approved: false, hasDraft: true }
    const scenarioDraft = { ...draft, pageId: scenarioPage.id, title: scenarioPage.title }
    mocks.pages = [featurePage, scenarioPage]
    mocks.page.mockResolvedValue({
      ...detail,
      page: scenarioPage,
      approvedRevision: undefined,
      draftRevision: scenarioDraft,
      children: [],
      reviewStates: [{
        revisionId: scenarioDraft.id,
        state: 'blocked',
        blockers: [{ id: featurePage.id, type: 'parent_feature', relatedPageId: featurePage.id, relatedPageTitle: featurePage.title }],
      }],
    })
    window.history.replaceState({}, '', '/page/scenario-page')

    render(<Router><PagePage pageId="scenario-page" /></Router>)

    await screen.findByText('Najprv schváľte funkciu')
    fireEvent.click(screen.getByText('Zablokované'))
    expect(screen.getByText('Najprv schváľte funkciu')).toBeVisible()
    expect(screen.getByText('Scenár možno schváliť až po schválení funkcie „Zmluvy“.')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Schváliť' })).toBeDisabled()
  })

  it('creates independently versioned scenarios from their parent feature', async () => {
    const featurePage = { ...detail.page, id: 'feature-1', kind: 'feature' as const, conceptKind: undefined, title: 'Zmluvy' }
    mocks.page.mockResolvedValue({ ...detail, page: featurePage, children: [] })
    window.history.replaceState({}, '', '/page/feature-1')
    const user = userEvent.setup()

    render(<Router><PagePage pageId="feature-1" /></Router>)

    await user.click(await screen.findByRole('button', { name: 'Pridať scenár' }))
    expect(mocks.openNewPage).toHaveBeenCalledWith('scenario', 'feature-1')
  })

  it('lists independently versioned scenarios beneath their feature', async () => {
    const featurePage = { ...detail.page, id: 'feature-1', kind: 'feature' as const, conceptKind: undefined, title: 'Zmluvy' }
    const scenarioPage = { ...detail.page, id: 'scenario-1', kind: 'scenario' as const, parentId: featurePage.id, conceptKind: undefined, title: 'Podpis zmluvy' }
    mocks.page.mockResolvedValue({ ...detail, page: featurePage, children: [scenarioPage] })
    window.history.replaceState({}, '', '/page/feature-1')

    render(<Router><PagePage pageId="feature-1" /></Router>)

    expect(await screen.findByRole('link', { name: /Podpis zmluvy/ })).toHaveAttribute('href', '/page/scenario-1')
  })

  it('edits inline and saves through the current draft revision', async () => {
    const user = userEvent.setup()
    window.history.replaceState({}, '', '/page/page-1?revision=draft-revision')
    render(<Router><PagePage pageId="page-1" /></Router>)
    await screen.findByRole('heading', { name: 'Presný draft' })
    const headerActions = document.querySelector<HTMLElement>('.document-header-actions')!

    await user.click(within(headerActions).getByRole('button', { name: 'Zmeniť' }))
    await user.click(screen.getByRole('button', { name: 'Zrušiť' }))
    expect(within(headerActions).getByRole('button', { name: 'Zmeniť' })).toBeVisible()
    await user.click(within(headerActions).getByRole('button', { name: 'Zmeniť' }))
    fireEvent.submit(screen.getByRole('button', { name: 'Uložiť novú verziu' }).closest('form')!)

    await waitFor(() => expect(mocks.saveRevision).toHaveBeenCalledWith('page-1', 'draft-revision', expect.any(Object)))
    await waitFor(() => expect(mocks.reloadPages).toHaveBeenCalledOnce())
    expect(mocks.page).toHaveBeenCalledTimes(2)
    expect(screen.getByRole('tab', { name: 'Draft #2' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('heading', { name: 'Presný draft' })).toBeVisible()
  })

  it('opens and closes immutable revision history', async () => {
    const user = userEvent.setup()
    mocks.page.mockResolvedValue({ ...detail, revisions: [{ id: approved.id, number: 1, status: 'approved', title: approved.title, createdBy: author, createdAt: approved.createdAt }] })
    render(<Router><PagePage pageId="page-1" /></Router>)
    await screen.findByRole('heading', { name: 'Presný draft' })
    await user.click(screen.getByRole('button', { name: 'História' }))
    expect(screen.getByRole('heading', { name: 'História verzií' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Zavrieť' }))
    expect(screen.queryByRole('heading', { name: 'História verzií' })).not.toBeInTheDocument()
  })
})
