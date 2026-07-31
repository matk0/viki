import { useState, type FormEvent } from 'react'
import { Plus, Save, Trash2, X } from 'lucide-react'
import { api, APIError } from '../../api/client'
import type { Page, Revision, RevisionContent } from '../../api/types'
import { useWorkspace } from '../../workspace'
import { VikiSelect } from '../VikiSelect'
import { BDDStepsEditor } from './BDDStepsEditor'
import { MarkdownEditor } from './MarkdownEditor'

export function PageEditor({ page, revision, onCancel, onSaved }: { page: Page; revision: Revision; onCancel: () => void; onSaved: () => Promise<void> }) {
  const { pages } = useWorkspace()
  const [content, setContent] = useState<RevisionContent>({ title: revision.title, bodyMd: revision.bodyMd, aliases: revision.aliases, steps: revision.steps, references: revision.references })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const update = <K extends keyof RevisionContent>(key: K, value: RevisionContent[K]) => setContent((current) => ({ ...current, [key]: value }))
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setSaving(true); setError('')
    try { await api.saveRevision(page.id, revision.id, content); await onSaved() }
    catch (reason) {
      if (reason instanceof APIError && reason.status === 409) setError('Medzitým vznikla novšia revízia. Obnovte stránku a zapracujte svoje zmeny znova.')
      else setError(reason instanceof Error ? reason.message : 'Koncept sa nepodarilo uložiť.')
    } finally { setSaving(false) }
  }
  return <form className="page-editor panel" onSubmit={(event) => void submit(event)}>
    <div className="editor-header"><div><span className="eyebrow">Nová nemenná revízia</span><h2>Upraviť obsah</h2></div><button type="button" className="icon-button" onClick={onCancel} aria-label="Zavrieť editor"><X size={18} /></button></div>
    <label>Názov<input required value={content.title} onChange={(event) => update('title', event.target.value)} /></label>
    <label>Opis<MarkdownEditor value={content.bodyMd} onChange={(value) => update('bodyMd', value)} /></label>
    {page.kind === 'subscenario' && <BDDStepsEditor steps={content.steps} onChange={(steps) => update('steps', steps)} />}
    <div className="reference-editor">
      <div className="field-heading"><span>Súvisiace stránky</span><small>Štruktúrované vzťahy</small></div>
      {content.references.map((reference, index) => <div className="reference-edit-row" key={`${reference.targetPageId}-${index}`}>
        <VikiSelect compact ariaLabel={`Cieľ vzťahu ${index + 1}`} listboxLabel={`Ciele vzťahu ${index + 1}`} value={reference.targetPageId} onChange={(value) => update('references', content.references.map((item, position) => position === index ? { ...item, targetPageId: value } : item))} options={pages.filter((item) => item.id !== page.id).map((item) => ({ value: item.id, label: item.title }))} />
        <input aria-label={`Typ vzťahu ${index + 1}`} value={reference.relation} onChange={(event) => update('references', content.references.map((item, position) => position === index ? { ...item, relation: event.target.value } : item))} placeholder="napr. používa" />
        <button type="button" className="icon-button danger" onClick={() => update('references', content.references.filter((_, position) => position !== index))} aria-label="Odstrániť vzťah"><Trash2 size={15} /></button>
      </div>)}
      <button type="button" className="text-button" disabled={pages.filter((item) => item.id !== page.id).length === 0} onClick={() => { const target = pages.find((item) => item.id !== page.id); if (target) update('references', [...content.references, { targetPageId: target.id, targetTitle: target.title, relation: 'uses' }]) }}><Plus size={15} />Pridať vzťah</button>
    </div>
    <label>Alias názvy<input value={content.aliases.join(', ')} onChange={(event) => update('aliases', event.target.value.split(',').map((item) => item.trim()).filter(Boolean))} placeholder="synonymum, iný názov" /></label>
    {error && <div className="form-error" role="alert">{error}</div>}
    <div className="editor-actions"><button type="button" className="secondary-button" onClick={onCancel}>Zrušiť</button><button className="primary-button" disabled={saving}><Save size={16} />{saving ? 'Ukladám…' : 'Uložiť novú revíziu'}</button></div>
  </form>
}
