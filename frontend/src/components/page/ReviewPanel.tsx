import { Check, CheckCircle2, CornerDownRight, MessageSquare, Send, ThumbsDown, ThumbsUp } from 'lucide-react'
import { useMemo, useState, type FormEvent } from 'react'
import { api } from '../../api/client'
import type { Comment, PageDetail, Revision } from '../../api/types'
import { VikiSelect } from '../VikiSelect'
import { formatDate } from '../ui'
import { useI18n } from '../../i18n'

export function ReviewPanel({ detail, revision, onChanged }: { detail: PageDetail; revision: Revision; onChanged: () => Promise<void> }) {
  const { t } = useI18n()
  const [comment, setComment] = useState('')
  const [anchor, setAnchor] = useState('revision')
  const [rejectReason, setRejectReason] = useState('')
  const [showReject, setShowReject] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const votes = detail.votes.filter((vote) => vote.revisionId === revision.id)
  const comments = detail.comments.filter((entry) => entry.revisionId === revision.id || (entry.blocking && !entry.resolvedAt))
  const blockers = useMemo(() => detail.comments.filter((entry) => entry.blocking && !entry.resolvedAt), [detail.comments])
  const run = async (action: () => Promise<unknown>) => {
    setBusy(true); setError('')
    try { await action(); await onChanged() } catch (reason) { setError(reason instanceof Error ? reason.message : t('review.failed')) }
    finally { setBusy(false) }
  }
  const addComment = (event: FormEvent) => {
    event.preventDefault()
    if (!comment.trim()) return
    void run(async () => {
      const [anchorKind, anchorId] = anchor === 'revision' ? [undefined, undefined] : anchor.split(':', 2)
      await api.comment({ pageId: detail.page.id, revisionId: revision.id, body: comment.trim(), anchorKind, anchorId })
      setComment('')
    })
  }
  const reject = () => {
    void run(async () => { await api.vote(revision.id, 'reject', rejectReason.trim()); setRejectReason(''); setShowReject(false) })
  }
  return <div className="review-panel-content">
    <div className="review-summary">
      <div><strong>{t('review.revision', { number: revision.number })}</strong><span>{revision.status === 'draft' ? t('review.awaiting') : t('review.published')}</span></div>
      <div className="vote-counts"><span>{t('review.approvals', { count: votes.filter((vote) => vote.value === 'approve').length })}</span><span>{t('review.rejections', { count: votes.filter((vote) => vote.value === 'reject').length })}</span></div>
    </div>
    {revision.status === 'draft' && <>
      <div className="vote-actions">
        <button className="approve-button" disabled={busy} onClick={() => void run(() => api.vote(revision.id, 'approve'))}><ThumbsUp size={16} />{t('review.approve')}</button>
        <button className="reject-button" disabled={busy} onClick={() => setShowReject(!showReject)}><ThumbsDown size={16} />{t('review.reject')}</button>
      </div>
      {showReject && <div className="reject-box"><label>{t('review.reason')}<textarea autoFocus rows={3} value={rejectReason} onChange={(event) => setRejectReason(event.target.value)} placeholder={t('review.reasonPlaceholder')} /></label><button className="danger-button" disabled={busy || !rejectReason.trim()} onClick={reject}>{t('review.submitObjection')}</button></div>}
      {blockers.length > 0 && <div className="blocker-callout"><strong>{blockers.length === 1 ? t('review.openBlocker.one') : t('review.openBlocker.other', { count: blockers.length })}</strong><span>{t('review.resolveBeforePublish')}</span></div>}
    </>}
    <div className="comments-heading"><MessageSquare size={16} /><strong>{t('review.comments')}</strong><span>{comments.length}</span></div>
    <div className="comment-list">
      {comments.length === 0 && <p className="muted">{t('review.noComments')}</p>}
      {comments.map((entry) => <CommentThread key={entry.id} comment={entry} busy={busy} onResolve={() => void run(() => api.resolveComment(entry.id))} onReply={(body) => run(() => api.comment({ pageId: detail.page.id, revisionId: revision.id, body, parentCommentId: entry.id }))} />)}
    </div>
    <form className="comment-form" onSubmit={addComment}><VikiSelect compact className="comment-anchor-select" ariaLabel={t('review.commentAnchor')} listboxLabel={t('review.commentAnchors')} value={anchor} onChange={setAnchor} options={[{ value: 'revision', label: t('review.wholeRevision') }, { value: 'field:title', label: t('new.title') }, { value: 'field:bodyMd', label: t('review.description') }, ...revision.steps.map((step, index) => ({ value: `step:${step.stableId ?? step.id}`, label: t('review.step', { number: index + 1 }) }))]} /><textarea rows={3} value={comment} onChange={(event) => setComment(event.target.value)} placeholder={t('review.addComment')} /><button className="icon-button filled" disabled={busy || !comment.trim()} aria-label={t('review.sendComment')}><Send size={15} /></button></form>
    {error && <div className="form-error" role="alert">{error}</div>}
    {revision.status === 'draft' && <button className="publish-button" disabled={busy || blockers.length > 0} onClick={() => void run(() => api.publish(revision.id))}><CheckCircle2 size={17} />{blockers.length ? t('review.publishBlocked') : t('review.publish')}</button>}
  </div>
}

function CommentThread({ comment, busy, onResolve, onReply }: { comment: Comment; busy: boolean; onResolve: () => void; onReply: (body: string) => Promise<void> }) {
  const { locale, t } = useI18n()
  const [replying, setReplying] = useState(false)
  const [body, setBody] = useState('')
  return <article className={`comment-thread ${comment.blocking && !comment.resolvedAt ? 'blocking' : ''}`}>
    <div className="comment-meta"><span className="avatar small">{comment.author.displayName.slice(0, 1)}</span><div><strong>{comment.author.displayName}</strong><small>{formatDate(comment.createdAt, true, locale)}</small></div>{comment.blocking && <span className={`comment-state ${comment.resolvedAt ? 'resolved' : ''}`}>{comment.resolvedAt ? t('review.resolved') : t('review.blocking')}</span>}</div>
    {comment.anchorKind && <span className="comment-anchor">{comment.anchorKind === 'step' ? t('review.behaviorStep') : comment.anchorId === 'title' ? t('new.title') : t('review.description')}</span>}<p>{comment.body}</p>
    <div className="comment-actions"><button type="button" onClick={() => setReplying(!replying)}><CornerDownRight size={13} />{t('review.reply')}</button>{comment.blocking && !comment.resolvedAt && <button type="button" disabled={busy} onClick={onResolve}><Check size={13} />{t('review.markResolved')}</button>}</div>
    {comment.replies.map((reply) => <div className="comment-reply" key={reply.id}><strong>{reply.author.displayName}</strong><p>{reply.body}</p></div>)}
    {replying && <form className="reply-form" onSubmit={(event) => { event.preventDefault(); if (!body.trim()) return; void onReply(body.trim()).then(() => { setBody(''); setReplying(false) }) }}><input autoFocus value={body} onChange={(event) => setBody(event.target.value)} placeholder={t('review.replyPlaceholder')} /><button disabled={!body.trim()}>{t('review.send')}</button></form>}
  </article>
}
