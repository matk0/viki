import { useEffect, useState } from 'react'
import { Circle, Files } from 'lucide-react'
import { api } from '../api/client'
import type { AssistantDraftProposal } from '../api/types'
import { EmptyState, formatDate, Spinner } from '../components/ui'
import { Link } from '../router'
import { translate, useI18n, type Locale } from '../i18n'

export function DraftsPage() {
  const { locale, t } = useI18n()
  const [proposals, setProposals] = useState<AssistantDraftProposal[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    api.draftProposals()
      .then((result) => setProposals(result.proposals))
      .catch((reason) => setError(reason instanceof Error ? reason.message : t('drafts.loadFailed')))
  }, [])

  const pending = proposals?.filter((proposal) => proposal.status === 'awaiting_approval') ?? []

  return <div className="page-container drafts-index-page">
    <header className="page-heading">
      <div><h1>{t('drafts.title')}</h1><p>{t('drafts.description')}</p></div>
    </header>

    {!proposals && !error ? <Spinner label={t('drafts.loading')} />
      : error ? <EmptyState title={t('drafts.unavailable')} body={error} />
        : pending.length === 0 ? <EmptyState icon={<Files />} title={t('drafts.empty')} body={t('drafts.emptyBody')} />
          : <div className="draft-groups">
            <DraftGroup title={t('drafts.awaiting')} proposals={pending} locale={locale} />
          </div>}
  </div>
}

function DraftGroup({ title, proposals, locale }: { title: string; proposals: AssistantDraftProposal[]; locale: Locale }) {
  const { t } = useI18n()
  return <section className="draft-group">
    <header><h2>{title}</h2><span>{proposals.length}</span></header>
    <div className="panel draft-list">
      {proposals.map((proposal) => <Link className={`draft-list-item ${proposal.status}`} to={`/drafts/${proposal.id}`} key={proposal.id}>
        <span className="draft-list-icon"><Circle size={16} /></span>
        <span className="draft-list-copy">
          <strong>{proposal.summary || t('drafts.changeProposal')}</strong>
          <small>{changeCount(proposal.operations.length, locale)} · {formatDate(proposal.createdAt, true, locale)}</small>
        </span>
        <span className="draft-status">{t('drafts.awaitingStatus')}</span>
      </Link>)}
    </div>
  </section>
}

function changeCount(count: number, locale: Locale): string {
  const key = count === 1 ? 'drafts.change.one' : locale === 'sk' && count >= 2 && count <= 4 ? 'drafts.change.few' : 'drafts.change.other'
  return translate(locale, key, { count })
}
