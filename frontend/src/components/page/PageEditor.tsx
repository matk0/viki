import { useEffect, useState, type FormEvent } from 'react'
import { Save } from 'lucide-react'
import { api, APIError } from '../../api/client'
import type { Page, Revision, RevisionContent, StepDefinition } from '../../api/types'
import { BDDStepsEditor } from './BDDStepsEditor'
import { MarkdownEditor } from './MarkdownEditor'
import { useI18n } from '../../i18n'

export function PageEditor({ page, revision, onCancel, onSaved }: { page: Page; revision: Revision; onCancel: () => void; onSaved: () => Promise<void> }) {
  const { t } = useI18n()
  const [content, setContent] = useState<RevisionContent>({ title: revision.title, bodyMd: revision.bodyMd, steps: revision.steps, references: revision.references })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [stepDefinitions, setStepDefinitions] = useState<StepDefinition[]>([])
  useEffect(() => {
    if (page.kind !== 'scenario') return
    void api.stepDefinitions().then(({ definitions }) => {
      setStepDefinitions(definitions)
    }).catch(() => undefined)
  }, [page.kind])
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
    <label>{t('new.title')}<input required value={content.title} onChange={(event) => update('title', event.target.value)} /></label>
    <label>{t('review.description')}<MarkdownEditor value={content.bodyMd} onChange={(value) => update('bodyMd', value)} /></label>
    {page.kind === 'scenario' && <BDDStepsEditor steps={content.steps} definitions={stepDefinitions} onChange={(steps) => update('steps', steps)} />}
    {error && <div className="form-error" role="alert">{error}</div>}
    <div className="editor-actions"><button type="button" className="secondary-button" onClick={onCancel}>{t('common.cancel')}</button><button className="primary-button" disabled={saving}><Save size={16} />{saving ? t('editor.saving') : t('editor.save')}</button></div>
  </form>
}
