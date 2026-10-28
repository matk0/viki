import { useEffect, useMemo, useRef, useState } from 'react'
import { ArrowLeft, BookOpen, ChevronDown, Pencil, Plus } from 'lucide-react'
import { api } from '../api/client'
import { localizedDetail, localizedRevision, translationPending } from '../localization'
import type { Page, PageDetail, Revision } from '../api/types'
import { Link, useRouter } from '../router'
import { useWorkspace } from '../workspace'
import { PageIcon } from '../components/PageIcon'
import { EmptyState, formatDate, kindLabel, Markdown, revisionBody, Spinner, StatusBadge, statusLabel } from '../components/ui'
import { BDDSteps } from '../components/page/BDDStepsEditor'
import { PageEditor } from '../components/page/PageEditor'
import { DiscussionPanel, ReviewPanel } from '../components/page/ReviewPanel'
import { RevisionHistory } from '../components/page/RevisionHistory'
import { translate, useI18n, type Locale } from '../i18n'

export function PagePage({ pageId }: { pageId: string }) {
  const { locale, t } = useI18n()
  const { search } = useRouter()
  const requestedRevisionId = search.get('revision')
  const [sourceDetail, setDetail] = useState<PageDetail | null>(null)
  const detail = useMemo(() => sourceDetail ? localizedDetail(sourceDetail, locale) : null, [sourceDetail, locale])
  const [repositoryRevision, setRepositoryRevision] = useState<Revision>()
  const loadSequence = useRef(0)
  const loadedIdentity = useRef<{ pageId: string; revisionId: string | null } | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(false)
  const [history, setHistory] = useState(false)
  const [relatedOpen, setRelatedOpen] = useState(false)
  const [version, setVersion] = useState<'approved' | 'draft'>('approved')
  const { pages, reloadPages, openNewPage, readOnly, syncStatus } = useWorkspace()
  const load = async () => {
    const sequence = ++loadSequence.current
    try {
      const next = await api.page(pageId)
      const selected = next.page.repositoryRevisionId && requestedRevisionId ? await api.revision(requestedRevisionId) : next.repositoryRevision
      if (sequence !== loadSequence.current) return
      if (selected && selected.pageId !== pageId) throw new Error(t('page.loadFailed'))
      setDetail(next)
      setRepositoryRevision(selected)
      if (requestedRevisionId && next.draftRevision?.id === requestedRevisionId) setVersion('draft')
      else if (requestedRevisionId && next.approvedRevision?.id === requestedRevisionId) setVersion('approved')
      else if (!next.approvedRevision && next.draftRevision) setVersion('draft')
      else if (!next.draftRevision) setVersion('approved')
      setError('')
    } catch (reason) {
      if (sequence === loadSequence.current) setError(reason instanceof Error ? reason.message : t('page.loadFailed'))
    }
  }
  useEffect(() => {
    if (loadedIdentity.current?.pageId !== pageId || loadedIdentity.current.revisionId !== requestedRevisionId) {
      setDetail(null); setRepositoryRevision(undefined); setEditing(false); setHistory(false); setRelatedOpen(false); setVersion('approved'); setError('')
    }
    loadedIdentity.current = { pageId, revisionId: requestedRevisionId }
    void load()
    return () => { loadSequence.current++ }
  }, [pageId, requestedRevisionId, syncStatus?.commit, syncStatus?.runId, syncStatus?.runAttempt, syncStatus?.localizationUpdatedAt])
  const revision = useMemo<Revision | undefined>(() => detail?.page.repositoryRevisionId ? repositoryRevision && localizedRevision(repositoryRevision, locale) : version === 'draft' ? detail?.draftRevision : detail?.approvedRevision, [detail, repositoryRevision, version, locale])
  const editableRevision = version === 'draft' ? detail?.draftRevision : detail?.draftRevision ? undefined : detail?.approvedRevision
  const changed = async () => { await load(); await reloadPages(); setEditing(false) }
  const saved = async () => { await changed(); setVersion('draft') }
  if (!detail && !error) return <Spinner label={t('page.loading')} />
  if (error || !detail) return <div className="page-container"><EmptyState title={t('page.loadFailed')} body={error} /></div>
  const parentFeature = detail.page.kind === 'scenario' ? pages.find((page) => page.id === detail.page.parentId && page.kind === 'feature') : undefined
  const canEdit = !readOnly && !detail.page.repositoryRevisionId
  return <div className={`document-layout${canEdit ? '' : ' document-readonly'}`}>
    <article className="document-page">
      <div className="document-breadcrumb"><Link to={detail.page.kind === 'concept' ? '/concepts' : '/features'}><ArrowLeft size={14} />{detail.page.kind === 'concept' ? t('kind.concepts') : t('kind.features')}</Link>{parentFeature && <><span>/</span><Link className="breadcrumb-parent" title={parentFeature.title} to={`/page/${parentFeature.id}`}><span className="breadcrumb-parent-title">{parentFeature.title}</span></Link></>}<span>/</span><span>{kindLabel(detail.page, locale)}</span></div>
      {detail.approvedRevision && detail.draftRevision && <div className="version-switch" role="tablist"><button role="tab" aria-selected={version === 'approved'} className={version === 'approved' ? 'active' : ''} onClick={() => setVersion('approved')}>{t('page.approved')}</button><button role="tab" aria-selected={version === 'draft'} className={version === 'draft' ? 'active' : ''} onClick={() => setVersion('draft')}>{t('page.draftNumber', { number: detail.draftRevision.number })}</button></div>}
      {revision ? <>
        <header className="document-header"><div className="document-header-actions"><button className="secondary-button document-header-button" onClick={() => setHistory(true)}>{t('page.history')}</button>{!editing && <button className="secondary-button document-header-button document-edit-button" disabled={!canEdit || (version === 'approved' && Boolean(detail.draftRevision))} onClick={() => setEditing(true)}>{version === 'draft' ? <Pencil size={15} /> : <Plus size={15} />}{version === 'draft' ? t('page.editDraft') : t('page.newVersion')}</button>}</div><div className="document-title-row"><div className="document-title-main"><h1>{revision.title}</h1></div><div className={`document-icon ${detail.page.kind} ${revision.status}`} role="img" aria-label={`${kindLabel(detail.page, locale)} · ${statusLabel(revision.status, t)}`}><PageIcon page={detail.page} size={25} /></div></div><p className="revision-meta">{detail.page.repositoryRevisionId ? `${t('history.revision', { number: revision.number })} · ${formatDate(revision.createdAt, true, locale)}` : t('page.revisionMeta', { number: revision.number, author: revision.createdBy.displayName, date: formatDate(revision.createdAt, true, locale) })}</p>{detail.page.archived && <p className="revision-meta" role="status">{t('page.archived')}</p>}</header>
        {editing && editableRevision && canEdit ? <PageEditor page={detail.page} revision={editableRevision} onCancel={() => setEditing(false)} onSaved={saved} /> : <>
          {translationPending(revision, locale) && <p className="revision-meta" role="status">{t('page.translationPending')}</p>}
          <section className="document-body" lang={translationPending(revision, locale) ? 'en' : locale}><Markdown inlineLinks={conceptInlineLinks(revision, pages)}>{revisionBody(revision) || t('page.emptyDescription')}</Markdown>{detail.page.kind === 'scenario' && <BDDSteps steps={revision.steps} />}</section>
          {detail.page.kind === 'feature' && <section className="document-section"><div className="document-section-heading"><h2>{t('page.scenarios')}</h2><button className="secondary-button" disabled={!canEdit} onClick={() => openNewPage('scenario', detail.page.id)}><Plus size={14} />{t('page.addScenario')}</button></div>{detail.children.length > 0 ? <div className="child-list">{detail.children.map((child, index) => <Link to={`/page/${child.id}`} key={child.id}><span>{index + 1}</span><PageIcon page={child} /><strong>{child.title}</strong><StatusBadge page={child} /></Link>)}</div> : <p className="muted">{t('page.noScenarios')}</p>}</section>}
          {revision.references.length > 0 && <section className="document-section document-accordion"><h2><button type="button" className="document-accordion-toggle" aria-expanded={relatedOpen} aria-controls={`related-pages-${revision.id}`} onClick={() => setRelatedOpen((open) => !open)}><span>{t('page.related')}</span><ChevronDown size={17} /></button></h2><div id={`related-pages-${revision.id}`} className="reference-list document-accordion-content" hidden={!relatedOpen}>{revision.references.map((reference) => {
            const targetPage = pages.find((page) => page.id === reference.targetPageId)
            return <Link to={`/page/${reference.targetPageId}`} key={`${reference.targetPageId}-${reference.relation}`}>{targetPage ? <PageIcon page={targetPage} size={15} /> : <BookOpen size={15} />}<span><strong>{targetPage?.title ?? reference.targetTitle}</strong><small>{relationLabel(reference.relation, locale)}</small></span></Link>
          })}</div></section>}
        </>}
      </> : <EmptyState icon={<BookOpen />} title={t('page.versionUnavailable')} body={t('page.versionUnavailableBody')} />}
    </article>
    {canEdit && <aside className="document-tools">
      {revision && <ReviewPanel detail={detail} revision={revision} onChanged={changed} onCreateVersion={version === 'approved' && !detail.draftRevision ? () => setEditing(true) : undefined} />}
      {revision && <div className="panel discussion-panel"><DiscussionPanel detail={detail} revision={revision} onChanged={changed} /></div>}
    </aside>}
    {history && <RevisionHistory detail={sourceDetail!} onClose={() => setHistory(false)} />}
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
