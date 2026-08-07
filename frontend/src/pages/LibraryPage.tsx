import { useMemo, useState } from 'react'
import { BookOpen, Filter, GitBranch, Plus, Search } from 'lucide-react'
import type { Page } from '../api/types'
import { Link } from '../router'
import { useWorkspace } from '../workspace'
import { PageIcon } from '../components/PageIcon'
import { VikiSelect } from '../components/VikiSelect'
import { EmptyState, kindLabel, StatusBadge } from '../components/ui'
import { useI18n } from '../i18n'

type StatusFilterValue = 'all' | 'approved' | 'draft'

export function LibraryPage({ kind }: { kind: 'concept' | 'feature' }) {
  const { t } = useI18n()
  const { pages, loadingPages, openNewPage } = useWorkspace()
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState<StatusFilterValue>('all')
  const children = useMemo(() => new Map(pages.filter((page) => page.kind === 'scenario').map((page) => [page.id, page])), [pages])
  const childGroups = useMemo(() => {
    const result = new Map<string, Page[]>()
    for (const child of children.values()) {
      if (!child.parentId) continue
      result.set(child.parentId, [...(result.get(child.parentId) ?? []), child])
    }
    return result
  }, [children])
  const relevant = useMemo(() => {
    const normalizedQuery = normalizeSearchText(query)
    return pages.filter((page) => {
      const kindMatches = kind === 'concept' ? page.kind === 'concept' : page.kind === 'feature'
      const queryMatches = normalizeSearchText(displayRevision(page, status).title).includes(normalizedQuery)
      const statusMatches = matchesStatus(page, status)
      const matchingScenario = kind === 'feature' && (childGroups.get(page.id) ?? []).some((child) => matchesStatus(child, status))
      return kindMatches && queryMatches && (statusMatches || matchingScenario)
    })
  }, [pages, kind, query, status, childGroups])
  const visibleChildGroups = useMemo(() => {
    const result = new Map<string, Page[]>()
    for (const [parentId, scenarios] of childGroups) {
      result.set(parentId, scenarios.filter((scenario) => matchesStatus(scenario, status)))
    }
    return result
  }, [childGroups, status])
  return <div className="page-container library-page">
    <header className="page-heading">
      <div><h1>{kind === 'concept' ? t('kind.concepts') : t('kind.features')}</h1><p>{kind === 'concept' ? t('library.conceptsDescription') : t('library.featuresDescription')}</p></div>
      <button className="primary-button" onClick={() => openNewPage(kind)}><Plus size={16} />{kind === 'concept' ? t('library.addConcept') : t('library.addFeature')}</button>
    </header>
    <div className="library-toolbar">
      <label className="search-field"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={kind === 'concept' ? t('library.searchConcepts') : t('library.searchFeatures')} /></label>
      <StatusFilter value={status} onChange={setStatus} />
    </div>
    {loadingPages ? <div className="skeleton-list" /> : relevant.length === 0 ? <EmptyState icon={kind === 'concept' ? <BookOpen /> : <GitBranch />} title={t('library.none')} body={t('library.noneBody')} /> : kind === 'concept' ? <ConceptList pages={relevant} status={status} /> : <FeatureList pages={relevant} children={visibleChildGroups} status={status} />}
  </div>
}

function matchesStatus(page: Page, status: StatusFilterValue): boolean {
  if (status === 'all') return true
  return status === 'draft' ? page.hasDraft : page.approved
}

function StatusFilter({ value, onChange }: { value: StatusFilterValue; onChange: (value: StatusFilterValue) => void }) {
  const { t } = useI18n()
  const statusOptions = [
    { value: 'all', label: t('library.all') },
    { value: 'approved', label: t('library.approved') },
    { value: 'draft', label: t('library.draft') },
  ] as const
  const selected = statusOptions.find((option) => option.value === value)!
  return <VikiSelect
    className="filter-select"
    ariaLabel={t('library.filterLabel', { status: selected.label })}
    listboxLabel={t('library.filterList')}
    value={value}
    options={statusOptions}
    leadingIcon={<Filter size={15} />}
    onChange={(next) => onChange(next as StatusFilterValue)}
  />
}

function normalizeSearchText(value: string): string {
  return value.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLocaleLowerCase('sk')
}

function displayRevision(page: Page, status: StatusFilterValue): { title: string; status: 'approved' | 'draft' } {
  if (status === 'approved' && page.approved) return { title: page.approvedRevisionTitle ?? page.title, status: 'approved' }
  if (status === 'draft' && page.hasDraft) return { title: page.draftRevisionTitle ?? page.title, status: 'draft' }
  if (page.hasDraft) return { title: page.draftRevisionTitle ?? page.title, status: 'draft' }
  if (page.approved) return { title: page.approvedRevisionTitle ?? page.title, status: 'approved' }
  return { title: page.title, status: 'draft' }
}

function pageRevisionHref(page: Page, status: 'approved' | 'draft'): string {
  if (status === 'draft' && page.latestDraftRevisionId) return `/page/${page.id}?revision=${page.latestDraftRevisionId}`
  return `/page/${page.id}`
}

function PageStatusPills({ page }: { page: Page }) {
  return <span className="page-status-pills">
    {page.approved && <StatusBadge status="approved" />}
    {(page.hasDraft || !page.approved) && <StatusBadge status="draft" />}
  </span>
}

function ConceptList({ pages, status }: { pages: Page[]; status: StatusFilterValue }) {
  const { t } = useI18n()
  const nouns = pages.filter((page) => page.conceptKind === 'noun')
  const verbs = pages.filter((page) => page.conceptKind === 'verb')
  return <div className="concept-columns"><ConceptGroup title={t('library.nouns')} pages={nouns} status={status} /><ConceptGroup title={t('library.verbs')} pages={verbs} status={status} /></div>
}

function ConceptGroup({ title, pages, status }: { title: string; pages: Page[]; status: StatusFilterValue }) {
  const { locale } = useI18n()
  return <section className="panel library-group"><div className="library-group-title"><h2>{title}</h2><span>{pages.length}</span></div><div className="compact-page-list">{pages.map((page) => {
    const display = displayRevision(page, status)
    return <Link to={pageRevisionHref(page, display.status)} key={page.id}><span className="compact-page-copy"><strong>{display.title}</strong><small>{kindLabel(page, locale)}</small></span><PageStatusPills page={page} /></Link>
  })}</div></section>
}

function FeatureList({ pages, children, status }: { pages: Page[]; children: Map<string, Page[]>; status: StatusFilterValue }) {
  const { t } = useI18n()
  return <div className="feature-list">{pages.map((page) => {
    const count = children.get(page.id)?.length ?? 0
    const display = displayRevision(page, status)
    return <section className="panel feature-card" key={page.id}><Link to={pageRevisionHref(page, display.status)} className="feature-card-heading"><LibraryPageIcon page={page} status={display.status} /><span><strong>{display.title}</strong><small>{count === 1 ? t('library.scenarioCount.one') : t('library.scenarioCount.other', { count })}</small></span><PageStatusPills page={page} /></Link><div className="feature-children">{(children.get(page.id) ?? []).map((child) => {
      const childDisplay = displayRevision(child, status)
      return <Link to={pageRevisionHref(child, childDisplay.status)} key={child.id}><span className="tree-line" /><PageIcon page={child} size={15} draft={childDisplay.status === 'draft'} /><span>{childDisplay.title}</span>{childDisplay.status === 'draft' && <i className="draft-dot" />}</Link>
    })}</div></section>
  })}</div>
}

function LibraryPageIcon({ page, status }: { page: Page; status: 'approved' | 'draft' }) {
  return <span className={`page-icon-box ${page.kind} ${status}`}><PageIcon page={page} draft={status === 'draft'} /></span>
}
