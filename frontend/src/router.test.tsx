import { act, fireEvent, render, renderHook, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { expect, it, vi } from 'vitest'
import { Link, Router, useRouter } from './router'

function wrapper({ children }: { children: ReactNode }) {
  return <Router>{children}</Router>
}

it('navigates, replaces, responds to history, and exposes parsed search state', () => {
  window.history.replaceState({}, '', '/concepts?q=zmluva')
  const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  const { result } = renderHook(() => useRouter(), { wrapper })
  expect(result.current.pathname).toBe('/concepts')
  expect(result.current.search.get('q')).toBe('zmluva')

  act(() => result.current.navigate('/features', true))
  expect(result.current.pathname).toBe('/features')
  act(() => result.current.navigate('/drafts'))
  expect(result.current.location).toBe('/drafts')
  act(() => {
    window.history.pushState({}, '', '/audit')
    fireEvent.popState(window)
  })
  expect(result.current.pathname).toBe('/audit')
  expect(scrollTo).toHaveBeenCalled()
  scrollTo.mockRestore()
})

it('intercepts only unmodified primary clicks on links', async () => {
  const user = userEvent.setup()
  const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  window.history.replaceState({}, '', '/')
  render(<Router><Link to="/concepts" className="link" ariaLabel="Concepts">Open</Link></Router>)
  const link = screen.getByRole('link', { name: 'Concepts' })
  await user.click(link)
  expect(window.location.pathname).toBe('/concepts')
  fireEvent.click(link, { button: 1 })
  fireEvent.click(link, { metaKey: true })
  fireEvent.click(link, { ctrlKey: true })
  fireEvent.click(link, { shiftKey: true })
  fireEvent.click(link, { altKey: true })
  expect(link).toHaveClass('link')
  scrollTo.mockRestore()
})

it('requires the router provider', () => {
  expect(() => renderHook(() => useRouter())).toThrow('useRouter must be used inside Router')
})
