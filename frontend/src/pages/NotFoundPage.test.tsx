import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { Router } from '../router'
import { NotFoundPage } from './NotFoundPage'

it('offers a route back to the concept library', () => {
  render(<Router><NotFoundPage /></Router>)

  expect(screen.getByRole('heading', { name: 'Stránka sa nenašla' })).toBeVisible()
  expect(screen.getByRole('link', { name: 'Späť na koncepty' })).toHaveAttribute('href', '/concepts')
})
