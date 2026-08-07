import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Step, StepDefinition } from '../../api/types'
import { I18nProvider, LanguageSwitcher } from '../../i18n'
import { BDDSteps, BDDStepsEditor } from './BDDStepsEditor'

describe('BDDStepsEditor', () => {
  const definitions: StepDefinition[] = [
    { id: 'definition-context', expression: 'zákazník má aktívnu zmluvu typu {string}', role: 'context', approved: true, usageCount: 4 },
    { id: 'definition-action', expression: 'zákazník podpíše zmluvu', role: 'action', approved: true, usageCount: 1 },
    { id: 'definition-outcome', expression: 'systém uloží podpis', role: 'outcome', approved: true, usageCount: 3 },
  ]

  it('locks an existing definition while keeping its parameters editable', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const steps: Step[] = [{
      stableId: 'given-1', keyword: 'given', definitionId: 'definition-context',
      expression: definitions[0].expression, arguments: ['internet'], text: 'zákazník má aktívnu zmluvu typu "internet"',
    }]

    render(<BDDStepsEditor steps={steps} definitions={definitions} onChange={onChange} />)

    expect(screen.queryByRole('textbox', { name: 'Text kroku 1' })).not.toBeInTheDocument()
    expect(screen.getByText(definitions[0].expression)).toBeInTheDocument()
    expect(screen.getByText('Použité v 4 scenároch')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('textbox', { name: 'Parameter 1 kroku 1' }), { target: { value: 'televízia' } })
    expect(onChange).toHaveBeenLastCalledWith([expect.objectContaining({ arguments: ['televízia'] })])
  })

  it('offers compatible approved definitions before a new proposal', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const steps: Step[] = [{ stableId: 'when-1', keyword: 'when', text: '', arguments: [] }]

    render(<BDDStepsEditor steps={steps} definitions={definitions} onChange={onChange} />)

    await user.type(screen.getByRole('combobox', { name: 'Vyhľadať definíciu kroku 1' }), 'podpise')
    expect(screen.queryByText(definitions[0].expression)).not.toBeInTheDocument()
    await user.click(screen.getByRole('option', { name: /zákazník podpíše zmluvu/ }))
    expect(onChange).toHaveBeenCalledWith([expect.objectContaining({
      definitionId: 'definition-action', expression: 'zákazník podpíše zmluvu', arguments: [],
    })])

    await user.click(screen.getByRole('button', { name: 'Navrhnúť nový krok' }))
    expect(screen.getByRole('textbox', { name: 'Nová definícia kroku 1' })).toBeInTheDocument()
  })

  it('clears an incompatible definition when the Gherkin phase changes', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<BDDStepsEditor definitions={definitions} steps={[{
      keyword: 'given', definitionId: 'definition-context', expression: definitions[0].expression,
      arguments: ['internet'], text: 'zákazník má aktívnu zmluvu typu "internet"',
    }]} onChange={onChange} />)

    await user.click(screen.getByRole('button', { name: 'Kľúčové slovo kroku 1' }))
    await user.click(screen.getByRole('option', { name: 'Keď' }))
    expect(onChange).toHaveBeenCalledWith([expect.objectContaining({ keyword: 'when', definitionId: undefined, expression: '', arguments: [], text: '' })])
  })

  it('can replace a selected definition or return a proposal to the catalog', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const selected = [{ keyword: 'when' as const, definitionId: 'definition-action', expression: definitions[1].expression, text: definitions[1].expression }]
    const view = render(<BDDStepsEditor definitions={definitions} steps={selected} onChange={onChange} />)

    expect(screen.getByText('Použité v 1 scenári')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Zmeniť' }))
    expect(screen.getByRole('combobox', { name: 'Vyhľadať definíciu kroku 1' })).toBeInTheDocument()

    view.rerender(<BDDStepsEditor definitions={definitions} steps={[{ keyword: 'when', expression: 'zákazník vyberie {word}', arguments: [], text: 'zákazník vyberie {word}' }]} onChange={onChange} />)
    fireEvent.change(screen.getByRole('textbox', { name: 'Parameter 1 kroku 1' }), { target: { value: 'balík' } })
    expect(onChange).toHaveBeenCalledWith([expect.objectContaining({ arguments: ['balík'] })])
    await user.click(screen.getByRole('button', { name: 'Použiť existujúci krok' }))
    expect(onChange).toHaveBeenCalledWith([expect.objectContaining({ definitionId: undefined, expression: '', arguments: [], text: '' })])
  })

  it('inherits context for an initial And step and reports no matching definition', async () => {
    const user = userEvent.setup()
    render(<BDDStepsEditor definitions={definitions} steps={[{ keyword: 'and', text: '', arguments: [] }]} onChange={vi.fn()} />)

    const search = screen.getByRole('combobox', { name: 'Vyhľadať definíciu kroku 1' })
    await user.click(search)
    expect(screen.getByRole('option', { name: /zákazník má aktívnu zmluvu/ })).toBeInTheDocument()
    await user.type(search, 'nič také')
    expect(screen.getByText('Žiadny zodpovedajúci krok')).toBeInTheDocument()
  })

  it('selects a parameterized definition and displays singular usage', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const parameterized: StepDefinition = { id: 'definition-action-parameter', expression: 'zákazník vyberie {word}', role: 'action', approved: true, usageCount: 1 }
    render(<BDDStepsEditor definitions={[parameterized]} steps={[{ keyword: 'when', text: '', arguments: [] }]} onChange={onChange} />)

    await user.click(screen.getByRole('combobox', { name: 'Vyhľadať definíciu kroku 1' }))
    expect(screen.getByText('Použité v 1 scenári')).toBeInTheDocument()
    await user.click(screen.getByRole('option', { name: /zákazník vyberie/ }))
    expect(onChange).toHaveBeenCalledWith([expect.objectContaining({ definitionId: parameterized.id, arguments: [''] })])
  })

  it('initializes missing parameter arrays for selected and proposed definitions', () => {
    const selectedChange = vi.fn()
    const selected = render(<BDDStepsEditor definitions={definitions} steps={[{
      keyword: 'given', definitionId: 'definition-context', expression: definitions[0].expression, text: definitions[0].expression,
    }]} onChange={selectedChange} />)
    const selectedParameter = screen.getByRole('textbox', { name: 'Parameter 1 kroku 1' })
    expect(selectedParameter).toHaveValue('')
    fireEvent.change(selectedParameter, { target: { value: 'internet' } })
    expect(selectedChange).toHaveBeenCalledWith([expect.objectContaining({ arguments: ['internet'] })])

    selected.rerender(<BDDStepsEditor definitions={definitions} steps={[{
      keyword: 'given', definitionId: 'definition-context', expression: definitions[0].expression, arguments: [], text: definitions[0].expression,
    }]} onChange={selectedChange} />)
    expect(screen.getByRole('textbox', { name: 'Parameter 1 kroku 1' })).toHaveValue('')

    selected.unmount()
    const proposedChange = vi.fn()
    render(<BDDStepsEditor steps={[{ keyword: 'given', expression: 'zákazník má {word}', text: 'zákazník má {word}' }]} onChange={proposedChange} />)
    fireEvent.change(screen.getByRole('textbox', { name: 'Parameter 1 kroku 1' }), { target: { value: 'zmluvu' } })
    expect(proposedChange).toHaveBeenCalledWith([expect.objectContaining({ arguments: ['zmluvu'] })])
  })

  it('renders structured Given/When/Then steps and adds an And step', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const steps: Step[] = [
      { stableId: 'given-1', keyword: 'given', text: 'zákazník zadal adresu' },
      { stableId: 'when-1', keyword: 'when', text: 'overí dostupnosť' },
      { stableId: 'then-1', keyword: 'then', text: 'systém zobrazí balíky' },
    ]

    render(<BDDStepsEditor steps={steps} onChange={onChange} />)

    expect(screen.getByRole('button', { name: 'Kľúčové slovo kroku 1' })).toHaveTextContent('Pokiaľ')
    expect(screen.getByRole('button', { name: 'Kľúčové slovo kroku 2' })).toHaveTextContent('Keď')
    expect(screen.getByRole('button', { name: 'Kľúčové slovo kroku 3' })).toHaveTextContent(/^Potom$/)

    await user.click(screen.getByRole('button', { name: 'Pridať krok' }))
    expect(onChange).toHaveBeenCalledWith([
      ...steps,
      expect.objectContaining({ keyword: 'and', text: '' }),
    ])
  })

  it('updates, reorders, removes, and renders behavior steps', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const steps: Step[] = [
      { stableId: 'given-1', keyword: 'given', text: 'first' },
      { id: 'when-1', keyword: 'when', text: 'second' },
      { keyword: 'then', text: 'third' },
    ]
    render(<><BDDStepsEditor steps={steps} onChange={onChange} /><BDDSteps steps={[...steps, { keyword: 'but', text: 'exception' }]} /></>)

    expect(screen.getAllByRole('button', { name: 'Posunúť krok hore' })[0]).toBeDisabled()
    expect(screen.getAllByRole('button', { name: 'Posunúť krok dole' })[2]).toBeDisabled()

    fireEvent.change(screen.getByRole('textbox', { name: 'Nová definícia kroku 1' }), { target: { value: 'first updated' } })
    expect(onChange).toHaveBeenCalledWith(expect.arrayContaining([expect.objectContaining({ expression: 'first updated', text: 'first updated' })]))
    await user.click(screen.getAllByRole('button', { name: 'Posunúť krok dole' })[0])
    expect(onChange).toHaveBeenCalledWith([steps[1], steps[0], steps[2]])
    await user.click(screen.getAllByRole('button', { name: 'Posunúť krok hore' })[1])
    expect(onChange).toHaveBeenCalledWith([steps[1], steps[0], steps[2]])
    await user.click(screen.getAllByRole('button', { name: 'Odstrániť krok' })[1])
    expect(onChange).toHaveBeenCalledWith([steps[0], steps[2]])

    await user.click(screen.getByRole('button', { name: 'Kľúčové slovo kroku 2' }))
    await user.click(screen.getByRole('option', { name: 'A zároveň' }))
    expect(onChange).toHaveBeenCalledWith(expect.arrayContaining([expect.objectContaining({ keyword: 'and' })]))
    expect(screen.getByText('Ale')).toBeInTheDocument()
    expect(screen.getByText('exception')).toBeInTheDocument()
  })

  it('switches every Gherkin step selector between Slovak and English', async () => {
    const user = userEvent.setup()
    const steps: Step[] = [
      { keyword: 'given', text: '' },
      { keyword: 'when', text: '' },
      { keyword: 'then', text: '' },
      { keyword: 'and', text: '' },
      { keyword: 'but', text: '' },
    ]

    render(<I18nProvider><LanguageSwitcher /><BDDStepsEditor steps={steps} onChange={vi.fn()} /></I18nProvider>)

    expect(screen.getAllByLabelText(/Kľúčové slovo kroku/).map((element) => element.textContent?.trim())).toEqual(['Pokiaľ', 'Keď', 'Potom', 'A zároveň', 'Ale'])
    await user.click(screen.getByRole('switch', { name: 'Jazyk' }))
    expect(screen.getAllByLabelText(/^Step \d keyword$/).map((element) => element.textContent?.trim())).toEqual(['Given', 'When', 'Then', 'And', 'But'])
  })
})
