import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Step } from '../../api/types'
import { BDDStepsEditor } from './BDDStepsEditor'

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

    expect(screen.getByRole('button', { name: 'Kľúčové slovo kroku 1' })).toHaveTextContent('Ak')
    expect(screen.getByRole('button', { name: 'Kľúčové slovo kroku 2' })).toHaveTextContent('Keď')
    expect(screen.getByRole('button', { name: 'Kľúčové slovo kroku 3' })).toHaveTextContent('Tak')

    await user.click(screen.getByRole('button', { name: 'Pridať krok' }))
    expect(onChange).toHaveBeenCalledWith([
      ...steps,
      expect.objectContaining({ keyword: 'and', text: '' }),
    ])
  })
})
