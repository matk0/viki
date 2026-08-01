import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Step } from '../../api/types'
import { BDDSteps, BDDStepsEditor } from './BDDStepsEditor'

describe('BDDStepsEditor', () => {
  it('renders structured Given/When/Then steps and adds an And step', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const steps: Step[] = [
      { stableId: 'given-1', keyword: 'given', text: 'zákazník zadal adresu' },
      { stableId: 'when-1', keyword: 'when', text: 'overí dostupnosť' },
      { stableId: 'then-1', keyword: 'then', text: 'systém zobrazí balíky' },
    ]

    render(<BDDStepsEditor steps={steps} onChange={onChange} />)

    expect(screen.getByRole('button', { name: 'Kľúčové slovo kroku 1' })).toHaveTextContent('Za predpokladu')
    expect(screen.getByRole('button', { name: 'Kľúčové slovo kroku 2' })).toHaveTextContent('Keď')
    expect(screen.getByRole('button', { name: 'Kľúčové slovo kroku 3' })).toHaveTextContent('Tak')

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

    fireEvent.change(screen.getByRole('textbox', { name: 'Text kroku 1' }), { target: { value: 'first updated' } })
    expect(onChange).toHaveBeenCalledWith(expect.arrayContaining([expect.objectContaining({ text: 'first updated' })]))
    await user.click(screen.getAllByRole('button', { name: 'Posunúť krok dole' })[0])
    expect(onChange).toHaveBeenCalledWith([steps[1], steps[0], steps[2]])
    await user.click(screen.getAllByRole('button', { name: 'Posunúť krok hore' })[1])
    expect(onChange).toHaveBeenCalledWith([steps[1], steps[0], steps[2]])
    await user.click(screen.getAllByRole('button', { name: 'Odstrániť krok' })[1])
    expect(onChange).toHaveBeenCalledWith([steps[0], steps[2]])

    await user.click(screen.getByRole('button', { name: 'Kľúčové slovo kroku 2' }))
    await user.click(screen.getByRole('option', { name: 'A' }))
    expect(onChange).toHaveBeenCalledWith(expect.arrayContaining([expect.objectContaining({ keyword: 'and' })]))
    expect(screen.getByText('Ale')).toBeInTheDocument()
    expect(screen.getByText('exception')).toBeInTheDocument()
  })
})
