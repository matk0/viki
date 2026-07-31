import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { NewPageDialog } from './NewPageDialog'

vi.mock('../workspace', () => ({
  useWorkspace: () => ({ pages: [], reloadPages: vi.fn() }),
}))

vi.mock('../router', () => ({
  useRouter: () => ({ navigate: vi.fn() }),
}))

describe('NewPageDialog', () => {
  it.each([
    ['primitive', 'Vytvoriť pojem'],
    ['scenario', 'Vytvoriť scenár'],
  ] as const)('keeps the %s launcher type fixed', (initialKind, heading) => {
    render(<NewPageDialog initialKind={initialKind} onClose={vi.fn()} />)

    expect(screen.getByRole('heading', { name: heading })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Typ stránky' })).not.toBeInTheDocument()
  })
})
