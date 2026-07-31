import { useEffect, useState, type FormEvent } from 'react'
import { Search } from 'lucide-react'
import { api } from '../api/client'
import type { PageKind, SearchResult } from '../api/types'
import { Link, useRouter } from '../router'
import { PageIcon } from '../components/PageIcon'
import { VikiSelect } from '../components/VikiSelect'
import { EmptyState, kindLabel, Spinner, StatusBadge } from '../components/ui'

export function SearchPage() {
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
    <header className="page-heading"><div><h1>Vyhľadávanie</h1><p>Nájdite publikované definície, správania aj rozpracované koncepty.</p></div></header>
    <form className="search-form" onSubmit={submit}>
      <label className="large-search"><Search size={20} /><input autoFocus value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Čo hľadáte?" /></label>
      <VikiSelect className="search-kind-select" ariaLabel="Typ stránky" listboxLabel="Typy stránok" value={kind} onChange={(value) => setKind(value as PageKind | '')} options={[{ value: '', label: 'Všetky typy' }, { value: 'primitive', label: 'Pojmy' }, { value: 'scenario', label: 'Scenáre' }, { value: 'subscenario', label: 'Podscenáre' }]} />
      <label className="checkbox-label"><input type="checkbox" checked={includeDrafts} onChange={(event) => setIncludeDrafts(event.target.checked)} />Zahrnúť koncepty</label>
      <button className="primary-button">Hľadať</button>
    </form>
    {loading ? <Spinner label="Hľadám v pracovnom priestore…" /> : searched && results.length === 0 ? <EmptyState icon={<Search />} title="Žiadne výsledky" body="Skúste všeobecnejší výraz alebo zahrňte koncepty." /> : <div className="search-results">{results.map((result) => <Link to={`/page/${result.page.id}`} className="search-result" key={`${result.revisionId}-${result.page.id}`}><span className={`page-icon-box ${result.page.kind}`}><PageIcon page={result.page} /></span><span className="search-result-copy"><span><strong>{result.page.title}</strong><small>{kindLabel(result.page)}</small>{result.draft && <StatusBadge status="draft" />}</span><p>{result.excerpt || 'Bez textového náhľadu.'}</p></span></Link>)}</div>}
  </div>
}
