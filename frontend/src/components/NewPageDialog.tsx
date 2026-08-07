import { useEffect, useState, type FormEvent } from 'react'
import { X } from 'lucide-react'
import { api } from '../api/client'
import type { PageKind, RevisionContent, Step, StepDefinition } from '../api/types'
import { useRouter } from '../router'
import { useWorkspace } from '../workspace'
import { VikiSelect } from './VikiSelect'
import { useI18n } from '../i18n'
import { Modal } from './Modal'
import { BDDStepsEditor } from './page/BDDStepsEditor'

function slugify(value: string) {
  return value.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
}

export function NewPageDialog({ initialKind, parentId, onClose }: { initialKind: PageKind; parentId?: string; onClose: () => void }) {
  const { t } = useI18n()
  const { reloadPages } = useWorkspace()
  const { navigate } = useRouter()
  const kind = initialKind
  const [conceptKind, setConceptKind] = useState<'noun' | 'verb'>('noun')
  const [title, setTitle] = useState('')
  const [steps, setSteps] = useState<Step[]>([
    { keyword: 'given', text: '', arguments: [] },
    { keyword: 'when', text: '', arguments: [] },
    { keyword: 'then', text: '', arguments: [] },
  ])
  const [stepDefinitions, setStepDefinitions] = useState<StepDefinition[]>([])
  const [scenarioTitle, setScenarioTitle] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (kind === 'concept') return
    void api.stepDefinitions().then(({ definitions }) => {
      setStepDefinitions(definitions)
    }).catch(() => undefined)
  }, [kind])
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    setSaving(true)
    const content: RevisionContent = {
      title: title.trim(), bodyMd: '', references: [],
      steps: kind === 'scenario' ? steps : [],
    }
    const initialScenario = kind === 'feature' ? {
      slug: slugify(scenarioTitle),
      content: {
        title: scenarioTitle.trim(), bodyMd: '', references: [],
        steps,
      },
    } : undefined
    try {
      const result = await api.createPage({ kind, ...(kind === 'concept' ? { conceptKind } : {}), ...(kind === 'scenario' ? { parentId } : {}), ...(initialScenario ? { initialScenario } : {}), slug: slugify(title), content })
      await reloadPages(); onClose(); navigate(`/page/${result.page.id}`)
    } catch (reason) { setError(reason instanceof Error ? reason.message : t('new.failed')) }
    finally { setSaving(false) }
  }
  return <Modal onClose={onClose}>
    <form className="modal-card new-page-dialog" role="dialog" aria-modal="true" aria-labelledby="new-page-dialog-title" onSubmit={(event) => void submit(event)}>
      <div className="modal-heading"><div><span className="eyebrow">{t('new.eyebrow')}</span><h2 id="new-page-dialog-title">{kind === 'concept' ? t('new.createConcept') : kind === 'feature' ? t('new.createFeature') : t('new.createScenario')}</h2></div><button type="button" className="icon-button" onClick={onClose} aria-label={t('common.close')}><X size={18} /></button></div>
      {kind === 'concept' && <div className="select-field"><span>{t('new.conceptKind')}</span><VikiSelect ariaLabel={t('new.conceptKind')} listboxLabel={t('new.conceptKinds')} value={conceptKind} onChange={(value) => setConceptKind(value as 'noun' | 'verb')} options={[{ value: 'noun', label: t('new.noun') }, { value: 'verb', label: t('new.verb') }]} /></div>}
      <label>{t('new.title')}<input autoFocus required value={title} onChange={(event) => setTitle(event.target.value)} placeholder={t('new.titlePlaceholder')} /></label>
      {kind === 'feature' && <div className="scenario-step-fields initial-scenario-fields">
        <h3>{t('new.initialScenario')}</h3>
        <label>{t('new.scenarioTitle')}<input required value={scenarioTitle} onChange={(event) => setScenarioTitle(event.target.value)} /></label>
        <BDDStepsEditor steps={steps} definitions={stepDefinitions} onChange={setSteps} />
      </div>}
      {kind === 'scenario' && <div className="scenario-step-fields">
        <BDDStepsEditor steps={steps} definitions={stepDefinitions} onChange={setSteps} />
      </div>}
      {error && <div className="form-error" role="alert">{error}</div>}
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>{t('common.cancel')}</button><button className="primary-button" disabled={saving}>{saving ? t('new.creating') : t('new.submit')}</button></div>
    </form>
  </Modal>
}
