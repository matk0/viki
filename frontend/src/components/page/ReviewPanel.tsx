import { BadgeCheck, Ban, Check, ChevronDown, ChevronUp, CircleCheck, ClipboardCheck, Clock3, CornerDownRight, Hammer, MessageSquare, Pencil, TriangleAlert, type LucideIcon } from 'lucide-react'
import { useState, type FormEvent, type KeyboardEvent, type ReactNode } from 'react'
import { api } from '../../api/client'
import type { Comment, DevelopmentStatus, Objection, PageDetail, ParentFeatureReviewBlocker, Revision } from '../../api/types'
import { Link } from '../../router'
import { formatDate } from '../ui'
import { useI18n } from '../../i18n'
import { Modal } from '../Modal'

export function ReviewPanel({ detail, revision, onChanged, onCreateVersion }: { detail: PageDetail; revision: Revision; onChanged: () => Promise<void>; onCreateVersion?: () => void }) {
  const { t } = useI18n()
  const [rejectReason, setRejectReason] = useState('')
  const [showReject, setShowReject] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const review = detail.reviewStates.find((entry) => entry.revisionId === revision.id)
  const development = detail.development?.revisionId === revision.id ? detail.development : undefined
  const developmentProgress = detail.page.kind === 'feature' ? detail.developmentProgress : undefined
  const blockers = review?.blockers ?? []
  const parentFeatureBlockers = blockers.filter((blocker): blocker is ParentFeatureReviewBlocker => blocker.type === 'parent_feature')
  const run = async (action: () => Promise<unknown>) => {
    setBusy(true); setError('')
    try { await action(); await onChanged() } catch (reason) { setError(reason instanceof Error ? reason.message : t('review.failed')) }
    finally { setBusy(false) }
  }
  const reject = () => {
    void run(async () => { await api.raiseObjection(revision.id, rejectReason.trim()); setRejectReason(''); setShowReject(false) })
  }
  const closeReject = () => { setShowReject(false); setRejectReason(''); setError('') }
  if (!review) throw new Error(`Missing review state for draft revision ${revision.id}`)
  return <>
    <div className="panel review-panel">
      <div className="panel-heading compact"><div><h2><ClipboardCheck size={17} />{t('page.review')}</h2></div></div>
      <div className={`review-panel-body${review.state === 'approved' ? '' : ' has-actions'}`}>
        <div className="review-panel-content">
          <ReviewStatusList state={review.state} development={development} developmentProgress={developmentProgress} onCreateVersion={onCreateVersion} blockedContent={review.state !== 'approved' && (detail.objections.length > 0 || parentFeatureBlockers.length > 0) ? <section className={`review-decision ${review.state}`}>
            {detail.objections.length > 0 && <div className="review-objections" aria-label={t('review.objections')}><h3>{t('review.objections')}</h3>{detail.objections.map((objection) => <ObjectionCard key={objection.id} objection={objection} busy={busy} onResolve={() => void run(() => api.resolveObjection(objection.id))} />)}</div>}
            {parentFeatureBlockers.length > 0 && <div className="review-blockers" aria-label={t('review.blockers')}><h3>{t('review.blockers')}</h3>{parentFeatureBlockers.map((blocker) => <ParentFeatureBlocker key={blocker.id} blocker={blocker} />)}</div>}
          </section> : undefined} />
          {error && <div className="form-error" role="alert">{error}</div>}
        </div>
        {review.state !== 'approved' && <div className="review-status-actions" role="group" aria-label={t('review.draftApproval')}>
          <button className="review-status-action object" aria-label={t('review.raiseObjection')} disabled={busy || review.state !== 'ready'} onClick={() => setShowReject(true)}><ChevronUp /></button>
          <button className="review-status-action approve" aria-label={t('review.approveDraft')} disabled={busy || review.state !== 'ready'} onClick={() => void run(() => api.approve(revision.id))}><ChevronDown /></button>
        </div>}
      </div>
    </div>
    {showReject && <Modal className="objection-dialog-backdrop" onClose={closeReject}>
      <form className="modal-card objection-dialog" role="dialog" aria-modal="true" aria-labelledby="objection-dialog-title" onSubmit={(event) => { event.preventDefault(); reject() }}>
        <div className="modal-heading"><div><h2 id="objection-dialog-title">{t('review.raiseObjection')}</h2></div></div>
        <label>{t('review.reason')}<textarea autoFocus rows={5} value={rejectReason} onChange={(event) => setRejectReason(event.target.value)} onKeyDown={submitTextareaOnEnter} placeholder={t('review.reasonPlaceholder')} /></label>
        <div className="modal-actions"><button type="button" className="secondary-button" onClick={closeReject}>{t('common.cancel')}</button><button className="primary-button" disabled={busy || !rejectReason.trim()}>{t('review.submitObjection')}</button></div>
      </form>
    </Modal>}
  </>
}

function ReviewStatusList({ state, development, developmentProgress, blockedContent, onCreateVersion }: { state: 'approved' | 'ready' | 'blocked'; development?: NonNullable<PageDetail['development']>; developmentProgress?: NonNullable<PageDetail['developmentProgress']>; blockedContent?: ReactNode; onCreateVersion?: () => void }) {
  const { t } = useI18n()
  const statuses = [
    { state: 'blocked', label: t('review.statusBlocked'), icon: Ban },
    { state: 'ready', label: t('review.statusDraft'), icon: Pencil },
    { state: 'approved', label: t('page.approved'), icon: BadgeCheck },
  ] as const
  return <div className="review-status-list" role="list" aria-label={t('review.statuses')}>
    {statuses.map((status) => {
      const disabled = state === 'approved' && status.state !== 'approved'
      const className = `review-status-line ${status.state}${state === status.state ? ' current' : ''}${disabled ? ' disabled' : ''}`
      if (status.state === 'blocked' && blockedContent) return <details className="review-status-accordion" role="listitem" key={status.state}>
        <summary className={className} aria-current={state === status.state ? 'step' : undefined} aria-disabled={disabled || undefined}><StatusLineLabel icon={status.icon} label={status.label} /><ChevronDown size={17} /></summary>
        <div className="review-status-expanded">{blockedContent}</div>
      </details>
      return <div className={className} role="listitem" aria-current={state === status.state ? 'step' : undefined} aria-disabled={disabled || undefined} key={status.state}><StatusLineLabel icon={status.icon} label={status.label} />{status.state === 'approved' && state === 'approved' && onCreateVersion && <button type="button" className="review-new-version" onClick={onCreateVersion}>{t('page.newVersion')}</button>}</div>
    })}
    {development && <div className={`review-status-line development development-${development.status} current`} role="listitem" aria-current="step"><StatusLineLabel icon={developmentStatusIcons[development.status]} label={developmentLabel(development.status, t)} /></div>}
    {developmentProgress && <FeatureDevelopmentStatus progress={developmentProgress} />}
  </div>
}

function FeatureDevelopmentStatus({ progress }: { progress: NonNullable<PageDetail['developmentProgress']> }) {
  const { t } = useI18n()
  const complete = progress.developed === progress.total
  return <div className={`review-status-line development development-${complete ? 'developed' : 'running'} current`} role="listitem" aria-current="step">
    <StatusLineLabel icon={complete ? CircleCheck : Hammer} label={complete ? t('development.developed') : t('development.running')} />
    <span className="review-status-progress">{progress.developed}/{progress.total}</span>
  </div>
}

const developmentStatusIcons: Record<DevelopmentStatus, LucideIcon> = {
  queued: Clock3,
  running: Hammer,
  developed: CircleCheck,
  blocked: TriangleAlert,
}

function StatusLineLabel({ icon: Icon, label }: { icon: LucideIcon; label: string }) {
  return <span className="review-status-label"><Icon size={17} aria-hidden="true" /><span>{label}</span></span>
}

function developmentLabel(status: DevelopmentStatus, t: ReturnType<typeof useI18n>['t']) {
  switch (status) {
    case 'queued': return t('development.queued')
    case 'running': return t('development.running')
    case 'developed': return t('development.developed')
    case 'blocked': return t('development.blocked')
  }
}

export function DiscussionPanel({ detail, revision, onChanged }: { detail: PageDetail; revision: Revision; onChanged: () => Promise<void> }) {
  const { t } = useI18n()
  const [comment, setComment] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const comments = detail.comments.filter((entry) => entry.revisionId === revision.id)
  const run = async (action: () => Promise<unknown>) => {
    setBusy(true); setError('')
    try { await action(); await onChanged() } catch (reason) { setError(reason instanceof Error ? reason.message : t('review.failed')) }
    finally { setBusy(false) }
  }
  const addComment = (event: FormEvent) => {
    event.preventDefault()
    if (!comment.trim()) return
    void run(async () => {
      await api.comment({ pageId: detail.page.id, revisionId: revision.id, body: comment.trim() })
      setComment('')
    })
  }
  return <details className="discussion-accordion">
    <summary><span><MessageSquare size={16} />{t('review.discussion', { count: comments.length })}</span><ChevronDown size={17} /></summary>
    <div className="discussion-panel-content">
      <div className="comment-list">
        {comments.map((entry) => <CommentThread key={entry.id} comment={entry} onReply={(body) => run(() => api.comment({ pageId: detail.page.id, revisionId: entry.revisionId, body, parentCommentId: entry.id }))} />)}
      </div>
      <form className="comment-form" onSubmit={addComment}>
        <label>{t('review.commentLabel')}<textarea rows={3} value={comment} onChange={(event) => setComment(event.target.value)} onKeyDown={submitTextareaOnEnter} /></label>
        <button className="primary-button" disabled={busy || !comment.trim()}>{t('review.addComment')}</button>
      </form>
      {error && <div className="form-error" role="alert">{error}</div>}
    </div>
  </details>
}

function submitTextareaOnEnter(event: KeyboardEvent<HTMLTextAreaElement>) {
  if (event.key !== 'Enter' || event.shiftKey) return
  event.preventDefault()
  event.currentTarget.form!.requestSubmit()
}

function ParentFeatureBlocker({ blocker }: { blocker: ParentFeatureReviewBlocker }) {
  const { t } = useI18n()
  return <article className="review-blocker parent-feature-blocker"><strong>{t('review.parentFeatureRequired')}</strong><p>{t('review.parentFeatureRequiredBody', { title: blocker.relatedPageTitle })}</p><Link to={`/page/${blocker.relatedPageId}`}>{blocker.relatedPageTitle}</Link></article>
}

function ObjectionCard({ objection, busy, onResolve }: { objection: Objection; busy: boolean; onResolve: () => void }) {
  const { t } = useI18n()
  const resolved = Boolean(objection.resolvedAt)
  return <article className={`review-blocker objection-blocker${resolved ? ' resolved' : ''}`}><div className="review-blocker-meta"><div><span>{objection.author.displayName}</span>{resolved && <span className="objection-resolution">{t('review.resolved')}</span>}</div></div><p>{objection.body}</p>{!resolved && <button type="button" disabled={busy} onClick={onResolve}><Check size={13} />{t('review.markResolved')}</button>}</article>
}

function CommentThread({ comment, onReply }: { comment: Comment; onReply: (body: string) => Promise<void> }) {
  const { locale, t } = useI18n()
  const [replying, setReplying] = useState(false)
  const [body, setBody] = useState('')
  return <article className="comment-thread">
    <div className="comment-meta"><span className="avatar small">{comment.author.displayName.slice(0, 1)}</span><div><strong>{comment.author.displayName}</strong><small>{formatDate(comment.createdAt, true, locale)}</small></div></div>
    <p>{comment.body}</p>
    <div className="comment-actions"><button type="button" onClick={() => setReplying(!replying)}><CornerDownRight size={13} />{t('review.reply')}</button></div>
    {comment.replies.map((reply) => <div className="comment-reply" key={reply.id}><strong>{reply.author.displayName}</strong><p>{reply.body}</p></div>)}
    {replying && <form className="reply-form" onSubmit={(event) => { event.preventDefault(); if (!body.trim()) return; void onReply(body.trim()).then(() => { setBody(''); setReplying(false) }) }}><input autoFocus value={body} onChange={(event) => setBody(event.target.value)} placeholder={t('review.replyPlaceholder')} /><button disabled={!body.trim()}>{t('review.send')}</button></form>}
  </article>
}
