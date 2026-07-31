import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import { VikiSelect } from './VikiSelect'

function ExampleSelect() {
  const [value, setValue] = useState('noun')
  return <VikiSelect
    ariaLabel="Druh pojmu"
    listboxLabel="Druhy pojmov"
    value={value}
    onChange={setValue}
    options={[
      { value: 'noun', label: 'Podstatné meno' },
      { value: 'verb', label: 'Sloveso' },
    ]}
  />
}

describe('VikiSelect', () => {
  it('renders a styled accessible option panel instead of a native select', async () => {
    const user = userEvent.setup()
    const { container } = render(<ExampleSelect />)

    expect(container.querySelector('select')).toBeNull()
    const trigger = screen.getByRole('button', { name: 'Druh pojmu' })
    expect(trigger).toHaveTextContent('Podstatné meno')

    await user.click(trigger)
    expect(screen.getByRole('listbox', { name: 'Druhy pojmov' })).toBeVisible()
    expect(screen.getByRole('option', { name: 'Podstatné meno' })).toHaveAttribute('aria-selected', 'true')

    await user.click(screen.getByRole('option', { name: 'Sloveso' }))
    expect(trigger).toHaveTextContent('Sloveso')
    expect(screen.queryByRole('listbox', { name: 'Druhy pojmov' })).not.toBeInTheDocument()
  })

  it('supports keyboard selection and returns focus to the trigger', async () => {
    const user = userEvent.setup()
    render(<ExampleSelect />)
    const trigger = screen.getByRole('button', { name: 'Druh pojmu' })

    trigger.focus()
    await user.keyboard('{ArrowDown}{ArrowDown}{Enter}')

    expect(trigger).toHaveTextContent('Sloveso')
    expect(trigger).toHaveFocus()
  })
})
