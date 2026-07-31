import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { LoginPage } from './LoginPage'

vi.mock('../auth', () => ({
  useAuth: () => ({ login: vi.fn() }),
}))

describe('LoginPage', () => {
  it('keeps only the viki wordmark in the first section', () => {
    const { container } = render(<LoginPage />)
    const firstSection = container.querySelector('section:first-of-type')

    expect(firstSection).toHaveTextContent(/^viki$/)
    expect(screen.queryByText('Zdieľané porozumenie predtým, než vznikne nový systém.')).not.toBeInTheDocument()
  })
})
