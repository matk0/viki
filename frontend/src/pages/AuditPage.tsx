import { useEffect, useState } from 'react'
import { Clock3 } from 'lucide-react'
import { api } from '../api/client'
import type { AuditEvent } from '../api/types'
import { EmptyState, formatDate, Spinner } from '../components/ui'
import { useI18n, type Locale, translate } from '../i18n'

export function AuditPage() {
  const { locale, t } = useI18n()
  const [events, setEvents] = useState<AuditEvent[] | null>(null)
  const [error, setError] = useState('')
  useEffect(() => { api.audit().then((result) => setEvents(result.events)).catch((error) => setError(error.message)) }, [])
  return <div className="page-container audit-page">
    <header className="page-heading"><div><h1>{t('audit.title')}</h1><p>{t('audit.description')}</p></div></header>
    {!events && !error ? <Spinner /> : error ? <EmptyState title={t('audit.unavailable')} body={error} /> : <div className="panel audit-list">{events?.map((event) => <div className="audit-row" key={event.id}><span className="audit-icon"><Clock3 size={15} /></span><div><p><strong>{event.actor?.displayName ?? t('common.system')}</strong> {auditLabel(event.action, locale)}</p><small>{event.entityType}{event.entityId ? ` · ${event.entityId.slice(0, 8)}` : ''}</small></div><time>{formatDate(event.createdAt, true, locale)}</time></div>)}</div>}
  </div>
}

function auditLabel(action: string, locale: Locale): string {
  const labels = {
    'page.created': 'audit.pageCreated', 'revision.saved': 'audit.revisionSaved', 'revision.approved': 'audit.revisionApproved', 'revision.published': 'audit.revisionPublished',
    'comment.created': 'audit.commentCreated', 'objection.created': 'audit.objectionCreated', 'objection.resolved': 'audit.objectionResolved',
    'assistant.drafts_created': 'audit.assistantDrafts', 'assistant.proposal_created': 'audit.assistantProposal',
    'assistant.proposal_published': 'audit.assistantPublished', 'assistant.proposal_discarded': 'audit.assistantDiscarded', 'ai.drafts_created': 'audit.assistantDrafts',
  } as const
  const key = labels[action as keyof typeof labels]
  return key ? translate(locale, key) : action
}
