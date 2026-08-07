import { useState } from 'react'
import { ArrowLeftRight, X } from 'lucide-react'
import { api } from '../../api/client'
import type { PageDetail, Revision } from '../../api/types'
import { formatDate, Markdown, Spinner, StatusBadge } from '../ui'
import { useI18n } from '../../i18n'
import { Modal } from '../Modal'

export function RevisionHistory({ detail, onClose }: { detail: PageDetail; onClose: () => void }) {
  const { locale, t } = useI18n()
  const [selected, setSelected] = useState<Revision | null>(detail.draftRevision ?? detail.approvedRevision ?? null)
  const [loading, setLoading] = useState(false)
  const choose = async (id: string) => { setLoading(true); try { setSelected(await api.revision(id)) } finally { setLoading(false) } }
  const comparisonRevision = selected && detail.approvedRevision?.id !== selected.id ? detail.approvedRevision : undefined
  return <Modal className="history-backdrop" onClose={onClose}><div className="history-modal" role="dialog" aria-modal="true" aria-labelledby="revision-history-title">
    <header><div><h2 id="revision-history-title">{t('history.title')}</h2></div><button className="icon-button" onClick={onClose} aria-label={t('common.close')}><X size={18} /></button></header>
    <div className="history-layout"><aside>{detail.revisions.map((revision) => <button key={revision.id} className={selected?.id === revision.id ? 'active' : ''} onClick={() => void choose(revision.id)}><span>{t('history.revision', { number: revision.number })}</span><StatusBadge status={revision.status} /><small>{revision.createdBy.displayName} · {formatDate(revision.createdAt, true, locale)}</small></button>)}</aside>
      <main>{loading ? <Spinner /> : selected && <><div className="revision-preview-heading"><div><h3>{selected.title}</h3><span>{t('history.revision', { number: selected.number })}</span></div><StatusBadge status={selected.status} /></div>{comparisonRevision && <div className="compare-note"><ArrowLeftRight size={15} />{t('history.comparing', { number: comparisonRevision.number })}</div>}<div className={comparisonRevision ? 'revision-compare' : ''}><section>{comparisonRevision && <span className="eyebrow">{t('history.selected')}</span>}<Markdown>{selected.bodyMd || t('history.noDescription')}</Markdown></section>{comparisonRevision && <section><span className="eyebrow">{t('history.approved')}</span><Markdown>{comparisonRevision.bodyMd || t('history.noDescription')}</Markdown></section>}</div></>}</main>
    </div>
  </div></Modal>
}
