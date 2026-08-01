import { useEffect, useState } from 'react'
import { ArrowLeftRight, X } from 'lucide-react'
import { api } from '../../api/client'
import type { PageDetail, Revision } from '../../api/types'
import { formatDate, Markdown, Spinner, StatusBadge } from '../ui'
import { useI18n } from '../../i18n'

export function RevisionHistory({ detail, onClose }: { detail: PageDetail; onClose: () => void }) {
  const { locale, t } = useI18n()
  const [selected, setSelected] = useState<Revision | null>(detail.draftRevision ?? detail.acceptedRevision ?? null)
  const [loading, setLoading] = useState(false)
  const choose = async (id: string) => { setLoading(true); try { setSelected(await api.revision(id)) } finally { setLoading(false) } }
  useEffect(() => { document.body.classList.add('modal-open'); return () => document.body.classList.remove('modal-open') }, [])
  return <div className="modal-backdrop history-backdrop"><div className="history-modal">
    <header><div><span className="eyebrow">{t('history.eyebrow')}</span><h2>{t('history.title')}</h2></div><button className="icon-button" onClick={onClose} aria-label={t('common.close')}><X size={18} /></button></header>
    <div className="history-layout"><aside>{detail.revisions.map((revision) => <button key={revision.id} className={selected?.id === revision.id ? 'active' : ''} onClick={() => void choose(revision.id)}><span>{t('history.revision', { number: revision.number })}</span><StatusBadge status={revision.status} /><small>{revision.createdBy.displayName} · {formatDate(revision.createdAt, true, locale)}</small></button>)}</aside>
      <main>{loading ? <Spinner /> : selected && <><div className="revision-preview-heading"><div><h3>{selected.title}</h3><span>{t('history.revision', { number: selected.number })}</span></div><StatusBadge status={selected.status} /></div>{detail.acceptedRevision && detail.acceptedRevision.id !== selected.id && <div className="compare-note"><ArrowLeftRight size={15} />{t('history.comparing', { number: detail.acceptedRevision.number })}</div>}<div className={detail.acceptedRevision && detail.acceptedRevision.id !== selected.id ? 'revision-compare' : ''}><section><span className="eyebrow">{t('history.selected')}</span><Markdown>{selected.bodyMd || t('history.noDescription')}</Markdown></section>{detail.acceptedRevision && detail.acceptedRevision.id !== selected.id && <section><span className="eyebrow">{t('history.published')}</span><Markdown>{detail.acceptedRevision.bodyMd || t('history.noDescription')}</Markdown></section>}</div></>}</main>
    </div>
  </div></div>
}
