import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '../api/client'
import { LoginPage } from './LoginPage'

const mocks = vi.hoisted(() => ({ login: vi.fn() }))
vi.mock('../auth', () => ({ useAuth: () => ({ login: mocks.login }) }))

beforeEach(() => mocks.login.mockReset())

describe('LoginPage', () => {
  it('keeps only the viki wordmark in the first section', () => {
    const { container } = render(<LoginPage />)
    const firstSection = container.querySelector('section:first-of-type')

    expect(firstSection).toHaveTextContent(/^viki$/)
    expect(screen.queryByText('Zdieľané porozumenie predtým, než vznikne nový systém.')).not.toBeInTheDocument()
  })

  it('submits edited credentials and exposes the pending state', async () => {
    const user = userEvent.setup()
    let resolve!: () => void
    mocks.login.mockReturnValue(new Promise<void>((next) => { resolve = next }))
    render(<LoginPage />)

    await user.clear(screen.getByLabelText('E-mail'))
    await user.type(screen.getByLabelText('E-mail'), 'new@example.com')
    await user.clear(screen.getByLabelText('Heslo'))
    await user.type(screen.getByLabelText('Heslo'), 'secret')
    await user.click(screen.getByRole('button', { name: 'Prihlásiť sa' }))
    expect(mocks.login).toHaveBeenCalledWith('new@example.com', 'secret')
    expect(screen.getByRole('button', { name: 'Prihlasujem…' })).toBeDisabled()

    resolve()
    await waitFor(() => expect(screen.getByRole('button', { name: 'Prihlásiť sa' })).toBeEnabled())
  })

  it('shows API messages and a localized fallback for unexpected failures', async () => {
    const view = render(<LoginPage />)
    mocks.login.mockRejectedValueOnce(new APIError(401, 'invalid_credentials', 'Nesprávne údaje'))
    fireEvent.submit(view.container.querySelector('form')!)
    expect(await screen.findByRole('alert')).toHaveTextContent('Nesprávne údaje')

    mocks.login.mockRejectedValueOnce(new Error('network'))
    fireEvent.submit(view.container.querySelector('form')!)
    expect(await screen.findByRole('alert')).toHaveTextContent('Prihlásenie sa nepodarilo.')
  })
})
