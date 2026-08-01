import { useState, type FormEvent } from 'react'
import { X } from 'lucide-react'
import { api } from '../api/client'
import type { RevisionContent } from '../api/types'
import { useRouter } from '../router'
import { useWorkspace } from '../workspace'
import { VikiSelect } from './VikiSelect'
import { useI18n } from '../i18n'

function slugify(value: string) {
  return value.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
}

export function NewPageDialog({ initialKind, onClose }: { initialKind: 'concept' | 'feature'; onClose: () => void }) {
  const { t } = useI18n()
  const { reloadPages } = useWorkspace()
  const { navigate } = useRouter()
  const kind = initialKind
  const [conceptKind, setConceptKind] = useState<'noun' | 'verb'>('noun')
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
      const result = await api.createPage({ kind, ...(kind === 'concept' ? { conceptKind } : {}), slug, content })
      await reloadPages(); onClose(); navigate(`/page/${result.page.id}`)
    } catch (reason) { setError(reason instanceof Error ? reason.message : t('new.failed')) }
    finally { setSaving(false) }
  }
  return <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
    <form className="modal-card new-page-dialog" onSubmit={(event) => void submit(event)}>
      <div className="modal-heading"><div><span className="eyebrow">{t('new.eyebrow')}</span><h2>{kind === 'concept' ? t('new.createConcept') : t('new.createFeature')}</h2></div><button type="button" className="icon-button" onClick={onClose} aria-label={t('common.close')}><X size={18} /></button></div>
      {kind === 'concept' && <div className="select-field"><span>{t('new.conceptKind')}</span><VikiSelect ariaLabel={t('new.conceptKind')} listboxLabel={t('new.conceptKinds')} value={conceptKind} onChange={(value) => setConceptKind(value as 'noun' | 'verb')} options={[{ value: 'noun', label: t('new.noun') }, { value: 'verb', label: t('new.verb') }]} /></div>}
      <label>{t('new.title')}<input autoFocus required value={title} onChange={(event) => { setTitle(event.target.value); if (!customSlug) setSlug(slugify(event.target.value)) }} placeholder={t('new.titlePlaceholder')} /></label>
      <label>Slug<input required pattern="[a-z0-9]+(?:-[a-z0-9]+)*" value={slug} onChange={(event) => { setCustomSlug(true); setSlug(event.target.value) }} placeholder="service-availability" /><small>{t('new.slugHelp')}</small></label>
      {error && <div className="form-error" role="alert">{error}</div>}
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>{t('common.cancel')}</button><button className="primary-button" disabled={saving}>{saving ? t('new.creating') : t('new.submit')}</button></div>
    </form>
  </div>
}
