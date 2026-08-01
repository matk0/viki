import { useEffect, useState, type FormEvent } from 'react'
import { Search } from 'lucide-react'
import { api } from '../api/client'
import type { PageKind, SearchResult } from '../api/types'
import { Link, useRouter } from '../router'
import { PageIcon } from '../components/PageIcon'
import { VikiSelect } from '../components/VikiSelect'
import { EmptyState, kindLabel, Spinner, StatusBadge } from '../components/ui'
import { useI18n } from '../i18n'

export function SearchPage() {
  const { locale, t } = useI18n()
  const router = useRouter()
  const [query, setQuery] = useState(router.search.get('q') ?? '')
  const [kind, setKind] = useState<PageKind | ''>((router.search.get('kind') as PageKind) ?? '')
  const [includeDrafts, setIncludeDrafts] = useState(router.search.get('includeDrafts') === 'true')
  const [results, setResults] = useState<SearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [searched, setSearched] = useState(false)
  const run = async (nextQuery = query) => {
    setLoading(true); setSearched(true)
    try { setResults((await api.search(nextQuery, kind || undefined, includeDrafts)).results) }
    finally { setLoading(false) }
  }
  useEffect(() => { if (query || includeDrafts) void run(query) }, []) // Initial URL state only.
  const submit = (event: FormEvent) => {
    event.preventDefault()
    const params = new URLSearchParams({ q: query, includeDrafts: String(includeDrafts) })
    if (kind) params.set('kind', kind)
    router.navigate(`/search?${params}`, true)
    void run()
  }
  return <div className="page-container search-page">
    <header className="page-heading"><div><h1>{t('search.title')}</h1><p>{t('search.description')}</p></div></header>
    <form className="search-form" onSubmit={submit}>
      <label className="large-search"><Search size={20} /><input autoFocus value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t('search.placeholder')} /></label>
      <VikiSelect className="search-kind-select" ariaLabel={t('search.pageType')} listboxLabel={t('search.pageTypes')} value={kind} onChange={(value) => setKind(value as PageKind | '')} options={[{ value: '', label: t('search.allTypes') }, { value: 'concept', label: t('kind.concepts') }, { value: 'feature', label: t('kind.features') }, { value: 'scenario', label: t('kind.scenarios') }]} />
      <label className="checkbox-label"><input type="checkbox" checked={includeDrafts} onChange={(event) => setIncludeDrafts(event.target.checked)} />{t('search.includeDrafts')}</label>
      <button className="primary-button">{t('search.submit')}</button>
    </form>
    {loading ? <Spinner label={t('search.searching')} /> : searched && results.length === 0 ? <EmptyState icon={<Search />} title={t('search.noResults')} body={t('search.noResultsBody')} /> : <div className="search-results">{results.map((result) => <Link to={`/page/${result.page.id}`} className="search-result" key={`${result.revisionId}-${result.page.id}`}><span className={`page-icon-box ${result.page.kind}`}><PageIcon page={result.page} /></span><span className="search-result-copy"><span><strong>{result.page.title}</strong><small>{kindLabel(result.page, locale)}</small>{result.draft && <StatusBadge status="draft" />}</span><p>{result.excerpt || t('search.noPreview')}</p></span></Link>)}</div>}
  </div>
}
