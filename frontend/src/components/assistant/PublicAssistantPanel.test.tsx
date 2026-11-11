import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { I18nProvider } from '../../i18n'
import { Router } from '../../router'
import { PublicAssistantPanel } from './PublicAssistantPanel'

const close = vi.fn()
vi.mock('../../workspace', () => ({ useWorkspace: () => ({ setAssistantOpen: close }) }))
const fetcher = vi.fn()
beforeEach(() => { close.mockReset(); fetcher.mockReset(); vi.stubGlobal('fetch', fetcher); vi.stubEnv('VITE_PUBLIC_DEMO', 'true') })
afterEach(() => { vi.unstubAllGlobals(); vi.unstubAllEnvs() })
function panel() { return render(<I18nProvider><Router><PublicAssistantPanel /></Router></I18nProvider>) }
const citation = { pageId: 'page', revisionId: 'revision', pageTitle: 'Review rules', draft: false }
const success = (answer = 'Resolve the objection first.', citations = [citation]) => ({ ok: true, json: async () => ({ answer, citations }) })
function submit(content: string) {
  fireEvent.change(screen.getByLabelText('Your question'), { target: { value: content } })
  fireEvent.submit(screen.getByLabelText('Your question').closest('form')!)
}

it('keeps history within this mounted tab, sends bounded context, and links exact cited revisions', async () => {
  fetcher.mockResolvedValueOnce(success('A'.repeat(900))).mockResolvedValueOnce(success('Another answer.', []))
  panel()
  expect(fetcher).not.toHaveBeenCalled()
  expect(screen.getByText('Let Viki explain itself.')).toBeVisible()
  expect(screen.getByText(/History stays in this tab/)).toBeVisible()
  expect(screen.getByRole('button', { name: 'New chat' })).toBeDisabled()
  fireEvent.submit(screen.getByLabelText('Your question').closest('form')!)
  expect(fetcher).not.toHaveBeenCalled()
  submit('What blocks approval?')
  await screen.findByRole('link', { name: 'Review rules' })
  expect(screen.getByRole('link', { name: 'Review rules' })).toHaveAttribute('href', '/page/page?revision=revision')
  expect(JSON.parse(fetcher.mock.calls[0][1].body)).toEqual({ question: 'What blocks approval?', history: [] })
  submit('How do I resolve it?')
  await screen.findByText('Another answer.')
  const second = JSON.parse(fetcher.mock.calls[1][1].body)
  expect(second.history[0].content).toBe('What blocks approval?')
  expect(second.history[1].content).toHaveLength(800)
  fireEvent.click(screen.getByRole('button', { name: 'New chat' }))
  expect(screen.queryByText('Another answer.')).not.toBeInTheDocument()
  expect(screen.getByText('Let Viki explain itself.')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Close assistant' }))
  expect(close).toHaveBeenCalledWith(false)
})

it('prevents duplicate submissions while the answer is pending', async () => {
  let resolve!: (value: ReturnType<typeof success>) => void
  fetcher.mockImplementationOnce(() => new Promise((done) => { resolve = done }))
  panel()
  submit('First question')
  expect(screen.getByRole('status')).toHaveTextContent('Reading the documentation')
  submit('Do not submit twice')
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(screen.getByRole('button', { name: 'Ask' })).toBeDisabled()
  resolve(success())
  await screen.findByText('Resolve the objection first.')
  expect(screen.getByRole('button', { name: 'Ask' })).toBeEnabled()
})

it('shows quota and network failures without inventing an assistant answer', async () => {
  fetcher.mockResolvedValueOnce({ ok: false, json: async () => ({ error: { message: 'Free allowance used up.' } }) })
    .mockRejectedValueOnce('offline')
  panel()
  submit('Question')
  expect(await screen.findByRole('alert')).toHaveTextContent('Free allowance used up.')
  submit('Another question')
  await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('The answer could not be loaded.'))
  expect(screen.queryByText('Viki', { selector: 'strong' })).not.toBeInTheDocument()
})

it('keeps two visitors’ component histories separate', async () => {
  fetcher.mockResolvedValue(success())
  render(<I18nProvider><Router><PublicAssistantPanel /><PublicAssistantPanel /></Router></I18nProvider>)
  const [first, second] = screen.getAllByRole('region', { name: 'Ask about Viki' })
  await userEvent.type(within(first).getByLabelText('Your question'), 'Visitor A private phrase')
  fireEvent.submit(within(first).getByLabelText('Your question').closest('form')!)
  await within(first).findByText('Resolve the objection first.')
  expect(within(second).queryByText('Visitor A private phrase')).not.toBeInTheDocument()
  expect(within(second).getByText('Let Viki explain itself.')).toBeVisible()
})
