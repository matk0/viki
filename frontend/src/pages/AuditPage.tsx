import { useEffect, useState } from 'react'
import { Clock3 } from 'lucide-react'
import { api } from '../api/client'
import type { AuditEvent } from '../api/types'
import { EmptyState, formatDate, Spinner } from '../components/ui'

export function AuditPage() {
  const [events, setEvents] = useState<AuditEvent[] | null>(null)
  const [error, setError] = useState('')
  useEffect(() => { api.audit().then((result) => setEvents(result.events)).catch((error) => setError(error.message)) }, [])
  return <div className="page-container audit-page">
    <header className="page-heading"><div><h1>História zmien</h1><p>Nemenný záznam ľudských aj asistenčných akcií v pracovnom priestore.</p></div></header>
    {!events && !error ? <Spinner /> : error ? <EmptyState title="História sa nedá načítať" body={error} /> : <div className="panel audit-list">{events?.map((event) => <div className="audit-row" key={event.id}><span className="audit-icon"><Clock3 size={15} /></span><div><p><strong>{event.actor?.displayName ?? 'Systém'}</strong> {auditLabel(event.action)}</p><small>{event.entityType}{event.entityId ? ` · ${event.entityId.slice(0, 8)}` : ''}</small></div><time>{formatDate(event.createdAt)}</time></div>)}</div>}
  </div>
}

function auditLabel(action: string): string {
  const labels: Record<string, string> = {
    'page.created': 'vytvoril(a) stránku',
    'revision.saved': 'uložil(a) nový koncept',
    'revision.published': 'publikoval(a) revíziu',
    'comment.created': 'pridal(a) komentár',
    'comment.resolved': 'vyriešil(a) komentár',
    'vote.recorded': 'hlasoval(a) o revízii',
    'assistant.drafts_created': 'vytvoril(a) koncepty cez asistenta',
    'assistant.proposal_created': 'pripravil(a) návrh cez asistenta',
    'assistant.proposal_published': 'schválil(a) návrh asistenta',
    'assistant.proposal_discarded': 'odmietol/odmietla návrh asistenta',
    'ai.drafts_created': 'vytvoril(a) koncepty cez asistenta',
  }
  return labels[action] ?? action
}
