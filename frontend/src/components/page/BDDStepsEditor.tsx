import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import type { BDDKeyword, Step } from '../../api/types'
import { VikiSelect } from '../VikiSelect'
import { useI18n } from '../../i18n'

export function BDDStepsEditor({ steps, onChange }: { steps: Step[]; onChange: (steps: Step[]) => void }) {
  const { t } = useI18n()
  const labels = bddLabels(t)
  const keywords = Object.entries(labels) as [BDDKeyword, string][]
  const update = (index: number, patch: Partial<Step>) => onChange(steps.map((step, position) => position === index ? { ...step, ...patch } : step))
  const move = (index: number, delta: number) => {
    const next = [...steps]
    const target = index + delta
    ;[next[index], next[target]] = [next[target], next[index]]
    onChange(next)
  }
  return (
    <div className="bdd-editor">
      <div className="field-heading"><span>{t('bdd.heading')}</span><small>{t('bdd.structured')}</small></div>
      {steps.map((step, index) => (
        <div className="bdd-edit-row" key={step.stableId ?? step.id ?? index}>
          <VikiSelect compact ariaLabel={t('bdd.keyword', { number: index + 1 })} listboxLabel={t('bdd.keywords', { number: index + 1 })} value={step.keyword} onChange={(value) => update(index, { keyword: value as BDDKeyword })} options={keywords.map(([value, label]) => ({ value, label }))} />
          <input aria-label={t('bdd.text', { number: index + 1 })} value={step.text} onChange={(event) => update(index, { text: event.target.value })} placeholder={t('bdd.placeholder')} />
          <div className="row-actions">
            <button type="button" className="icon-button" onClick={() => move(index, -1)} disabled={index === 0} aria-label={t('bdd.moveUp')}><ArrowUp size={15} /></button>
            <button type="button" className="icon-button" onClick={() => move(index, 1)} disabled={index === steps.length - 1} aria-label={t('bdd.moveDown')}><ArrowDown size={15} /></button>
            <button type="button" className="icon-button danger" onClick={() => onChange(steps.filter((_, position) => position !== index))} aria-label={t('bdd.remove')}><Trash2 size={15} /></button>
          </div>
        </div>
      ))}
      <button type="button" className="text-button" onClick={() => onChange([...steps, { stableId: crypto.randomUUID(), keyword: 'and', text: '' }])}><Plus size={15} />{t('bdd.add')}</button>
    </div>
  )
}

export function BDDSteps({ steps }: { steps: Step[] }) {
  const { t } = useI18n()
  const labels = bddLabels(t)
  return <div className="bdd-steps">{steps.map((step, index) => <div key={step.stableId ?? step.id ?? index}><strong>{labels[step.keyword]}</strong><span>{step.text}</span></div>)}</div>
}

function bddLabels(t: ReturnType<typeof useI18n>['t']): Record<BDDKeyword, string> {
  return { given: t('bdd.given'), when: t('bdd.when'), then: t('bdd.then'), and: t('bdd.and'), but: t('bdd.but') }
}
