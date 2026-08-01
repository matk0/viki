import { act, render, renderHook, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AuthProvider, useAuth } from './auth'

const mocks = vi.hoisted(() => ({ me: vi.fn(), login: vi.fn(), logout: vi.fn() }))

vi.mock('./api/client', () => ({
  APIError: class APIError extends Error {
    constructor(public status: number, public code: string, message: string) { super(message) }
  },
  api: mocks,
}))

function Consumer() {
  const { user, loading, login, logout } = useAuth()
  return <div>
    <span>{loading ? 'loading' : user?.email ?? 'anonymous'}</span>
    <button onClick={() => void login('matej@example.com', 'password')}>login</button>
    <button onClick={() => void logout()}>logout</button>
  </div>
}

function wrapper({ children }: { children: ReactNode }) {
  return <AuthProvider>{children}</AuthProvider>
}

describe('AuthProvider', () => {
  beforeEach(() => {
    mocks.me.mockReset()
    mocks.login.mockReset()
    mocks.logout.mockReset()
  })

  it('loads the current user then supports login and logout state transitions', async () => {
    const user = userEvent.setup()
    mocks.me.mockResolvedValue({ user: { id: 'user-1', email: 'current@example.com', displayName: 'Current', createdAt: '' } })
    mocks.login.mockResolvedValue({ user: { id: 'user-2', email: 'matej@example.com', displayName: 'Matej', createdAt: '' } })
    mocks.logout.mockResolvedValue(undefined)
    render(<AuthProvider><Consumer /></AuthProvider>)

    expect(screen.getByText('loading')).toBeInTheDocument()
    expect(await screen.findByText('current@example.com')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'login' }))
    expect(await screen.findByText('matej@example.com')).toBeInTheDocument()
    expect(mocks.login).toHaveBeenCalledWith('matej@example.com', 'password')
    await user.click(screen.getByRole('button', { name: 'logout' }))
    expect(await screen.findByText('anonymous')).toBeInTheDocument()
  })

  it('treats unauthorized bootstrap as anonymous and reports unexpected failures', async () => {
    const error = new Error('offline')
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const { APIError } = await import('./api/client')
    mocks.me.mockRejectedValueOnce(new APIError(401, 'unauthorized', 'unauthorized'))
    const first = render(<AuthProvider><Consumer /></AuthProvider>)
    expect(await screen.findByText('anonymous')).toBeInTheDocument()
    first.unmount()

    mocks.me.mockRejectedValueOnce(error)
    const unexpected = render(<AuthProvider><Consumer /></AuthProvider>)
    await waitFor(() => expect(consoleError).toHaveBeenCalledWith(error))
    unexpected.unmount()
    const forbidden = new APIError(403, 'forbidden', 'forbidden')
    mocks.me.mockRejectedValueOnce(forbidden)
    render(<AuthProvider><Consumer /></AuthProvider>)
    await waitFor(() => expect(consoleError).toHaveBeenCalledWith(forbidden))
    consoleError.mockRestore()
  })

  it('requires the provider', () => {
    expect(() => renderHook(() => useAuth())).toThrow('useAuth must be used inside AuthProvider')
  })

  it('exposes a stable loading value through the hook', async () => {
    let resolve!: (value: unknown) => void
    mocks.me.mockReturnValue(new Promise((next) => { resolve = next }))
    const { result } = renderHook(() => useAuth(), { wrapper })
    expect(result.current.loading).toBe(true)
    await act(async () => resolve({ user: { id: 'user-1', email: 'a@b.c', displayName: 'A', createdAt: '' } }))
    expect(result.current.loading).toBe(false)
  })
})
