import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { NewPageDialog } from './NewPageDialog'

const mocks = vi.hoisted(() => ({ createPage: vi.fn(), reloadPages: vi.fn(), navigate: vi.fn() }))

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, createPage: mocks.createPage } }
})
vi.mock('../workspace', () => ({ useWorkspace: () => ({ pages: [], reloadPages: mocks.reloadPages }) }))
vi.mock('../router', () => ({ useRouter: () => ({ navigate: mocks.navigate }) }))

beforeEach(() => {
  Object.values(mocks).forEach((mock) => mock.mockReset())
  mocks.reloadPages.mockResolvedValue(undefined)
})

describe('NewPageDialog', () => {
  it.each([
    ['concept', 'Vytvoriť koncept'],
    ['feature', 'Vytvoriť funkciu'],
  ] as const)('keeps the %s launcher type fixed', (initialKind, heading) => {
    render(<NewPageDialog initialKind={initialKind} onClose={vi.fn()} />)

    expect(screen.getByRole('heading', { name: heading })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Typ stránky' })).not.toBeInTheDocument()
  })

  it('creates a verb concept with an accent-insensitive generated slug', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    let resolve!: (value: { page: { id: string } }) => void
    mocks.createPage.mockReturnValue(new Promise((next) => { resolve = next }))
    render(<NewPageDialog initialKind="concept" onClose={onClose} />)

    await user.click(screen.getByRole('button', { name: 'Druh konceptu' }))
    await user.click(screen.getByRole('option', { name: 'Sloveso' }))
    await user.type(screen.getByLabelText('Názov'), 'Číslo zákazníka')
    expect(screen.getByPlaceholderText('service-availability')).toHaveValue('cislo-zakaznika')
    await user.click(screen.getByRole('button', { name: 'Vytvoriť draft' }))
    expect(screen.getByRole('button', { name: 'Vytváram…' })).toBeDisabled()

    resolve({ page: { id: 'page-1' } })
    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledWith('/page/page-1'))
    expect(mocks.createPage).toHaveBeenCalledWith({
      kind: 'concept', conceptKind: 'verb', slug: 'cislo-zakaznika',
      content: { title: 'Číslo zákazníka', bodyMd: '', aliases: [], steps: [], references: [] },
    })
    expect(mocks.reloadPages).toHaveBeenCalledOnce()
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('preserves a custom feature slug and omits concept-only data', async () => {
    const user = userEvent.setup()
    mocks.createPage.mockResolvedValue({ page: { id: 'feature-1' } })
    render(<NewPageDialog initialKind="feature" onClose={vi.fn()} />)

    expect(screen.queryByRole('button', { name: 'Druh konceptu' })).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('Názov'), 'First title')
    await user.clear(screen.getByPlaceholderText('service-availability'))
    await user.type(screen.getByPlaceholderText('service-availability'), 'custom-feature')
    await user.clear(screen.getByLabelText('Názov'))
    await user.type(screen.getByLabelText('Názov'), 'Changed title')
    expect(screen.getByPlaceholderText('service-availability')).toHaveValue('custom-feature')
    await user.click(screen.getByRole('button', { name: 'Vytvoriť draft' }))

    await waitFor(() => expect(mocks.createPage).toHaveBeenCalledWith(expect.objectContaining({ kind: 'feature', slug: 'custom-feature' })))
    expect(mocks.createPage.mock.calls[0][0]).not.toHaveProperty('conceptKind')
  })

  it('closes from explicit controls and the backdrop only', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    const view = render(<NewPageDialog initialKind="feature" onClose={onClose} />)

    fireEvent.mouseDown(view.container.querySelector('.modal-card')!)
    expect(onClose).not.toHaveBeenCalled()
    fireEvent.mouseDown(view.container.querySelector('.modal-backdrop')!)
    expect(onClose).toHaveBeenCalledOnce()
    await user.click(screen.getByRole('button', { name: 'Zavrieť' }))
    await user.click(screen.getByRole('button', { name: 'Zrušiť' }))
    expect(onClose).toHaveBeenCalledTimes(3)
  })

  it('reports provider and fallback creation failures', async () => {
    const user = userEvent.setup()
    mocks.createPage.mockRejectedValueOnce(new Error('offline'))
    const view = render(<NewPageDialog initialKind="feature" onClose={vi.fn()} />)
    await user.type(screen.getByLabelText('Názov'), 'Feature')
    await user.click(screen.getByRole('button', { name: 'Vytvoriť draft' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('offline')

    mocks.createPage.mockRejectedValueOnce('failure')
    fireEvent.submit(view.container.querySelector('form')!)
    expect(await screen.findByRole('alert')).toHaveTextContent('Stránku sa nepodarilo vytvoriť.')
  })
})
