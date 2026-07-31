import { useState, type FormEvent } from 'react'
import { X } from 'lucide-react'
import { api } from '../api/client'
import type { RevisionContent } from '../api/types'
import { useRouter } from '../router'
import { useWorkspace } from '../workspace'
import { VikiSelect } from './VikiSelect'

function slugify(value: string) {
  return value.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
}

export function NewPageDialog({ initialKind, onClose }: { initialKind: 'primitive' | 'scenario'; onClose: () => void }) {
  const { reloadPages } = useWorkspace()
  const { navigate } = useRouter()
  const kind = initialKind
  const [primitiveKind, setPrimitiveKind] = useState<'noun' | 'verb'>('noun')
  const [title, setTitle] = useState('')
  const [slug, setSlug] = useState('')
  const [customSlug, setCustomSlug] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    setSaving(true)
    const content: RevisionContent = { title: title.trim(), bodyMd: '', aliases: [], steps: [], references: [] }
    try {
      const result = await api.createPage({ kind, ...(kind === 'primitive' ? { primitiveKind } : {}), slug, content })
      await reloadPages(); onClose(); navigate(`/page/${result.page.id}`)
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Stránku sa nepodarilo vytvoriť.') }
    finally { setSaving(false) }
  }
  return <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
    <form className="modal-card new-page-dialog" onSubmit={(event) => void submit(event)}>
      <div className="modal-heading"><div><span className="eyebrow">Nový koncept</span><h2>Vytvoriť {kind === 'primitive' ? 'pojem' : 'scenár'}</h2></div><button type="button" className="icon-button" onClick={onClose} aria-label="Zavrieť"><X size={18} /></button></div>
      {kind === 'primitive' && <div className="select-field"><span>Druh pojmu</span><VikiSelect ariaLabel="Druh pojmu" listboxLabel="Druhy pojmov" value={primitiveKind} onChange={(value) => setPrimitiveKind(value as 'noun' | 'verb')} options={[{ value: 'noun', label: 'Podstatné meno' }, { value: 'verb', label: 'Sloveso' }]} /></div>}
      <label>Názov<input autoFocus required value={title} onChange={(event) => { setTitle(event.target.value); if (!customSlug) setSlug(slugify(event.target.value)) }} placeholder="Napríklad Dostupnosť služby" /></label>
      <label>Slug<input required pattern="[a-z0-9]+(?:-[a-z0-9]+)*" value={slug} onChange={(event) => { setCustomSlug(true); setSlug(event.target.value) }} placeholder="dostupnost-sluzby" /><small>Malé písmená bez diakritiky, oddelené pomlčkou.</small></label>
      {error && <div className="form-error" role="alert">{error}</div>}
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Zrušiť</button><button className="primary-button" disabled={saving}>{saving ? 'Vytváram…' : 'Vytvoriť koncept'}</button></div>
    </form>
  </div>
}
