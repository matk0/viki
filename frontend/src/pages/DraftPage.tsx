import { useEffect, useState, type FormEvent } from 'react'
import { Check, Circle, FilePlus2, Pencil, X } from 'lucide-react'
import { useAssistant } from '../assistant'
import { APIError, api } from '../api/client'
import type { AssistantChangeOperation, AssistantDraftProposal, AssistantOperationReview, AssistantOperationReviewValue, Page } from '../api/types'
import { Markdown } from '../components/ui'
import { useRouter } from '../router'
import { useWorkspace } from '../workspace'
import { translate, useI18n, type Locale } from '../i18n'

export function DraftPage({ proposalId }: { proposalId: string }) {
  const { locale, t } = useI18n()
  const assistant = useAssistant()
  const { navigate } = useRouter()
  const { pages, reloadPages } = useWorkspace()
  const streamed = assistant.proposals[proposalId]
  const [proposal, setProposal] = useState<AssistantDraftProposal | null>(streamed ?? null)
  const [loadingOperationKey, setLoadingOperationKey] = useState<string | null>(null)
  const [rejectTarget, setRejectTarget] = useState<AssistantChangeOperation | null>(null)
  const [rejectionReason, setRejectionReason] = useState('')
  const [error, setError] = useState('')
  const conversationHasTurn = Boolean(assistant.conversation?.messages.some((message) => message.id.startsWith(`${proposalId}-`) || message.id === `turn-${proposalId}`))
  const turnAssistantMessage = assistant.conversation?.messages.findLast((message) => message.role === 'assistant' && message.id.startsWith(`${proposalId}-`))
  const clarification = assistant.clarification && (
    assistant.clarification.turnId === proposalId
    || (!assistant.clarification.turnId && conversationHasTurn)
  ) ? assistant.clarification : null

  useEffect(() => {
    if (streamed) setProposal(streamed)
  }, [streamed])

  useEffect(() => {
    if (proposal?.status !== 'published') return
    navigate(publishedProposalDestination(proposal), true)
  }, [navigate, proposal])

  useEffect(() => {
    if (proposal || assistant.error || clarification || assistant.loading) return
    let cancelled = false
    let timer: number | undefined
    const load = async () => {
      try {
        const next = await api.draftProposal(proposalId)
        if (!cancelled) setProposal(next)
      } catch (reason) {
        if (cancelled) return
        if (reason instanceof APIError ? reason.status === 404 : (reason as { status?: number })?.status === 404) {
          if (assistant.conversation?.state === 'running') {
            timer = window.setTimeout(() => void load(), 750)
            return
          }
          setError(draftUnavailableMessage(assistant.conversation?.state, turnAssistantMessage?.content, t))
          return
        }
        setError(reason instanceof Error ? reason.message : t('proposal.loadFailed'))
      }
    }
    void load()
    return () => {
      cancelled = true
      if (timer) window.clearTimeout(timer)
    }
  }, [assistant.conversation?.state, assistant.error, assistant.loading, clarification, proposal, proposalId, turnAssistantMessage?.content])

  const reviewOperation = async (operation: AssistantChangeOperation, value: AssistantOperationReviewValue, reason = '') => {
    const currentProposal = proposal!
    const key = operationReviewKey(operation, currentProposal.operations)
    setLoadingOperationKey(key)
    setError('')
    try {
      const reviewed = await api.reviewDraftProposalOperation(proposalId, key, value, reason, operation.kind === 'feature')
      if (reviewed.status === 'published') {
        await reloadPages()
        navigate(publishedProposalDestination(reviewed), true)
      } else {
        setProposal(reviewed)
        if (reviewed.status !== 'awaiting_approval') await reloadPages()
      }
      return true
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('proposal.decisionFailed'))
      return false
    } finally {
      setLoadingOperationKey(null)
    }
  }

  const reject = async (event: FormEvent) => {
    event.preventDefault()
    const reason = rejectionReason.trim()
    if (!reason || !rejectTarget) return
    if (await reviewOperation(rejectTarget, 'reject', reason)) {
      setRejectTarget(null)
      setRejectionReason('')
    }
  }

  const closeRejectDialog = () => {
    if (loadingOperationKey) return
    setRejectTarget(null)
    setRejectionReason('')
    setError('')
  }

  return <div className="draft-page page-container">
    <header className="draft-page-heading">
      <h1>{t('proposal.title')}</h1>
      <p>{proposal?.summary ?? t('proposal.description')}</p>
    </header>

    {!proposal && !assistant.error && !clarification && !error && <GeneratingDraft state={assistant.activity?.state} />}

    {!proposal && clarification && <form className="clarification-card draft-clarification" onSubmit={(event) => void assistant.respondToClarification(event)}>
      <h2>{t('proposal.clarification')}</h2>
      <p>{clarification.message}</p>
      {clarification.choices && clarification.choices.length > 0 && <div className="clarification-choices">{clarification.choices.map((choice) => <button type="button" key={choice} onClick={() => assistant.setClarificationResponse(choice)}>{choice}</button>)}</div>}
      <label><span>{t('proposal.yourAnswer')}</span><textarea autoFocus rows={3} value={assistant.clarificationResponse} onChange={(event) => assistant.setClarificationResponse(event.target.value)} /></label>
      <button type="submit" disabled={!assistant.clarificationResponse.trim()}>{t('proposal.continue')}</button>
    </form>}

    {proposal && <>
      <section className="proposal-operations" aria-label={t('proposal.changes')}>
        {groupProposalOperations(proposal.operations).map((group, groupIndex) => <div className="proposal-operation-group" key={operationKey(group.parent, proposal.operations.indexOf(group.parent))}>
          <OperationPreview
            operation={group.parent}
            operations={proposal.operations}
            pages={pages}
            review={operationReview(proposal, group.parent)}
            reviewable={proposal.status === 'awaiting_approval'}
            disabled={loadingOperationKey !== null}
            onApprove={() => void reviewOperation(group.parent, 'approve')}
            onReject={() => { setError(''); setRejectionReason(''); setRejectTarget(group.parent) }}
          />
          {group.children.length > 0 && <div className="proposal-operation-children" aria-label={t('proposal.newTerms', { title: group.parent.content.title })}>
            {group.children.map((child) => <div className="proposal-operation-child" key={operationKey(child, proposal.operations.indexOf(child))}>
              <OperationPreview
                operation={child}
                operations={proposal.operations}
                pages={pages}
                review={operationReview(proposal, child)}
                reviewable={proposal.status === 'awaiting_approval'}
                disabled={loadingOperationKey !== null}
                onApprove={() => void reviewOperation(child, 'approve')}
                onReject={() => { setError(''); setRejectionReason(''); setRejectTarget(child) }}
              />
            </div>)}
          </div>}
        </div>)}
      </section>

      {proposal.status === 'published' && proposal.publishedRevisions.length > 0 && <div className="published-links">
        {proposal.publishedRevisions.map((revision) => <a key={revision.id} href={`/page/${revision.pageId}?revision=${revision.id}`}>{revision.title}<span>{t('proposal.openAccepted')}</span></a>)}
      </div>}

      {proposal.status === 'awaiting_approval' && rejectTarget && <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && closeRejectDialog()}>
        <form className="modal-card proposal-rejection-dialog" role="dialog" aria-modal="true" aria-labelledby="proposal-rejection-title" onSubmit={(event) => void reject(event)}>
          <div className="modal-heading">
            <div><span className="eyebrow">{t('proposal.reviewEyebrow')}</span><h2 id="proposal-rejection-title">{t('proposal.rejectTitle', { title: rejectTarget.content.title })}</h2></div>
            <button type="button" className="icon-button" disabled={loadingOperationKey !== null} onClick={closeRejectDialog} aria-label={t('common.close')}><X size={18} /></button>
          </div>
          <p className="proposal-rejection-copy">{rejectTarget.kind === 'feature' ? t('proposal.rejectFeatureBody') : t('proposal.rejectBody')}</p>
          <label>{t('proposal.rejectReason')}<textarea autoFocus required maxLength={2000} rows={5} value={rejectionReason} onChange={(event) => setRejectionReason(event.target.value)} placeholder={t('proposal.rejectPlaceholder')} /></label>
          {error && <div className="form-error" role="alert">{error}</div>}
          <div className="modal-actions"><button type="button" className="secondary-button" disabled={loadingOperationKey !== null} onClick={closeRejectDialog}>{t('common.cancel')}</button><button className="primary-button rejection-button" disabled={loadingOperationKey !== null || !rejectionReason.trim()}>{loadingOperationKey ? t('proposal.rejecting') : t('proposal.rejectKind', { kind: operationKindLabel(rejectTarget, locale).toLocaleLowerCase(locale === 'en' ? 'en' : 'sk') })}</button></div>
        </form>
      </div>}
    </>}

    {((error && !rejectTarget) || (!proposal && assistant.error)) && <div className="form-error" role="alert">{error || assistant.error}</div>}
  </div>
}

function draftUnavailableMessage(state: string | undefined, assistantMessage: string | undefined, t: ReturnType<typeof useI18n>['t']): string {
  if (assistantMessage?.trim().toLocaleLowerCase('en-US') === 'operation interrupted.') return t('proposal.interrupted')
  if (state === 'error') return t('proposal.failed')
  if (state === 'stopped') return t('proposal.stopped')
  return t('proposal.unavailable')
}

function GeneratingDraft({ state }: { state?: string }) {
  const { t } = useI18n()
  const activeLabel = proposalActivityLabel(state ?? 'thinking', t)
  return <section className="draft-generating" aria-live="polite">
    <div className="draft-orbit"><span /><span /><span /></div>
    <div><h2>{t('proposal.generating')}</h2><p>{activeLabel}…</p></div>
    <ol>
      <li className="done"><Check size={14} />{t('proposal.received')}</li>
      <li className="active"><span className="spinner" />{t('proposal.preparing')}</li>
      <li><Circle size={11} />{t('proposal.yourApproval')}</li>
    </ol>
  </section>
}

interface DisplayOperationGroup {
  parent: AssistantChangeOperation
  children: AssistantChangeOperation[]
}

function groupProposalOperations(operations: AssistantChangeOperation[]): DisplayOperationGroup[] {
  const newConcepts = new Map(
    operations
      .filter((operation) => operation.operation === 'create' && operation.kind === 'concept' && operation.clientKey)
      .map((operation) => [operation.clientKey!, operation]),
  )
  const nestedConceptKeys = new Set<string>()
  const groups = operations
    .filter((operation) => operation.kind !== 'concept')
    .map((parent) => {
      const children = parent.content.references.flatMap((reference) => {
        const key = reference.targetClientKey
        if (!key || nestedConceptKeys.has(key)) return []
        const concept = newConcepts.get(key)
        if (!concept) return []
        nestedConceptKeys.add(key)
        return [concept]
      })
      return { parent, children }
    })

  for (const operation of operations) {
    if (operation.kind !== 'concept' || (operation.clientKey && nestedConceptKeys.has(operation.clientKey))) continue
    groups.push({ parent: operation, children: [] })
  }

  return groups
}

function operationKey(operation: AssistantChangeOperation, index: number): string {
  return operation.clientKey || operation.pageId || `operation-${index + 1}`
}

function operationReviewKey(operation: AssistantChangeOperation, operations: AssistantChangeOperation[]): string {
  return operationKey(operation, operations.indexOf(operation))
}

function operationReview(proposal: AssistantDraftProposal, operation: AssistantChangeOperation): AssistantOperationReview | undefined {
  const key = operationReviewKey(operation, proposal.operations)
  return proposal.operationReviews.find((review) => review.operationKey === key)
}

function publishedProposalDestination(proposal: AssistantDraftProposal): string {
  const approvedOperations = proposal.operationReviews.length > 0
    ? proposal.operations.filter((operation) => operationReview(proposal, operation)?.value === 'approve')
    : proposal.operations
  const preferredKind = (['feature', 'scenario', 'concept'] as const)
    .find((kind) => approvedOperations.some((operation) => operation.kind === kind))
  const preferredIndex = preferredKind
    ? approvedOperations.findIndex((operation) => operation.kind === preferredKind)
    : 0
  const revision = proposal.publishedRevisions[preferredIndex] ?? proposal.publishedRevisions[0]
  return revision ? `/page/${revision.pageId}?revision=${revision.id}` : '/drafts'
}

function operationKindLabel(operation: AssistantChangeOperation, locale: Locale): string {
  return operation.kind === 'concept' ? translate(locale, 'kind.concept') : operation.kind === 'feature' ? translate(locale, 'kind.feature') : translate(locale, 'kind.scenario')
}

interface OperationPreviewProps {
  operation: AssistantChangeOperation
  operations: AssistantChangeOperation[]
  pages: Page[]
  review?: AssistantOperationReview
  reviewable: boolean
  disabled: boolean
  onApprove: () => void
  onReject: () => void
}

function OperationPreview({ operation, operations, pages, review, reviewable, disabled, onApprove, onReject }: OperationPreviewProps) {
  const { locale, t } = useI18n()
  const kind = operationKindLabel(operation, locale)
  const conceptReferences = operation.kind === 'concept' ? [] : operation.content.references.flatMap((reference) => {
    const resolved = resolveConceptReference(reference, operations, pages)
    return resolved ? [resolved] : []
  })
  const classes = ['proposal-operation', reviewable ? 'reviewable' : '', review ? `reviewed-${review.value}` : ''].filter(Boolean).join(' ')
  return <article className={classes} id={operation.clientKey ? draftOperationAnchor(operation.clientKey) : undefined} tabIndex={reviewable ? 0 : undefined}>
    {review && <span className={`operation-review-status ${review.value}`}>{review.value === 'approve' ? t('proposal.approved') : t('proposal.rejected')}</span>}
    <header>
      <span>{operation.operation === 'create' ? <FilePlus2 size={15} /> : <Pencil size={15} />}{operation.operation === 'create' ? t('proposal.create') : t('proposal.edit')} · {kind}</span>
      <h2>{operation.content.title}</h2>
      <small>/{operation.slug}</small>
    </header>
    {operation.content.bodyMd && <Markdown inlineLinks={conceptReferences.map((reference) => ({ href: reference.href, label: reference.title, className: 'concept-reference' }))}>{operation.content.bodyMd}</Markdown>}
    {operation.content.aliases.length > 0 && <div className="tag-list">{operation.content.aliases.map((alias) => <span key={alias}>{alias}</span>)}</div>}
    {operation.content.steps.length > 0 && <div className="bdd-steps">{operation.content.steps.map((step, stepIndex) => <div key={step.id || step.stableId || stepIndex}><strong>{t(bddStepKeys[step.keyword])}</strong><span>{step.text}</span></div>)}</div>}
    {reviewable && <div className="proposal-operation-actions" aria-label={t('proposal.decision', { title: operation.content.title })}>
      <button type="button" className="secondary-button reject-operation" aria-label={`${t('proposal.reject')} ${operation.content.title}`} aria-pressed={review?.value === 'reject'} disabled={disabled} onClick={onReject}><X size={15} />{t('proposal.reject')}</button>
      <button type="button" className="primary-button approve-operation" aria-label={`${t('proposal.approve')} ${operation.content.title}`} aria-pressed={review?.value === 'approve'} disabled={disabled} onClick={onApprove}><Check size={15} />{t('proposal.approve')}</button>
    </div>}
  </article>
}

function resolveConceptReference(reference: AssistantChangeOperation['content']['references'][number], operations: AssistantChangeOperation[], pages: Page[]): { href: string; title: string } | null {
  if (reference.targetClientKey) {
    const operation = operations.find((candidate) => candidate.clientKey === reference.targetClientKey && candidate.kind === 'concept')
    if (!operation) return null
    return { href: `#${draftOperationAnchor(reference.targetClientKey)}`, title: reference.targetTitle || operation.content.title }
  }
  if (reference.targetPageId) {
    const page = pages.find((candidate) => candidate.id === reference.targetPageId && candidate.kind === 'concept')
    if (!page) return null
    return { href: `/page/${page.id}`, title: reference.targetTitle || page.title }
  }
  return null
}

function draftOperationAnchor(clientKey: string): string {
  return `draft-operation-${clientKey}`
}

const bddStepKeys = { given: 'bdd.given', when: 'bdd.when', then: 'bdd.then', and: 'bdd.and', but: 'bdd.but' } as const

function proposalActivityLabel(state: string, t: ReturnType<typeof useI18n>['t']): string {
  if (state === 'submitting') return t('proposal.activity.submitting')
  if (state === 'thinking') return t('proposal.activity.thinking')
  if (state === 'searching') return t('proposal.activity.searching')
  if (state === 'reading') return t('proposal.activity.reading')
  if (['drafting', 'editing', 'writing'].includes(state)) return t('proposal.activity.drafting')
  if (state === 'applying') return t('proposal.activity.applying')
  if (state === 'awaiting_approval') return t('proposal.activity.awaiting')
  return t('proposal.generating')
}
