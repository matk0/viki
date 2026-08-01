import { useMemo, useState } from 'react'
import { BookOpen, Filter, GitBranch, Plus, Search } from 'lucide-react'
import type { Page } from '../api/types'
import { Link } from '../router'
import { useWorkspace } from '../workspace'
import { PageIcon } from '../components/PageIcon'
import { VikiSelect } from '../components/VikiSelect'
import { EmptyState, kindLabel, StatusBadge } from '../components/ui'
import { useI18n } from '../i18n'

type StatusFilterValue = 'all' | 'accepted' | 'draft'

export function LibraryPage({ kind }: { kind: 'concept' | 'feature' }) {
  const { t } = useI18n()
  const { pages, loadingPages, openNewPage } = useWorkspace()
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState<StatusFilterValue>('all')
  const relevant = useMemo(() => {
    const normalizedQuery = normalizeSearchText(query)
    return pages.filter((page) => {
      const kindMatches = kind === 'concept' ? page.kind === 'concept' : page.kind === 'feature'
      const queryMatches = normalizeSearchText(page.title).includes(normalizedQuery)
      const statusMatches = status === 'all' || (status === 'draft' ? page.hasDraft : page.accepted)
      return kindMatches && queryMatches && statusMatches
    })
  }, [pages, kind, query, status])
  const children = useMemo(() => new Map(pages.filter((page) => page.kind === 'scenario').map((page) => [page.id, page])), [pages])
  const childGroups = useMemo(() => {
    const result = new Map<string, Page[]>()
    for (const child of children.values()) {
      if (!child.parentId) continue
      result.set(child.parentId, [...(result.get(child.parentId) ?? []), child])
    }
    return result
  }, [children])
  return <div className="page-container library-page">
    <header className="page-heading">
      <div><h1>{kind === 'concept' ? t('kind.concepts') : t('kind.features')}</h1><p>{kind === 'concept' ? t('library.conceptsDescription') : t('library.featuresDescription')}</p></div>
      <button className="primary-button" onClick={() => openNewPage(kind)}><Plus size={16} />{kind === 'concept' ? t('library.addConcept') : t('library.addFeature')}</button>
    </header>
    <div className="library-toolbar">
      <label className="search-field"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={kind === 'concept' ? t('library.searchConcepts') : t('library.searchFeatures')} /></label>
      <StatusFilter value={status} onChange={setStatus} />
    </div>
    {loadingPages ? <div className="skeleton-list" /> : relevant.length === 0 ? <EmptyState icon={kind === 'concept' ? <BookOpen /> : <GitBranch />} title={t('library.none')} body={t('library.noneBody')} /> : kind === 'concept' ? <ConceptList pages={relevant} /> : <FeatureList pages={relevant} children={childGroups} />}
  </div>
}

function StatusFilter({ value, onChange }: { value: StatusFilterValue; onChange: (value: StatusFilterValue) => void }) {
  const { t } = useI18n()
  const statusOptions = [
    { value: 'all', label: t('library.all') },
    { value: 'accepted', label: t('library.published') },
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

function ConceptList({ pages }: { pages: Page[] }) {
  const { t } = useI18n()
  const nouns = pages.filter((page) => page.conceptKind === 'noun')
  const verbs = pages.filter((page) => page.conceptKind === 'verb')
  return <div className="concept-columns"><ConceptGroup title={t('library.nouns')} pages={nouns} /><ConceptGroup title={t('library.verbs')} pages={verbs} /></div>
}

function ConceptGroup({ title, pages }: { title: string; pages: Page[] }) {
  const { locale } = useI18n()
  return <section className="panel library-group"><div className="library-group-title"><h2>{title}</h2><span>{pages.length}</span></div><div className="compact-page-list">{pages.map((page) => <Link to={`/page/${page.id}`} key={page.id}><span className="compact-page-copy"><strong>{page.title}</strong><small>{kindLabel(page, locale)}</small></span><StatusBadge page={page} /></Link>)}</div></section>
}

function FeatureList({ pages, children }: { pages: Page[]; children: Map<string, Page[]> }) {
  const { t } = useI18n()
  return <div className="feature-list">{pages.map((page) => {
    const count = children.get(page.id)?.length ?? 0
    return <section className="panel feature-card" key={page.id}><Link to={`/page/${page.id}`} className="feature-card-heading"><LibraryPageIcon page={page} /><span><strong>{page.title}</strong><small>{count === 1 ? t('library.scenarioCount.one') : t('library.scenarioCount.other', { count })}</small></span><StatusBadge page={page} /></Link><div className="feature-children">{(children.get(page.id) ?? []).map((child) => <Link to={`/page/${child.id}`} key={child.id}><span className="tree-line" /><PageIcon page={child} size={15} draft={pageIconState(child) === 'draft'} /><span>{child.title}</span>{child.hasDraft && <i className="draft-dot" />}</Link>)}</div></section>
  })}</div>
}

function LibraryPageIcon({ page }: { page: Page }) {
  const state = pageIconState(page)
  return <span className={`page-icon-box ${page.kind} ${state}`}><PageIcon page={page} draft={state === 'draft'} /></span>
}

function pageIconState(page: Page): 'approved' | 'rejected' | 'draft' {
  if (page.unresolvedRejections > 0) return 'rejected'
  if (page.hasDraft) return 'draft'
  return page.accepted ? 'approved' : 'draft'
}
