import { useState, type FormEvent } from 'react'
import { Plus, Save, Trash2, X } from 'lucide-react'
import { api, APIError } from '../../api/client'
import type { Page, Revision, RevisionContent } from '../../api/types'
import { useWorkspace } from '../../workspace'
import { VikiSelect } from '../VikiSelect'
import { BDDStepsEditor } from './BDDStepsEditor'
import { MarkdownEditor } from './MarkdownEditor'
import { useI18n } from '../../i18n'

export function PageEditor({ page, revision, onCancel, onSaved }: { page: Page; revision: Revision; onCancel: () => void; onSaved: () => Promise<void> }) {
  const { t } = useI18n()
  const { pages } = useWorkspace()
  const [content, setContent] = useState<RevisionContent>({ title: revision.title, bodyMd: revision.bodyMd, aliases: revision.aliases, steps: revision.steps, references: revision.references })
  const [aliasesText, setAliasesText] = useState(revision.aliases.join(', '))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const update = <K extends keyof RevisionContent>(key: K, value: RevisionContent[K]) => setContent((current) => ({ ...current, [key]: value }))
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setSaving(true); setError('')
    try { await api.saveRevision(page.id, revision.id, content); await onSaved() }
    catch (reason) {
      if (reason instanceof APIError && reason.status === 409) setError(t('editor.conflict'))
      else setError(reason instanceof Error ? reason.message : t('editor.failed'))
    } finally { setSaving(false) }
  }
  return <form className="page-editor panel" onSubmit={(event) => void submit(event)}>
    <div className="editor-header"><div><span className="eyebrow">{t('editor.eyebrow')}</span><h2>{t('editor.title')}</h2></div><button type="button" className="icon-button" onClick={onCancel} aria-label={t('editor.close')}><X size={18} /></button></div>
    <label>{t('new.title')}<input required value={content.title} onChange={(event) => update('title', event.target.value)} /></label>
    <label>{t('review.description')}<MarkdownEditor value={content.bodyMd} onChange={(value) => update('bodyMd', value)} /></label>
    {page.kind === 'scenario' && <BDDStepsEditor steps={content.steps} onChange={(steps) => update('steps', steps)} />}
    <div className="reference-editor">
      <div className="field-heading"><span>{t('editor.related')}</span><small>{t('editor.structuredRelations')}</small></div>
      {content.references.map((reference, index) => <div className="reference-edit-row" key={`${reference.targetPageId}-${index}`}>
        <VikiSelect compact ariaLabel={t('editor.relationTarget', { number: index + 1 })} listboxLabel={t('editor.relationTargets', { number: index + 1 })} value={reference.targetPageId} onChange={(value) => update('references', content.references.map((item, position) => position === index ? { ...item, targetPageId: value } : item))} options={pages.filter((item) => item.id !== page.id).map((item) => ({ value: item.id, label: item.title }))} />
        <input aria-label={t('editor.relationType', { number: index + 1 })} value={reference.relation} onChange={(event) => update('references', content.references.map((item, position) => position === index ? { ...item, relation: event.target.value } : item))} placeholder={t('editor.relationPlaceholder')} />
        <button type="button" className="icon-button danger" onClick={() => update('references', content.references.filter((_, position) => position !== index))} aria-label={t('editor.removeRelation')}><Trash2 size={15} /></button>
      </div>)}
      <button type="button" className="text-button" disabled={pages.filter((item) => item.id !== page.id).length === 0} onClick={() => { const target = pages.find((item) => item.id !== page.id)!; update('references', [...content.references, { targetPageId: target.id, targetTitle: target.title, relation: 'uses' }]) }}><Plus size={15} />{t('editor.addRelation')}</button>
    </div>
    <label>{t('editor.aliases')}<input value={aliasesText} onChange={(event) => { setAliasesText(event.target.value); update('aliases', event.target.value.split(',').map((item) => item.trim()).filter(Boolean)) }} placeholder={t('editor.aliasesPlaceholder')} /></label>
    {error && <div className="form-error" role="alert">{error}</div>}
    <div className="editor-actions"><button type="button" className="secondary-button" onClick={onCancel}>{t('common.cancel')}</button><button className="primary-button" disabled={saving}><Save size={16} />{saving ? t('editor.saving') : t('editor.save')}</button></div>
  </form>
}
