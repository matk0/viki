import { useState } from 'react'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import type { BDDKeyword, Step, StepDefinition, StepRole } from '../../api/types'
import { VikiSelect } from '../VikiSelect'
import { useI18n } from '../../i18n'

export function BDDStepsEditor({ steps, definitions = [], onChange }: { steps: Step[]; definitions?: StepDefinition[]; onChange: (steps: Step[]) => void }) {
  const { t } = useI18n()
  const labels = bddLabels(t)
  const keywords = Object.entries(labels) as [BDDKeyword, string][]
  const update = (index: number, patch: Partial<Step>) => onChange(steps.map((step, position) => position === index ? { ...step, ...patch } : step))
  const updateKeyword = (index: number, keyword: BDDKeyword) => {
    const next = steps.map((step, position) => position === index ? { ...step, keyword } : step)
    const currentDefinition = definitions.find((definition) => definition.id === next[index].definitionId)
    if (currentDefinition && currentDefinition.role !== roleForStep(next, index)) {
      next[index] = { ...next[index], definitionId: undefined, expression: '', arguments: [], text: '' }
    }
    onChange(next)
  }
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
          <VikiSelect compact ariaLabel={t('bdd.keyword', { number: index + 1 })} listboxLabel={t('bdd.keywords', { number: index + 1 })} value={step.keyword} onChange={(value) => updateKeyword(index, value as BDDKeyword)} options={keywords.map(([value, label]) => ({ value, label }))} />
          <StepDefinitionField step={step} stepNumber={index + 1} role={roleForStep(steps, index)} definitions={definitions} onChange={(patch) => update(index, patch)} />
          <div className="row-actions">
            <button type="button" className="icon-button" onClick={() => move(index, -1)} disabled={index === 0} aria-label={t('bdd.moveUp')}><ArrowUp size={15} /></button>
            <button type="button" className="icon-button" onClick={() => move(index, 1)} disabled={index === steps.length - 1} aria-label={t('bdd.moveDown')}><ArrowDown size={15} /></button>
            <button type="button" className="icon-button danger" onClick={() => onChange(steps.filter((_, position) => position !== index))} aria-label={t('bdd.remove')}><Trash2 size={15} /></button>
          </div>
        </div>
      ))}
      <button type="button" className="text-button" onClick={() => onChange([...steps, { stableId: crypto.randomUUID(), keyword: 'and', text: '', arguments: [] }])}><Plus size={15} />{t('bdd.add')}</button>
    </div>
  )
}

function StepDefinitionField({ step, stepNumber, role, definitions, onChange }: {
  step: Step
  stepNumber: number
  role: StepRole
  definitions: StepDefinition[]
  onChange: (patch: Partial<Step>) => void
}) {
  const { t } = useI18n()
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)
  const [changing, setChanging] = useState(false)
  const [proposing, setProposing] = useState(false)
  const selected = definitions.find((definition) => definition.id === step.definitionId)
  const compatible = definitions.filter((definition) => definition.role === role && normalize(definition.expression).includes(normalize(search)))
  const expression = selected?.expression ?? step.expression ?? ''
  const parameters = expression.match(/\{(?:string|int|word)\}/g) ?? []

  if (selected && !changing) {
    return <div className="step-definition-field selected">
      <div className="step-definition-summary">
        <span>{selected.expression}</span>
        <small>{t(selected.usageCount === 1 ? 'bdd.used.one' : 'bdd.used.other', { count: selected.usageCount })}</small>
      </div>
      <button type="button" className="text-button step-definition-change" onClick={() => setChanging(true)}>{t('bdd.changeDefinition')}</button>
      {parameters.length > 0 && <div className="step-arguments">{parameters.map((parameter, index) => <input
        key={`${parameter}-${index}`}
        aria-label={t('bdd.parameter', { parameter: index + 1, number: stepNumber })}
        value={step.arguments?.[index] ?? ''}
        onChange={(event) => {
          const next = [...(step.arguments ?? [])]
          next[index] = event.target.value
          onChange({ arguments: next })
        }}
      />)}</div>}
    </div>
  }

  if (proposing || (!selected && Boolean(step.expression || step.text))) {
    return <div className="step-definition-field proposing">
      <input required aria-label={t('bdd.newDefinition', { number: stepNumber })} value={step.expression ?? step.text} onChange={(event) => onChange({ definitionId: undefined, expression: event.target.value, arguments: [], text: event.target.value })} placeholder={t('bdd.placeholder')} />
      <button type="button" className="text-button" onClick={() => { setProposing(false); setChanging(false); onChange({ definitionId: undefined, expression: '', text: '', arguments: [] }) }}>{t('bdd.useExisting')}</button>
      {parameters.length > 0 && <div className="step-arguments">{parameters.map((parameter, index) => <input
        key={`${parameter}-${index}`}
        aria-label={t('bdd.parameter', { parameter: index + 1, number: stepNumber })}
        value={step.arguments?.[index] ?? ''}
        onChange={(event) => {
          const next = [...(step.arguments ?? [])]
          next[index] = event.target.value
          onChange({ arguments: next })
        }}
      />)}</div>}
    </div>
  }

  return <div className="step-definition-field choosing">
    <input role="combobox" aria-expanded={open} aria-controls={`step-definitions-${stepNumber}`} aria-label={t('bdd.searchDefinition', { number: stepNumber })} value={search} onFocus={() => setOpen(true)} onChange={(event) => { setSearch(event.target.value); setOpen(true) }} placeholder={t('bdd.searchPlaceholder')} />
    {open && <div className="step-definition-options" id={`step-definitions-${stepNumber}`} role="listbox">
      {compatible.map((definition) => <button key={definition.id} type="button" role="option" aria-selected="false" onClick={() => {
        setChanging(false)
        setOpen(false)
        onChange({ definitionId: definition.id, expression: definition.expression, arguments: new Array((definition.expression.match(/\{(?:string|int|word)\}/g) ?? []).length).fill(''), text: definition.expression })
      }}>
        <span>{definition.expression}</span>
        <small>{t(definition.usageCount === 1 ? 'bdd.used.one' : 'bdd.used.other', { count: definition.usageCount })}</small>
      </button>)}
      {compatible.length === 0 && <small className="step-definition-empty">{t('bdd.noDefinitions')}</small>}
    </div>}
    <button type="button" className="text-button step-definition-propose" onClick={() => setProposing(true)}>{t('bdd.proposeDefinition')}</button>
  </div>
}

function roleForStep(steps: Step[], position: number): StepRole {
  for (let index = position; index >= 0; index -= 1) {
    if (steps[index].keyword === 'given') return 'context'
    if (steps[index].keyword === 'when') return 'action'
    if (steps[index].keyword === 'then') return 'outcome'
  }
  return 'context'
}

function normalize(value: string): string {
  return value.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLocaleLowerCase()
}

export function BDDSteps({ steps }: { steps: Step[] }) {
  const { t } = useI18n()
  const labels = bddLabels(t)
  return <div className="bdd-steps">{steps.map((step, index) => <div key={step.stableId ?? step.id ?? index}><strong>{labels[step.keyword]}</strong><span>{step.text}</span></div>)}</div>
}

function bddLabels(t: ReturnType<typeof useI18n>['t']): Record<BDDKeyword, string> {
  return { given: t('bdd.given'), when: t('bdd.when'), then: t('bdd.then'), and: t('bdd.and'), but: t('bdd.but') }
}
