import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { VikiSelect } from './VikiSelect'

function ExampleSelect() {
  const [value, setValue] = useState('noun')
  return <VikiSelect
    ariaLabel="Druh konceptu"
    listboxLabel="Druhy konceptov"
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
    const trigger = screen.getByRole('button', { name: 'Druh konceptu' })
    expect(trigger).toHaveTextContent('Podstatné meno')

    await user.click(trigger)
    expect(screen.getByRole('listbox', { name: 'Druhy konceptov' })).toBeVisible()
    expect(screen.getByRole('option', { name: 'Podstatné meno' })).toHaveAttribute('aria-selected', 'true')

    await user.click(screen.getByRole('option', { name: 'Sloveso' }))
    expect(trigger).toHaveTextContent('Sloveso')
    expect(screen.queryByRole('listbox', { name: 'Druhy konceptov' })).not.toBeInTheDocument()
  })

  it('supports keyboard selection and returns focus to the trigger', async () => {
    const user = userEvent.setup()
    render(<ExampleSelect />)
    const trigger = screen.getByRole('button', { name: 'Druh konceptu' })

    trigger.focus()
    await user.keyboard('{ArrowDown}{ArrowDown}{Enter}')

    expect(trigger).toHaveTextContent('Sloveso')
    expect(trigger).toHaveFocus()
  })

  it('closes on outside pointer and Escape while keeping inside interactions open', async () => {
    const user = userEvent.setup()
    render(<><ExampleSelect /><button>Outside</button></>)
    const trigger = screen.getByRole('button', { name: 'Druh konceptu' })
    await user.click(trigger)
    fireEvent.pointerDown(screen.getByRole('listbox'))
    expect(screen.getByRole('listbox')).toBeVisible()
    fireEvent.pointerDown(screen.getByRole('button', { name: 'Outside' }))
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()

    await user.click(trigger)
    fireEvent.keyDown(document, { key: 'x' })
    expect(screen.getByRole('listbox')).toBeVisible()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()
  })

  it('navigates disabled options, Home, End, Space, Enter, and Tab', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<VikiSelect ariaLabel="Choice" value="a" onChange={onChange} options={[
      { value: 'a', label: 'Alpha' }, { value: 'b', label: 'Beta', disabled: true }, { value: 'c', label: 'Gamma' },
    ]} />)
    const trigger = screen.getByRole('button', { name: 'Choice' })
    await user.click(trigger)
    const alpha = screen.getByRole('option', { name: 'Alpha' })
    const beta = screen.getByRole('option', { name: 'Beta' })
    const gamma = screen.getByRole('option', { name: 'Gamma' })

    fireEvent.keyDown(alpha, { key: 'ArrowDown' })
    expect(gamma).toHaveFocus()
    fireEvent.keyDown(gamma, { key: 'ArrowUp' })
    expect(alpha).toHaveFocus()
    fireEvent.keyDown(alpha, { key: 'End' })
    expect(gamma).toHaveFocus()
    fireEvent.keyDown(gamma, { key: 'Home' })
    expect(alpha).toHaveFocus()
    fireEvent.keyDown(beta, { key: 'Enter' })
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.keyDown(alpha, { key: 'x' })
    fireEvent.keyDown(alpha, { key: 'Enter' })
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()

    await user.click(trigger)
    fireEvent.keyDown(screen.getByRole('option', { name: 'Gamma' }), { key: ' ' })
    expect(onChange).toHaveBeenCalledWith('c')
    await user.click(trigger)
    fireEvent.keyDown(screen.getByRole('option', { name: 'Alpha' }), { key: 'Tab' })
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('handles trigger arrows, unknown values, empty options, and becoming disabled', async () => {
    const onChange = vi.fn()
    const view = render(<VikiSelect ariaLabel="Unknown" value="missing" onChange={onChange} options={[{ value: 'a', label: 'Alpha' }]} />)
    const trigger = screen.getByRole('button', { name: 'Unknown' })
    expect(trigger.querySelector('.viki-select-value')).toHaveTextContent('')
    fireEvent.keyDown(trigger, { key: 'ArrowUp' })
    expect(screen.getByRole('listbox', { name: 'Unknown' })).toBeVisible()
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    fireEvent.keyDown(trigger, { key: 'Enter' })
    view.rerender(<VikiSelect ariaLabel="Unknown" value="missing" onChange={onChange} options={[{ value: 'a', label: 'Alpha' }]} disabled />)
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
    expect(trigger).toBeDisabled()

    view.rerender(<VikiSelect ariaLabel="Empty" value="" onChange={onChange} options={[]} />)
    fireEvent.keyDown(screen.getByRole('button', { name: 'Empty' }), { key: 'ArrowDown' })
    expect(screen.getByRole('listbox', { name: 'Empty' })).toBeVisible()
    expect(screen.queryByRole('option')).not.toBeInTheDocument()
  })
})
