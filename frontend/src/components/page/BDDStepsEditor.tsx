import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import type { BDDKeyword, Step } from '../../api/types'
import { VikiSelect } from '../VikiSelect'

const labels: Record<BDDKeyword, string> = {
  given: 'Ak',
  when: 'Keď',
  then: 'Tak',
  and: 'A',
  but: 'Ale',
}

const keywords = Object.entries(labels) as [BDDKeyword, string][]

export function BDDStepsEditor({ steps, onChange }: { steps: Step[]; onChange: (steps: Step[]) => void }) {
  const update = (index: number, patch: Partial<Step>) => onChange(steps.map((step, position) => position === index ? { ...step, ...patch } : step))
  const move = (index: number, delta: number) => {
    const next = [...steps]
    const target = index + delta
    if (target < 0 || target >= next.length) return
    ;[next[index], next[target]] = [next[target], next[index]]
    onChange(next)
  }
  return (
    <div className="bdd-editor">
      <div className="field-heading"><span>Kroky správania</span><small>Štruktúrované BDD</small></div>
      {steps.map((step, index) => (
        <div className="bdd-edit-row" key={step.stableId ?? step.id ?? index}>
          <VikiSelect compact ariaLabel={`Kľúčové slovo kroku ${index + 1}`} listboxLabel={`Kľúčové slová kroku ${index + 1}`} value={step.keyword} onChange={(value) => update(index, { keyword: value as BDDKeyword })} options={keywords.map(([value, label]) => ({ value, label }))} />
          <input aria-label={`Text kroku ${index + 1}`} value={step.text} onChange={(event) => update(index, { text: event.target.value })} placeholder="Popíšte podmienku alebo výsledok…" />
          <div className="row-actions">
            <button type="button" className="icon-button" onClick={() => move(index, -1)} disabled={index === 0} aria-label="Posunúť krok hore"><ArrowUp size={15} /></button>
            <button type="button" className="icon-button" onClick={() => move(index, 1)} disabled={index === steps.length - 1} aria-label="Posunúť krok dole"><ArrowDown size={15} /></button>
            <button type="button" className="icon-button danger" onClick={() => onChange(steps.filter((_, position) => position !== index))} aria-label="Odstrániť krok"><Trash2 size={15} /></button>
          </div>
        </div>
      ))}
      <button type="button" className="text-button" onClick={() => onChange([...steps, { stableId: crypto.randomUUID(), keyword: 'and', text: '' }])}><Plus size={15} />Pridať krok</button>
    </div>
  )
}

export function BDDSteps({ steps }: { steps: Step[] }) {
  return <div className="bdd-steps">{steps.map((step, index) => <div key={step.stableId ?? step.id ?? index}><strong>{labels[step.keyword]}</strong><span>{step.text}</span></div>)}</div>
}
