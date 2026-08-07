import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { NewPageDialog } from './NewPageDialog'

const mocks = vi.hoisted(() => ({ createPage: vi.fn(), stepDefinitions: vi.fn(), reloadPages: vi.fn(), navigate: vi.fn() }))

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, createPage: mocks.createPage, stepDefinitions: mocks.stepDefinitions } }
})
vi.mock('../workspace', () => ({ useWorkspace: () => ({ pages: [], reloadPages: mocks.reloadPages }) }))
vi.mock('../router', () => ({ useRouter: () => ({ navigate: mocks.navigate }) }))

beforeEach(() => {
  Object.values(mocks).forEach((mock) => mock.mockReset())
  mocks.reloadPages.mockResolvedValue(undefined)
  mocks.stepDefinitions.mockResolvedValue({ definitions: [] })
})

async function fillNewSteps(user: ReturnType<typeof userEvent.setup>, values: [string, string, string]) {
  const proposeButtons = screen.getAllByRole('button', { name: 'Navrhnúť nový krok' })
  for (const button of proposeButtons) await user.click(button)
  for (const [index, value] of values.entries()) {
    await user.type(screen.getByRole('textbox', { name: `Nová definícia kroku ${index + 1}` }), value)
  }
}

describe('NewPageDialog', () => {
  it.each([
    ['concept', 'Vytvoriť koncept'],
    ['feature', 'Vytvoriť funkciu'],
    ['scenario', 'Vytvoriť scenár'],
  ] as const)('keeps the %s launcher type fixed', (initialKind, heading) => {
    render(<NewPageDialog initialKind={initialKind} parentId={initialKind === 'scenario' ? 'feature-1' : undefined} onClose={vi.fn()} />)

    expect(screen.getByRole('heading', { name: heading })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Typ stránky' })).not.toBeInTheDocument()
    expect(screen.queryByText(/^Slug/)).not.toBeInTheDocument()
    expect(screen.queryByPlaceholderText('service-availability')).not.toBeInTheDocument()
  })

  it('creates a scenario draft beneath its feature', async () => {
    const user = userEvent.setup()
    mocks.createPage.mockResolvedValue({ page: { id: 'scenario-1' } })
    render(<NewPageDialog initialKind="scenario" parentId="feature-1" onClose={vi.fn()} />)

    await user.type(screen.getByLabelText('Názov'), 'Podpis zmluvy')
    await fillNewSteps(user, ['zákazník má pripravenú zmluvu', 'zákazník zmluvu podpíše', 'systém uloží podpis'])
    await user.click(screen.getByRole('button', { name: 'Vytvoriť draft' }))

    await waitFor(() => expect(mocks.createPage).toHaveBeenCalledWith({
      kind: 'scenario',
      parentId: 'feature-1',
      slug: 'podpis-zmluvy',
      content: {
        title: 'Podpis zmluvy', bodyMd: '', references: [],
        steps: [
          { keyword: 'given', definitionId: undefined, expression: 'zákazník má pripravenú zmluvu', arguments: [], text: 'zákazník má pripravenú zmluvu' },
          { keyword: 'when', definitionId: undefined, expression: 'zákazník zmluvu podpíše', arguments: [], text: 'zákazník zmluvu podpíše' },
          { keyword: 'then', definitionId: undefined, expression: 'systém uloží podpis', arguments: [], text: 'systém uloží podpis' },
        ],
      },
    }))
    expect(mocks.createPage.mock.calls[0][0]).not.toHaveProperty('conceptKind')
  })

  it('creates a feature and its first independently versioned scenario together', async () => {
    const user = userEvent.setup()
    mocks.createPage.mockResolvedValue({ page: { id: 'feature-1' } })
    render(<NewPageDialog initialKind="feature" onClose={vi.fn()} />)

    await user.type(screen.getByLabelText('Názov'), 'Overenie súhlasu')
    await user.type(screen.getByLabelText('Názov scenára'), 'Zákazník udelí súhlas')
    await fillNewSteps(user, ['prebieha telefonát so zákazníkom', 'zákazník udelí súhlas', 'systém zaznamená súhlas'])
    await user.click(screen.getByRole('button', { name: 'Vytvoriť draft' }))

    await waitFor(() => expect(mocks.createPage).toHaveBeenCalledWith({
      kind: 'feature',
      slug: 'overenie-suhlasu',
      content: { title: 'Overenie súhlasu', bodyMd: '', steps: [], references: [] },
      initialScenario: {
        slug: 'zakaznik-udeli-suhlas',
        content: {
          title: 'Zákazník udelí súhlas', bodyMd: '', references: [],
          steps: [
            { keyword: 'given', definitionId: undefined, expression: 'prebieha telefonát so zákazníkom', arguments: [], text: 'prebieha telefonát so zákazníkom' },
            { keyword: 'when', definitionId: undefined, expression: 'zákazník udelí súhlas', arguments: [], text: 'zákazník udelí súhlas' },
            { keyword: 'then', definitionId: undefined, expression: 'systém zaznamená súhlas', arguments: [], text: 'systém zaznamená súhlas' },
          ],
        },
      },
    }))
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
    expect(screen.queryByText(/^Slug/)).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Vytvoriť draft' }))
    expect(screen.getByRole('button', { name: 'Vytváram…' })).toBeDisabled()

    resolve({ page: { id: 'page-1' } })
    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledWith('/page/page-1'))
    expect(mocks.createPage).toHaveBeenCalledWith({
      kind: 'concept', conceptKind: 'verb', slug: 'cislo-zakaznika',
      content: { title: 'Číslo zákazníka', bodyMd: '', steps: [], references: [] },
    })
    expect(mocks.reloadPages).toHaveBeenCalledOnce()
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('derives hidden feature and scenario slugs from their final titles', async () => {
    const user = userEvent.setup()
    mocks.createPage.mockResolvedValue({ page: { id: 'feature-1' } })
    render(<NewPageDialog initialKind="feature" onClose={vi.fn()} />)

    expect(screen.queryByRole('button', { name: 'Druh konceptu' })).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('Názov'), 'First title')
    await user.clear(screen.getByLabelText('Názov'))
    await user.type(screen.getByLabelText('Názov'), 'Changed title')
    await user.type(screen.getByLabelText('Názov scenára'), 'Initial scenario')
    await user.clear(screen.getByLabelText('Názov scenára'))
    await user.type(screen.getByLabelText('Názov scenára'), 'Changed scenario')
    await fillNewSteps(user, ['given', 'when', 'then'])
    expect(screen.queryByText(/^Slug/)).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Vytvoriť draft' }))

    await waitFor(() => expect(mocks.createPage).toHaveBeenCalledWith(expect.objectContaining({
      kind: 'feature',
      slug: 'changed-title',
      initialScenario: expect.objectContaining({ slug: 'changed-scenario' }),
    })))
    expect(mocks.createPage.mock.calls[0][0]).not.toHaveProperty('conceptKind')
  })

  it('closes from explicit controls and the backdrop only', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    render(<NewPageDialog initialKind="feature" onClose={onClose} />)

    fireEvent.mouseDown(screen.getByRole('dialog'))
    expect(onClose).not.toHaveBeenCalled()
    fireEvent.mouseDown(document.body.querySelector('.modal-backdrop')!)
    expect(onClose).toHaveBeenCalledOnce()
    await user.click(screen.getByRole('button', { name: 'Zavrieť' }))
    await user.click(screen.getByRole('button', { name: 'Zrušiť' }))
    expect(onClose).toHaveBeenCalledTimes(3)
  })

  it('reports provider and fallback creation failures', async () => {
    const user = userEvent.setup()
    mocks.createPage.mockRejectedValueOnce(new Error('offline'))
    render(<NewPageDialog initialKind="concept" onClose={vi.fn()} />)
    await user.type(screen.getByLabelText('Názov'), 'Feature')
    await user.click(screen.getByRole('button', { name: 'Vytvoriť draft' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('offline')

    mocks.createPage.mockRejectedValueOnce('failure')
    fireEvent.submit(screen.getByRole('dialog'))
    expect(await screen.findByRole('alert')).toHaveTextContent('Stránku sa nepodarilo vytvoriť.')
  })

  it('keeps manual scenario creation usable when the definition catalog is unavailable', async () => {
    mocks.stepDefinitions.mockRejectedValue(new Error('catalog offline'))
    render(<NewPageDialog initialKind="scenario" parentId="feature-1" onClose={vi.fn()} />)

    await waitFor(() => expect(mocks.stepDefinitions).toHaveBeenCalledOnce())
    expect(screen.getAllByRole('button', { name: 'Navrhnúť nový krok' })).toHaveLength(3)
  })
})
