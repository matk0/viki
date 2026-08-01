import { useEffect, useMemo, useState } from 'react'
import { ArrowLeft, BookOpen, Clock3, MessageSquare, Pencil } from 'lucide-react'
import { api } from '../api/client'
import type { Page, PageDetail, Revision } from '../api/types'
import { Link, useRouter } from '../router'
import { useWorkspace } from '../workspace'
import { PageIcon } from '../components/PageIcon'
import { EmptyState, formatDate, kindLabel, Markdown, Spinner, StatusBadge } from '../components/ui'
import { BDDSteps } from '../components/page/BDDStepsEditor'
import { PageEditor } from '../components/page/PageEditor'
import { ReviewPanel } from '../components/page/ReviewPanel'
import { RevisionHistory } from '../components/page/RevisionHistory'
import { translate, useI18n, type Locale } from '../i18n'

export function PagePage({ pageId }: { pageId: string }) {
  const { locale, t } = useI18n()
  const { search } = useRouter()
  const requestedRevisionId = search.get('revision')
  const [detail, setDetail] = useState<PageDetail | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(false)
  const [history, setHistory] = useState(false)
  const [version, setVersion] = useState<'accepted' | 'draft'>('accepted')
  const { pages, reloadPages } = useWorkspace()
  const load = async () => { try { const next = await api.page(pageId); setDetail(next); if (requestedRevisionId && next.draftRevision?.id === requestedRevisionId) setVersion('draft'); else if (requestedRevisionId && next.acceptedRevision?.id === requestedRevisionId) setVersion('accepted'); else if (!next.acceptedRevision && next.draftRevision) setVersion('draft'); else if (!next.draftRevision) setVersion('accepted'); setError('') } catch (reason) { setError(reason instanceof Error ? reason.message : t('page.loadFailed')) } }
  useEffect(() => { setDetail(null); setEditing(false); setVersion('accepted'); void load() }, [pageId, requestedRevisionId])
  const revision = useMemo<Revision | undefined>(() => version === 'draft' ? detail?.draftRevision : detail?.acceptedRevision, [detail, version])
  const editableRevision = detail?.draftRevision ?? detail?.acceptedRevision
  const changed = async () => { await load(); await reloadPages(); setEditing(false) }
  if (!detail && !error) return <Spinner label={t('page.loading')} />
  if (error || !detail) return <div className="page-container"><EmptyState title={t('page.loadFailed')} body={error} /></div>
  return <div className="document-layout">
    <article className="document-page">
      <div className="document-breadcrumb"><Link to={detail.page.kind === 'concept' ? '/concepts' : '/features'}><ArrowLeft size={14} />{detail.page.kind === 'concept' ? t('kind.concepts') : t('kind.features')}</Link><span>/</span><span>{kindLabel(detail.page, locale)}</span></div>
      {detail.acceptedRevision && detail.draftRevision && <div className="version-switch" role="tablist"><button role="tab" aria-selected={version === 'accepted'} className={version === 'accepted' ? 'active' : ''} onClick={() => setVersion('accepted')}>{t('page.published')}</button><button role="tab" aria-selected={version === 'draft'} className={version === 'draft' ? 'active' : ''} onClick={() => setVersion('draft')}>{t('page.draftNumber', { number: detail.draftRevision.number })}</button></div>}
      {revision ? <>
        <header className="document-header"><div className="document-title-row"><div className="document-title-main"><div className={`document-icon ${detail.page.kind}`}><PageIcon page={detail.page} size={25} /></div><h1>{revision.title}</h1></div><StatusBadge status={revision.status} /></div><p className="revision-meta">{t('page.revisionMeta', { number: revision.number, author: revision.createdBy.displayName, date: formatDate(revision.createdAt, true, locale) })}</p>{editableRevision && !editing && <button className="secondary-button document-edit-button" onClick={() => setEditing(true)}><Pencil size={15} />{t('common.edit')}</button>}</header>
        {editing && editableRevision ? <PageEditor page={detail.page} revision={editableRevision} onCancel={() => setEditing(false)} onSaved={changed} /> : <>
          <section className="document-body"><Markdown inlineLinks={conceptInlineLinks(revision, pages)}>{revision.bodyMd || t('page.emptyDescription')}</Markdown>{detail.page.kind === 'scenario' && <BDDSteps steps={revision.steps} />}</section>
          {revision.aliases.length > 0 && <section className="document-section"><h2>{t('page.aliases')}</h2><div className="tag-list">{revision.aliases.map((alias) => <span key={alias}>{alias}</span>)}</div></section>}
          {revision.references.length > 0 && <section className="document-section"><h2>{t('page.related')}</h2><div className="reference-list">{revision.references.map((reference) => {
            const targetPage = pages.find((page) => page.id === reference.targetPageId)
            return <Link to={`/page/${reference.targetPageId}`} key={`${reference.targetPageId}-${reference.relation}`}>{targetPage ? <PageIcon page={targetPage} size={15} /> : <BookOpen size={15} />}<span><strong>{targetPage?.title ?? reference.targetTitle}</strong><small>{relationLabel(reference.relation, locale)}</small></span></Link>
          })}</div></section>}
          {detail.children.length > 0 && <section className="document-section"><h2>{t('page.scenarios')}</h2><div className="child-list">{detail.children.map((child, index) => <Link to={`/page/${child.id}`} key={child.id}><span>{index + 1}</span><PageIcon page={child} /><strong>{child.title}</strong><StatusBadge page={child} /></Link>)}</div></section>}
        </>}
      </> : <EmptyState icon={<BookOpen />} title={t('page.versionUnavailable')} body={t('page.versionUnavailableBody')} />}
    </article>
    <aside className="document-tools">
      <div className="document-toolbar"><button className="secondary-button" onClick={() => setHistory(true)}><Clock3 size={15} />{t('page.history')}</button></div>
      {revision && <div className="panel review-panel"><div className="panel-heading compact"><div><h2><MessageSquare size={17} />{t('page.review')}</h2><p>{t('page.reviewBody')}</p></div></div><ReviewPanel detail={detail} revision={revision} onChanged={changed} /></div>}
    </aside>
    {history && <RevisionHistory detail={detail} onClose={() => setHistory(false)} />}
  </div>
}

function conceptInlineLinks(revision: Revision, pages: Page[]) {
  return revision.references.flatMap((reference) => {
    const targetPage = pages.find((page) => page.id === reference.targetPageId)
    if (targetPage?.kind !== 'concept') return []
    return [{ href: `/page/${targetPage.id}`, label: targetPage.title, className: 'concept-reference' }]
  })
}

function relationLabel(relation: string, locale: Locale): string {
  const labels = { uses: 'relation.uses', requires: 'relation.requires', produces: 'relation.produces', relates_to: 'relation.relates_to' } as const
  const key = labels[relation as keyof typeof labels]
  return key ? translate(locale, key) : relation
}
