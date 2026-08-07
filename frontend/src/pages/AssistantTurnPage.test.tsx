import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Page } from '../api/types'
import type { AssistantTurnProgress } from '../assistant'
import { AssistantTurnPage, draftDestination } from './AssistantTurnPage'

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  stop: vi.fn(),
  pages: vi.fn(),
  assistant: {} as Record<string, unknown>,
}))

vi.mock('../assistant', () => ({ useAssistant: () => mocks.assistant }))
vi.mock('../router', () => ({
  Link: ({ to, children, className }: { to: string; children: React.ReactNode; className?: string }) => <a href={to} className={className}>{children}</a>,
  useRouter: () => ({ navigate: mocks.navigate }),
}))
vi.mock('../api/client', () => ({ api: { pages: mocks.pages } }))

const turn: AssistantTurnProgress = {
  id: 'turn-live',
  mode: 'edit',
  status: 'running',
  activities: ['submitted', 'searching', 'searched', 'reading'],
  summary: 'Pripravujem funkciu rezervácie.',
  drafts: [{ revisionId: 'revision-concept', pageId: 'page-concept', pageTitle: 'Zákazník' }],
}

describe('AssistantTurnPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useRealTimers()
    mocks.assistant = {
      turns: { [turn.id]: turn },
      conversation: null,
      clarification: null,
      connection: 'connected',
      stop: mocks.stop,
    }
  })

  it('shows a safe live account of the edit without exposing raw agent reasoning', () => {
    render(<AssistantTurnPage turnId={turn.id} />)

    expect(screen.getByRole('heading', { name: 'Viki pripravuje zmeny' })).toBeInTheDocument()
    expect(screen.getByText('Rozumiem zadaniu')).toBeInTheDocument()
    expect(screen.getByText('Hľadám súvisiace informácie')).toBeInTheDocument()
    expect(screen.getByText('Našiel som súvisiace stránky')).toBeInTheDocument()
    expect(screen.getAllByText('Čítam existujúce stránky')).toHaveLength(2)
    expect(screen.getByText('Pripravujem funkciu rezervácie.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Zákazník/ })).toHaveAttribute('href', '/page/page-concept?revision=revision-concept')
  })

  it('opens the new Feature after the completed turn, ahead of its concepts and scenarios', async () => {
    vi.useFakeTimers()
    const completed: AssistantTurnProgress = {
      ...turn,
      status: 'completed',
      activities: ['submitted', 'drafting', 'drafted'],
      drafts: [
        turn.drafts[0],
        { revisionId: 'revision-scenario', pageId: 'page-scenario', pageTitle: 'Úspešná rezervácia' },
        { revisionId: 'revision-feature', pageId: 'page-feature', pageTitle: 'Rezervácia služby' },
      ],
    }
    mocks.assistant = { ...mocks.assistant, turns: { [turn.id]: completed } }
    mocks.pages.mockResolvedValue({ pages: [
      { id: 'page-concept', kind: 'concept' },
      { id: 'page-scenario', kind: 'scenario' },
      { id: 'page-feature', kind: 'feature' },
    ] })

    render(<AssistantTurnPage turnId={turn.id} />)
    expect(screen.getByText('Hotovo. Otváram novú funkciu…')).toBeInTheDocument()

    await act(async () => {
      vi.advanceTimersByTime(850)
      await Promise.resolve()
    })

    expect(mocks.navigate).toHaveBeenCalledWith('/page/page-feature?revision=revision-feature', true)
  })

  it('stops a running turn and covers every safe activity label and icon', async () => {
    const user = userEvent.setup()
    const cases = [
      ['thinking', 'Rozumiem zadaniu'],
      ['searching', 'Hľadám súvisiace informácie'],
      ['searched', 'Našiel som súvisiace stránky'],
      ['reading', 'Čítam existujúce stránky'],
      ['read', 'Mám potrebné podklady'],
      ['drafting', 'Vytváram drafty'],
      ['editing', 'Vytváram drafty'],
      ['writing', 'Vytváram drafty'],
      ['applying', 'Vytváram drafty'],
      ['drafted', 'Drafty sú pripravené'],
      ['awaiting_approval', 'Drafty sú pripravené'],
      ['clarification_answered', 'Pokračujem po doplnení'],
      ['unknown', 'Pracujem na zmenách'],
    ] as const
    mocks.assistant = { ...mocks.assistant, turns: { [turn.id]: { ...turn, activities: [] } } }
    const view = render(<AssistantTurnPage turnId={turn.id} />)
    expect(screen.getAllByText('Rozumiem zadaniu')).toHaveLength(2)
    await user.click(screen.getByRole('button', { name: 'Zastaviť' }))
    expect(mocks.stop).toHaveBeenCalledOnce()

    for (const [state, label] of cases) {
      mocks.assistant = { ...mocks.assistant, turns: { [turn.id]: { ...turn, activities: [state] } } }
      view.rerender(<AssistantTurnPage turnId={turn.id} />)
      expect(screen.getAllByText(label)).toHaveLength(2)
    }
  })

  it('shows clarification, stopped, and error outcomes without redirecting', () => {
    const view = render(<AssistantTurnPage turnId={turn.id} />)

    mocks.assistant = {
      ...mocks.assistant,
      turns: { [turn.id]: { ...turn, status: 'awaiting_clarification' } },
      clarification: { turnId: turn.id },
    }
    view.rerender(<AssistantTurnPage turnId={turn.id} />)
    expect(screen.getAllByText('Potrebujem od vás doplnenie. Odpovedzte v asistentovi.')).toHaveLength(2)

    mocks.assistant = { ...mocks.assistant, turns: { [turn.id]: { ...turn, status: 'stopped' } }, clarification: null }
    view.rerender(<AssistantTurnPage turnId={turn.id} />)
    expect(screen.getByText('Príprava zmien bola zastavená.')).toBeInTheDocument()

    mocks.assistant = { ...mocks.assistant, turns: { [turn.id]: { ...turn, status: 'error', error: 'Hermes zlyhal.' } } }
    view.rerender(<AssistantTurnPage turnId={turn.id} />)
    expect(screen.getAllByText('Hermes zlyhal.')).toHaveLength(2)

    mocks.assistant = { ...mocks.assistant, turns: { [turn.id]: { ...turn, status: 'error', error: undefined } } }
    view.rerender(<AssistantTurnPage turnId={turn.id} />)
    expect(screen.getByText('Zmeny sa nepodarilo pripraviť.')).toBeInTheDocument()
  })

  it('recovers a completed turn from canonical Hermes history and removes duplicate receipts', () => {
    vi.useFakeTimers()
    mocks.assistant = {
      ...mocks.assistant,
      turns: {},
      conversation: {
        state: 'idle',
        messages: [
          { id: `${turn.id}-user`, role: 'user', mode: 'edit', content: 'ignored', drafts: [] },
          { id: `turn-${turn.id}`, role: 'assistant', mode: 'edit', content: '', drafts: [turn.drafts[0]] },
          { id: `${turn.id}-assistant`, role: 'assistant', mode: 'edit', content: 'Hotovo.', drafts: [turn.drafts[0]] },
          { id: `${turn.id}-assistant-2`, role: 'assistant', mode: 'edit', content: 'Draft je pripravený.', drafts: [] },
        ],
      },
    }

    const view = render(<AssistantTurnPage turnId={turn.id} />)
    expect(screen.getByText('Hotovo.')).toBeInTheDocument()
    expect(screen.getByText('Draft je pripravený.')).toBeInTheDocument()
    expect(screen.getAllByRole('link', { name: /Zákazník/ })).toHaveLength(1)
    view.unmount()
  })

  it('recovers every active and terminal conversation status from Hermes history', () => {
    const expectations = [
      ['running', 'Rozumiem zadaniu'],
      ['awaiting_clarification', 'Potrebujem od vás doplnenie. Odpovedzte v asistentovi.'],
      ['stopped', 'Príprava zmien bola zastavená.'],
      ['error', 'Zmeny sa nepodarilo pripraviť.'],
    ] as const
    mocks.assistant = {
      ...mocks.assistant,
      turns: {},
      conversation: { state: 'running', messages: [{ id: `${turn.id}-assistant`, role: 'assistant', content: 'Pracujem.', drafts: [] }] },
    }
    const view = render(<AssistantTurnPage turnId={turn.id} />)

    for (const [state, label] of expectations) {
      mocks.assistant = { ...mocks.assistant, conversation: { ...(mocks.assistant.conversation as object), state } }
      view.rerender(<AssistantTurnPage turnId={turn.id} />)
      expect(screen.getAllByText(label).length).toBeGreaterThan(0)
    }
  })

  it('reports an unavailable turn when neither live progress nor matching history exists', () => {
    mocks.assistant = { ...mocks.assistant, turns: {}, conversation: null }
    const view = render(<AssistantTurnPage turnId="missing" />)
    expect(screen.getByRole('alert')).toHaveTextContent('Tento priebeh už nie je dostupný.')

    mocks.assistant = { ...mocks.assistant, conversation: { state: 'idle', messages: [{ id: 'other-assistant', role: 'assistant', drafts: [] }] } }
    view.rerender(<AssistantTurnPage turnId="missing" />)
    expect(screen.getByRole('alert')).toHaveTextContent('Tento priebeh už nie je dostupný.')
  })

  it('keeps completed drafts visible when destination lookup fails', async () => {
    vi.useFakeTimers()
    mocks.assistant = { ...mocks.assistant, turns: { [turn.id]: { ...turn, status: 'completed' } } }
    mocks.pages.mockRejectedValue(new Error('offline'))
    render(<AssistantTurnPage turnId={turn.id} />)

    await act(async () => {
      vi.advanceTimersByTime(850)
      await Promise.resolve()
    })

    expect(screen.getByRole('alert')).toHaveTextContent('Zmeny sa nepodarilo pripraviť.')
    expect(mocks.navigate).not.toHaveBeenCalled()
  })

  it('does not update navigation or errors after the progress screen is left', async () => {
    vi.useFakeTimers()
    let resolvePages!: (value: { pages: Page[] }) => void
    mocks.assistant = { ...mocks.assistant, turns: { [turn.id]: { ...turn, status: 'completed' } } }
    mocks.pages.mockReturnValue(new Promise((resolve) => { resolvePages = resolve }))
    const view = render(<AssistantTurnPage turnId={turn.id} />)

    act(() => vi.advanceTimersByTime(850))
    view.unmount()
    await act(async () => resolvePages({ pages: [] }))
    expect(mocks.navigate).not.toHaveBeenCalled()

    let rejectPages!: (reason: Error) => void
    mocks.pages.mockReturnValue(new Promise((_resolve, reject) => { rejectPages = reject }))
    const second = render(<AssistantTurnPage turnId={turn.id} />)
    act(() => vi.advanceTimersByTime(850))
    second.unmount()
    await act(async () => rejectPages(new Error('offline')))
    expect(mocks.navigate).not.toHaveBeenCalled()
  })

  it('chooses the best available draft destination with a deterministic fallback', () => {
    const concept = { revisionId: 'concept revision', pageId: 'concept', pageTitle: 'Concept' }
    const scenario = { revisionId: 'scenario revision', pageId: 'scenario', pageTitle: 'Scenario' }
    const feature = { revisionId: 'feature revision', pageId: 'feature', pageTitle: 'Feature' }
    const pages = [
      { id: 'concept', kind: 'concept' },
      { id: 'scenario', kind: 'scenario' },
      { id: 'feature', kind: 'feature' },
    ] as Page[]

    expect(draftDestination([concept, scenario, feature], pages)).toBe('/page/feature?revision=feature%20revision')
    expect(draftDestination([concept, scenario], pages)).toBe('/page/scenario?revision=scenario%20revision')
    expect(draftDestination([concept], pages)).toBe('/page/concept?revision=concept%20revision')
    expect(draftDestination([concept], [])).toBe('/page/concept?revision=concept%20revision')
    expect(draftDestination([], [])).toBe('/features')
  })
})
