import { useEffect, useMemo, useState } from 'react'
import { ArrowLeft, BookOpen, Clock3, GitBranch, MessageSquare, Pencil } from 'lucide-react'
import { api } from '../api/client'
import type { PageDetail, Revision } from '../api/types'
import { Link, useRouter } from '../router'
import { useWorkspace } from '../workspace'
import { PageIcon } from '../components/PageIcon'
import { EmptyState, formatDate, kindLabel, Markdown, Spinner, StatusBadge } from '../components/ui'
import { BDDSteps } from '../components/page/BDDStepsEditor'
import { PageEditor } from '../components/page/PageEditor'
import { ReviewPanel } from '../components/page/ReviewPanel'
import { RevisionHistory } from '../components/page/RevisionHistory'

export function PagePage({ pageId }: { pageId: string }) {
  const { search } = useRouter()
  const requestedRevisionId = search.get('revision')
  const [detail, setDetail] = useState<PageDetail | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(false)
  const [history, setHistory] = useState(false)
  const [version, setVersion] = useState<'accepted' | 'draft'>('accepted')
  const { reloadPages } = useWorkspace()
  const load = async () => { try { const next = await api.page(pageId); setDetail(next); if (requestedRevisionId && next.draftRevision?.id === requestedRevisionId) setVersion('draft'); else if (requestedRevisionId && next.acceptedRevision?.id === requestedRevisionId) setVersion('accepted'); else if (!next.acceptedRevision && next.draftRevision) setVersion('draft'); else if (!next.draftRevision) setVersion('accepted'); setError('') } catch (reason) { setError(reason instanceof Error ? reason.message : 'Stránku sa nepodarilo načítať.') } }
  useEffect(() => { setDetail(null); setEditing(false); setVersion('accepted'); void load() }, [pageId, requestedRevisionId])
  const revision = useMemo<Revision | undefined>(() => version === 'draft' ? detail?.draftRevision : detail?.acceptedRevision, [detail, version])
  const editableRevision = detail?.draftRevision ?? detail?.acceptedRevision
  const changed = async () => { await load(); await reloadPages(); setEditing(false) }
  if (!detail && !error) return <Spinner label="Načítavam stránku…" />
  if (error || !detail) return <div className="page-container"><EmptyState title="Stránku sa nepodarilo načítať" body={error} /></div>
  return <div className="document-layout">
    <article className="document-page">
      <div className="document-breadcrumb"><Link to={detail.page.kind === 'primitive' ? '/primitives' : '/scenarios'}><ArrowLeft size={14} />{detail.page.kind === 'primitive' ? 'Pojmy' : 'Scenáre'}</Link><span>/</span><span>{kindLabel(detail.page)}</span></div>
      {detail.acceptedRevision && detail.draftRevision && <div className="version-switch" role="tablist"><button role="tab" aria-selected={version === 'accepted'} className={version === 'accepted' ? 'active' : ''} onClick={() => setVersion('accepted')}>Publikované</button><button role="tab" aria-selected={version === 'draft'} className={version === 'draft' ? 'active' : ''} onClick={() => setVersion('draft')}>Koncept #{detail.draftRevision?.number}</button></div>}
      {revision ? <>
        <header className="document-header"><div className="document-title-row"><div className="document-title-main"><div className={`document-icon ${detail.page.kind}`}><PageIcon page={detail.page} size={25} /></div><h1>{revision.title}</h1></div><StatusBadge status={revision.status} /></div><p className="revision-meta">Revízia #{revision.number} · {revision.createdBy.displayName} · {formatDate(revision.createdAt)}</p>{editableRevision && !editing && <button className="secondary-button document-edit-button" onClick={() => setEditing(true)}><Pencil size={15} />Upraviť</button>}</header>
        {editing && editableRevision ? <PageEditor page={detail.page} revision={editableRevision} onCancel={() => setEditing(false)} onSaved={changed} /> : <>
          <section className="document-body"><Markdown>{revision.bodyMd || '_Táto stránka zatiaľ nemá opis._'}</Markdown>{detail.page.kind === 'subscenario' && <BDDSteps steps={revision.steps} />}</section>
          {revision.aliases.length > 0 && <section className="document-section"><h2>Alias názvy</h2><div className="tag-list">{revision.aliases.map((alias) => <span key={alias}>{alias}</span>)}</div></section>}
          {revision.references.length > 0 && <section className="document-section"><h2>Súvisiace stránky</h2><div className="reference-list">{revision.references.map((reference) => <Link to={`/page/${reference.targetPageId}`} key={`${reference.targetPageId}-${reference.relation}`}><GitBranch size={15} /><span><strong>{reference.targetTitle}</strong><small>{relationLabel(reference.relation)}</small></span></Link>)}</div></section>}
          {detail.children.length > 0 && <section className="document-section"><h2>Podscenáre</h2><div className="child-list">{detail.children.map((child, index) => <Link to={`/page/${child.id}`} key={child.id}><span>{index + 1}</span><PageIcon page={child} /><strong>{child.title}</strong><StatusBadge page={child} /></Link>)}</div></section>}
        </>}
      </> : <EmptyState icon={<BookOpen />} title="Verzia nie je dostupná" body="Vyberte dostupnú publikovanú verziu alebo koncept." />}
    </article>
    <aside className="document-tools">
      <div className="document-toolbar"><button className="secondary-button" onClick={() => setHistory(true)}><Clock3 size={15} />História</button></div>
      {revision && <div className="panel review-panel"><div className="panel-heading compact"><div><h2><MessageSquare size={17} />Kontrola</h2><p>Hlasovanie, komentáre a publikovanie.</p></div></div><ReviewPanel detail={detail} revision={revision} onChanged={changed} /></div>}
    </aside>
    {history && <RevisionHistory detail={detail} onClose={() => setHistory(false)} />}
  </div>
}

function relationLabel(relation: string): string {
  const labels: Record<string, string> = { uses: 'používa', requires: 'vyžaduje', produces: 'vytvára', relates_to: 'súvisí s' }
  return labels[relation] ?? relation
}
