import { useEffect, useMemo, useState } from 'react'
import { Check, Circle, Files, X } from 'lucide-react'
import { api } from '../api/client'
import type { AssistantDraftProposal, AssistantDraftProposalStatus } from '../api/types'
import { EmptyState, formatDate, Spinner } from '../components/ui'
import { Link } from '../router'

export function DraftsPage() {
  const [proposals, setProposals] = useState<AssistantDraftProposal[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    api.draftProposals()
      .then((result) => setProposals(result.proposals))
      .catch((reason) => setError(reason instanceof Error ? reason.message : 'Koncepty sa nepodarilo načítať.'))
  }, [])

  const pending = useMemo(() => proposals?.filter((proposal) => proposal.status === 'awaiting_approval') ?? [], [proposals])
  const finished = useMemo(() => proposals?.filter((proposal) => proposal.status !== 'awaiting_approval') ?? [], [proposals])

  return <div className="page-container drafts-index-page">
    <header className="page-heading">
      <div><h1>Koncepty</h1><p>Návrhy zmien pripravené viki asistentom na kontrolu a publikovanie.</p></div>
    </header>

    {!proposals && !error ? <Spinner label="Načítavam koncepty…" />
      : error ? <EmptyState title="Koncepty sa nedajú načítať" body={error} />
        : proposals?.length === 0 ? <EmptyState icon={<Files />} title="Zatiaľ žiadne koncepty" body="V režime Úpravy požiadajte viki asistenta o prípravu zmien." />
          : <div className="draft-groups">
            {pending.length > 0 && <DraftGroup title="Čakajú na schválenie" proposals={pending} />}
            {finished.length > 0 && <DraftGroup title="Vybavené" proposals={finished} />}
          </div>}
  </div>
}

function DraftGroup({ title, proposals }: { title: string; proposals: AssistantDraftProposal[] }) {
  return <section className="draft-group">
    <header><h2>{title}</h2><span>{proposals.length}</span></header>
    <div className="panel draft-list">
      {proposals.map((proposal) => <Link className={`draft-list-item ${proposal.status}`} to={`/drafts/${proposal.id}`} key={proposal.id}>
        <span className="draft-list-icon">{statusIcon(proposal.status)}</span>
        <span className="draft-list-copy">
          <strong>{proposal.summary || 'Návrh zmien'}</strong>
          <small>{changeCount(proposal.operations.length)} · {formatDate(proposal.createdAt)}</small>
        </span>
        <span className="draft-status">{statusLabel(proposal.status)}</span>
      </Link>)}
    </div>
  </section>
}

function statusIcon(status: AssistantDraftProposalStatus) {
  if (status === 'published') return <Check size={17} />
  if (status === 'discarded') return <X size={16} />
  return <Circle size={16} />
}

function statusLabel(status: AssistantDraftProposalStatus): string {
  if (status === 'published') return 'Publikované'
  if (status === 'discarded') return 'Odmietnuté'
  return 'Čaká na schválenie'
}

function changeCount(count: number): string {
  if (count === 1) return '1 zmena'
  if (count >= 2 && count <= 4) return `${count} zmeny`
  return `${count} zmien`
}
