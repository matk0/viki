import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { Modal } from './Modal'

afterEach(() => {
  document.body.classList.remove('modal-open')
})

it('renders above the application, locks scrolling, and closes only from modal-level actions', () => {
  const onClose = vi.fn()
  const view = render(<Modal className="history-backdrop" onClose={onClose}>
    <div role="dialog" aria-label="Test modal"><button>Inside</button></div>
  </Modal>)

  const backdrop = document.body.querySelector<HTMLElement>('.modal-backdrop')!
  expect(backdrop).toHaveClass('history-backdrop')
  expect(view.container.querySelector('.modal-backdrop')).not.toBeInTheDocument()
  expect(document.body).toHaveClass('modal-open')

  fireEvent.mouseDown(screen.getByRole('button', { name: 'Inside' }))
  expect(onClose).not.toHaveBeenCalled()
  fireEvent.mouseDown(backdrop)
  expect(onClose).toHaveBeenCalledOnce()
  fireEvent.keyDown(document, { key: 'Enter' })
  expect(onClose).toHaveBeenCalledOnce()
  fireEvent.keyDown(document, { key: 'Escape' })
  expect(onClose).toHaveBeenCalledTimes(2)

  view.unmount()
  expect(document.body).not.toHaveClass('modal-open')
  fireEvent.keyDown(document, { key: 'Escape' })
  expect(onClose).toHaveBeenCalledTimes(2)
})

it('omits an empty modifier class', () => {
  const view = render(<Modal onClose={vi.fn()}><div role="dialog" aria-label="Plain modal" /></Modal>)

  expect(document.body.querySelector('.modal-backdrop')).toHaveClass('modal-backdrop')
  fireEvent.keyDown(document, { key: 'Tab' })
  view.unmount()
})

it('contains keyboard focus and returns it to the opening control', () => {
  const trigger = document.createElement('button')
  document.body.append(trigger)
  trigger.focus()
  const view = render(<Modal onClose={vi.fn()}>
    <div role="dialog" aria-label="Focus modal">
      <button>First</button>
      <button>Middle</button>
      <button>Last</button>
    </div>
  </Modal>)

  const first = screen.getByRole('button', { name: 'First' })
  const middle = screen.getByRole('button', { name: 'Middle' })
  const last = screen.getByRole('button', { name: 'Last' })
  expect(first).toHaveFocus()

  fireEvent.keyDown(document, { key: 'Tab', shiftKey: true })
  expect(last).toHaveFocus()
  fireEvent.keyDown(document, { key: 'Tab' })
  expect(first).toHaveFocus()
  middle.focus()
  fireEvent.keyDown(document, { key: 'Tab' })
  expect(middle).toHaveFocus()

  view.unmount()
  expect(trigger).toHaveFocus()
  trigger.remove()
})

it('preserves an explicitly focused modal field', () => {
  const view = render(<Modal onClose={vi.fn()}>
    <div role="dialog" aria-label="Autofocus modal"><input aria-label="Preferred" autoFocus /><button>Later</button></div>
  </Modal>)

  expect(screen.getByRole('textbox', { name: 'Preferred' })).toHaveFocus()
  view.unmount()
})
