import { useMemo, useState } from 'react'
import { BookOpen, Filter, GitBranch, Plus, Search } from 'lucide-react'
import type { Page } from '../api/types'
import { Link } from '../router'
import { useWorkspace } from '../workspace'
import { PageIcon } from '../components/PageIcon'
import { VikiSelect } from '../components/VikiSelect'
import { EmptyState, kindLabel, StatusBadge } from '../components/ui'

type StatusFilterValue = 'all' | 'accepted' | 'draft'

const statusOptions: { value: StatusFilterValue; label: string }[] = [
  { value: 'all', label: 'Všetky' },
  { value: 'accepted', label: 'Publikované' },
  { value: 'draft', label: 'Koncept' },
]

export function LibraryPage({ kind }: { kind: 'primitive' | 'scenario' }) {
  const { pages, loadingPages, openNewPage } = useWorkspace()
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState<StatusFilterValue>('all')
  const relevant = useMemo(() => {
    const normalizedQuery = normalizeSearchText(query)
    return pages.filter((page) => {
      const kindMatches = kind === 'primitive' ? page.kind === 'primitive' : page.kind === 'scenario'
      const queryMatches = normalizeSearchText(page.title).includes(normalizedQuery)
      const statusMatches = status === 'all' || (status === 'draft' ? page.hasDraft : page.accepted)
      return kindMatches && queryMatches && statusMatches
    })
  }, [pages, kind, query, status])
  const children = useMemo(() => new Map(pages.filter((page) => page.kind === 'subscenario').map((page) => [page.id, page])), [pages])
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
      <div><h1>{kind === 'primitive' ? 'Pojmy' : 'Scenáre'}</h1><p>{kind === 'primitive' ? 'Kanonické podstatné mená a slovesá používané vo firme.' : 'Schopnosti systému a ich konkrétne BDD správania.'}</p></div>
      <button className="primary-button" onClick={() => openNewPage(kind)}><Plus size={16} />Pridať {kind === 'primitive' ? 'pojem' : 'scenár'}</button>
    </header>
    <div className="library-toolbar">
      <label className="search-field"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={`Hľadať v ${kind === 'primitive' ? 'pojmoch' : 'scenároch'}…`} /></label>
      <StatusFilter value={status} onChange={setStatus} />
    </div>
    {loadingPages ? <div className="skeleton-list" /> : relevant.length === 0 ? <EmptyState icon={kind === 'primitive' ? <BookOpen /> : <GitBranch />} title="Nič sa nenašlo" body="Skúste zmeniť vyhľadávanie alebo filter." /> : kind === 'primitive' ? <PrimitiveList pages={relevant} /> : <ScenarioList pages={relevant} children={childGroups} />}
  </div>
}

function StatusFilter({ value, onChange }: { value: StatusFilterValue; onChange: (value: StatusFilterValue) => void }) {
  const selected = statusOptions.find((option) => option.value === value) ?? statusOptions[0]
  return <VikiSelect
    className="filter-select"
    ariaLabel={`Filtrovať podľa stavu: ${selected.label}`}
    listboxLabel="Filtrovať podľa stavu"
    value={value}
    options={statusOptions}
    leadingIcon={<Filter size={15} />}
    onChange={(next) => onChange(next as StatusFilterValue)}
  />
}

function normalizeSearchText(value: string): string {
  return value.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLocaleLowerCase('sk')
}

function PrimitiveList({ pages }: { pages: Page[] }) {
  const nouns = pages.filter((page) => page.primitiveKind === 'noun')
  const verbs = pages.filter((page) => page.primitiveKind === 'verb')
  return <div className="primitive-columns"><PrimitiveGroup title="Podstatné mená" pages={nouns} /><PrimitiveGroup title="Slovesá" pages={verbs} /></div>
}

function PrimitiveGroup({ title, pages }: { title: string; pages: Page[] }) {
  return <section className="panel library-group"><div className="library-group-title"><h2>{title}</h2><span>{pages.length}</span></div><div className="compact-page-list">{pages.map((page) => <Link to={`/page/${page.id}`} key={page.id}><span className="compact-page-copy"><strong>{page.title}</strong><small>{kindLabel(page)}</small></span><StatusBadge page={page} /></Link>)}</div></section>
}

function ScenarioList({ pages, children }: { pages: Page[]; children: Map<string, Page[]> }) {
  return <div className="scenario-list">{pages.map((page) => <section className="panel scenario-card" key={page.id}><Link to={`/page/${page.id}`} className="scenario-card-heading"><LibraryPageIcon page={page} /><span><strong>{page.title}</strong><small>{children.get(page.id)?.length ?? 0} podscenáre</small></span><StatusBadge page={page} /></Link><div className="scenario-children">{(children.get(page.id) ?? []).map((child) => <Link to={`/page/${child.id}`} key={child.id}><span className="tree-line" /><PageIcon page={child} size={15} draft={pageIconState(child) === 'draft'} /><span>{child.title}</span>{child.hasDraft && <i className="draft-dot" />}</Link>)}</div></section>)}</div>
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
