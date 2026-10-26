import { useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import type { PageDetail, RepositorySource, Revision } from '../api/types'
import { Link, useRouter } from '../router'
import { useWorkspace } from '../workspace'
import { Markdown, Spinner } from './ui'

export function RepositorySyncBanner() {
 const { syncStatus: status, syncError } = useWorkspace()
 if (syncError) return <div className="repository-sync error" role="status">{syncError} Content changes are disabled until authority is known.</div>
 if (status?.mode !== 'git_managed') return null
 return <section className="repository-sync" aria-label="Repository synchronization">
  <strong>Git-managed · Read-only</strong><span>Canonical specifications: <a href={status.repository}>{status.repository}</a></span>
  {status.commit ? <span>Repository baseline <a href={`${status.repository}/commit/${status.commit}`}>{status.commit.slice(0, 12)}</a> · {status.scenarioCount} verified scenarios</span> : <span>Awaiting an eligible successful main snapshot.</span>}
  <span>{status.stale ? 'Sync needs attention' : 'Poller current'} · Last checked: {status.lastCheckedAt ?? 'not checked yet'} · Last import: {status.lastImportedAt ?? 'none'}</span>
  {status.lastError && <span role="alert">Last sync error: {status.lastError}. The last good baseline remains available.</span>}
  <small>Repository CI verification is separate from human approval and production deployment.</small>
 </section>
}
export function RepositoryLibrary({ kind }: { kind: 'concept' | 'feature' }) {
 const { pages, loadingPages } = useWorkspace()
 const [query,setQuery] = useState('')
 const visible = pages.filter(p => p.kind === kind && `${p.title} ${p.repositorySource?.externalId}`.toLowerCase().includes(query.toLowerCase()))
 return <div className="page-container repository-library"><header className="page-heading"><div><h1>{kind === 'concept' ? 'Concepts' : 'Features'}</h1><p>Exact English business specifications from the canonical repository.</p></div></header>
 <label className="search-field">Search specifications<input value={query} onChange={event=>setQuery(event.target.value)} /></label>
 {loadingPages ? <Spinner /> : visible.length === 0 ? <p>No matching repository specifications.</p> : visible.map(page=><section className="panel repository-card" key={page.id}><Link to={`/page/${page.id}`}><strong>{page.title}</strong></Link><span>{page.repositorySource?.externalId} · Repository baseline</span>
 {pages.filter(child=>child.parentId===page.id).map(child=><Link className="repository-scenario" key={child.id} to={`/page/${child.id}`}>{child.title} · Passed at {child.repositorySource?.verifiedCommit.slice(0,12)}</Link>)}</section>)}
 </div>
}
export function RepositoryProvenance({ source }: { source: RepositorySource }) {
 return <section className="repository-provenance"><strong>Repository baseline · {source.externalId}</strong>
 <span>Source: <a href={`${source.repository}/blob/${source.commit}/${source.path}`}>{source.path}</a> at <a href={`${source.repository}/commit/${source.commit}`}>{source.commit}</a></span>
 <span>Verified: {source.result} at {source.verifiedCommit} · <a href={`${source.repository}/actions/runs/${source.runId}/attempts/${source.runAttempt}`}>CI run {source.runId}, attempt {source.runAttempt}</a></span>
 <small>Source SHA-256: {source.contentHash}. This is CI evidence, not human approval or a production-deployment receipt.</small>
 </section>
}
export function RepositoryPage({ pageId }: { pageId: string }) {
 const { search } = useRouter()
 const { syncStatus } = useWorkspace()
 const loadSequence = useRef(0)
 const requested = search.get('revision')
 const [detail,setDetail] = useState<PageDetail | null>(null)
 const [selected,setSelected] = useState<Revision | null>(null)
 const [error,setError] = useState('')
 const loadRevision = async (id:string) => {
  const sequence = ++loadSequence.current
  try { const version = await api.revision(id); if (sequence !== loadSequence.current) return; if (version.pageId !== pageId) throw new Error('Wrong page'); setSelected(version); setError('') }
  catch { if (sequence === loadSequence.current) setError('This repository version could not be loaded.') }
 }
 useEffect(() => {
  const sequence = ++loadSequence.current
  setDetail(null); setSelected(null); setError('')
  void Promise.all([api.page(pageId), requested ? api.revision(requested) : Promise.resolve(null)]).then(([next, version]) => {
   if (sequence !== loadSequence.current) return
   const chosen = requested ? version : next.repositoryRevision ?? null
   if (chosen && chosen.pageId !== pageId) throw new Error('Wrong page')
   setDetail(next); setSelected(chosen)
  }).catch(() => { if (sequence === loadSequence.current) setError('This repository page or version could not be loaded.') })
  return () => { loadSequence.current++ }
 }, [pageId, requested, syncStatus?.commit, syncStatus?.runId, syncStatus?.runAttempt])

 if(error)return <div className="page-container" role="alert">{error}</div>
 if(!detail)return <Spinner />
 if(!selected)return <div className="page-container">No repository baseline is available for this page.</div>
 return <article className="page-container repository-document"><Link to={detail.page.kind==='concept'?'/concepts':'/features'}>Back to catalogue</Link>
 {detail.page.archived&&<p role="status">Removed from the current catalogue. This version remains available as historical evidence.</p>}
 {detail.page.parentId&&<p><Link to={`/page/${detail.page.parentId}`}>Parent feature</Link></p>}
 <h1>{selected.title}</h1>{selected.repositorySource&&<RepositoryProvenance source={selected.repositorySource} />}
 <div className="document-body"><Markdown>{selected.bodyMd}</Markdown></div>
 {selected.steps.length>0&&<pre className="repository-gherkin" aria-label="Canonical Gherkin steps">{selected.steps.map(step=>`${step.keyword[0].toUpperCase()}${step.keyword.slice(1)} ${step.text}`).join('\n')}</pre>}
 {selected.references.length>0&&<section><h2>Related concepts</h2>{selected.references.map(reference=><p key={reference.targetPageId}><Link to={`/page/${reference.targetPageId}`}>{reference.targetTitle}</Link></p>)}</section>}
 {detail.children.length>0&&<section><h2>Scenarios</h2>{detail.children.map(child=><p key={child.id}><Link to={`/page/${child.id}`}>{child.title}</Link></p>)}</section>}
 <section><h2>Immutable business versions</h2>{detail.revisions.map(revision=><button className="secondary-button" aria-pressed={selected.id===revision.id} key={revision.id} onClick={()=>void loadRevision(revision.id)}>Version {revision.number} · {revision.title}</button>)}</section>
 {selected.repositorySource&&<details><summary>Exact canonical source</summary><pre className="repository-gherkin">{selected.repositorySource.content}</pre></details>}
 </article>
}
