import { useEffect, useState } from 'react'
import { ArrowLeftRight, X } from 'lucide-react'
import { api } from '../../api/client'
import type { PageDetail, Revision } from '../../api/types'
import { formatDate, Markdown, Spinner, StatusBadge } from '../ui'

export function RevisionHistory({ detail, onClose }: { detail: PageDetail; onClose: () => void }) {
  const [selected, setSelected] = useState<Revision | null>(detail.draftRevision ?? detail.acceptedRevision ?? null)
  const [loading, setLoading] = useState(false)
  const choose = async (id: string) => { setLoading(true); try { setSelected(await api.revision(id)) } finally { setLoading(false) } }
  useEffect(() => { document.body.classList.add('modal-open'); return () => document.body.classList.remove('modal-open') }, [])
  return <div className="modal-backdrop history-backdrop"><div className="history-modal">
    <header><div><span className="eyebrow">Nemenná auditná stopa</span><h2>História revízií</h2></div><button className="icon-button" onClick={onClose} aria-label="Zavrieť"><X size={18} /></button></header>
    <div className="history-layout"><aside>{detail.revisions.map((revision) => <button key={revision.id} className={selected?.id === revision.id ? 'active' : ''} onClick={() => void choose(revision.id)}><span>Revízia #{revision.number}</span><StatusBadge status={revision.status} /><small>{revision.createdBy.displayName} · {formatDate(revision.createdAt)}</small></button>)}</aside>
      <main>{loading ? <Spinner /> : selected && <><div className="revision-preview-heading"><div><h3>{selected.title}</h3><span>Revízia #{selected.number}</span></div><StatusBadge status={selected.status} /></div>{detail.acceptedRevision && detail.acceptedRevision.id !== selected.id && <div className="compare-note"><ArrowLeftRight size={15} />Porovnávate s publikovanou revíziou #{detail.acceptedRevision.number}</div>}<div className={detail.acceptedRevision && detail.acceptedRevision.id !== selected.id ? 'revision-compare' : ''}><section><span className="eyebrow">Vybraná revízia</span><Markdown>{selected.bodyMd || '_Bez opisu_'}</Markdown></section>{detail.acceptedRevision && detail.acceptedRevision.id !== selected.id && <section><span className="eyebrow">Publikovaná revízia</span><Markdown>{detail.acceptedRevision.bodyMd || '_Bez opisu_'}</Markdown></section>}</div></>}</main>
    </div>
  </div></div>
}
