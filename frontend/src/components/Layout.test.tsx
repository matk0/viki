import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, expect, it, vi } from 'vitest'
import { I18nProvider } from '../i18n'
import { Layout } from './Layout'

const mocks = vi.hoisted(() => ({
  assistantOpen: false,
  setAssistantOpen: vi.fn(),
  closeNewPage: vi.fn(),
  logout: vi.fn(),
  voiceListening: false,
  startVoice: vi.fn(),
  stopVoice: vi.fn(),
  cancelVoice: vi.fn(),
  navigate: vi.fn(),
  pathname: '/concepts',
  newPageKind: null as 'concept' | 'feature' | null,
}))

vi.mock('../auth', () => ({
  useAuth: () => ({
    user: { displayName: 'Matej', email: 'matej@matejlukasik.com' },
    logout: mocks.logout,
  }),
}))

vi.mock('../workspace', () => ({
  useWorkspace: () => ({
    assistantOpen: mocks.assistantOpen,
    setAssistantOpen: mocks.setAssistantOpen,
    newPageKind: mocks.newPageKind,
    closeNewPage: mocks.closeNewPage,
  }),
}))

vi.mock('../router', () => ({
  Link: ({ to, className, children }: { to: string; className?: string; children: ReactNode }) => <a href={to} className={className}>{children}</a>,
  useRouter: () => ({ pathname: mocks.pathname, navigate: mocks.navigate }),
}))

vi.mock('../assistant', () => ({
  useAssistant: () => ({
    voice: {
      listening: mocks.voiceListening,
      start: mocks.startVoice,
      stop: mocks.stopVoice,
      cancel: mocks.cancelVoice,
    },
  }),
}))

vi.mock('./assistant/AssistantPanel', () => ({ AssistantPanel: () => <div>Assistant panel</div> }))
vi.mock('./NewPageDialog', () => ({ NewPageDialog: ({ initialKind }: { initialKind: string }) => <div>New {initialKind}</div> }))

beforeEach(() => {
  mocks.assistantOpen = false
  mocks.voiceListening = false
  mocks.pathname = '/concepts'
  mocks.newPageKind = null
  for (const mock of [mocks.setAssistantOpen, mocks.closeNewPage, mocks.logout, mocks.startVoice, mocks.stopVoice, mocks.cancelVoice, mocks.navigate]) mock.mockReset()
})

function renderLayout() {
  return render(<Layout><p>Obsah</p></Layout>, { wrapper: I18nProvider })
}

it('uses the supplied assistant-stars asset in both launcher states', () => {
  mocks.assistantOpen = false
  const { rerender } = renderLayout()

  const closedLauncher = screen.getByRole('button', { name: 'Otvoriť asistenta' })
  expect(closedLauncher.querySelector('img')).toHaveAttribute('src', '/assistant-stars.svg')
  expect(closedLauncher.querySelector('img')).toHaveAttribute('aria-hidden', 'true')
  expect(closedLauncher.querySelector('svg')).not.toBeInTheDocument()
  fireEvent.click(closedLauncher)
  expect(mocks.cancelVoice).not.toHaveBeenCalled()
  expect(mocks.setAssistantOpen).toHaveBeenCalledWith(true)

  mocks.assistantOpen = true
  rerender(<Layout><p>Obsah</p></Layout>)

  const openLauncher = screen.getByRole('button', { name: 'Zavrieť asistenta' })
  expect(openLauncher.querySelector('img')).toHaveAttribute('src', '/assistant-stars.svg')
  expect(openLauncher.querySelector('svg')).not.toBeInTheDocument()
})

it('keeps only Concepts and Features as primary content navigation', () => {
  renderLayout()

  expect(screen.getByRole('link', { name: 'Koncepty' })).toHaveAttribute('href', '/concepts')
  expect(screen.getByRole('link', { name: 'Funkcie' })).toHaveAttribute('href', '/features')
  expect(screen.queryByRole('link', { name: 'Drafty' })).not.toBeInTheDocument()
})

it('places the language switch beside the viki wordmark in the sidebar header', () => {
  const view = renderLayout()

  const header = view.container.querySelector('.sidebar-brand')
  expect(header).toContainElement(screen.getByRole('link', { name: 'viki' }))
  expect(header).toContainElement(screen.getByRole('switch', { name: 'Jazyk' }))
})

it('navigates to features and switches the complete navigation to English', async () => {
  const user = userEvent.setup()
  renderLayout()

  expect(screen.getByRole('link', { name: 'Funkcie' })).toHaveAttribute('href', '/features')
  const languageToggle = screen.getByRole('switch', { name: 'Jazyk' })
  expect(languageToggle).toHaveAttribute('aria-checked', 'false')
  await user.click(languageToggle)

  expect(screen.getByRole('link', { name: 'Concepts' })).toHaveAttribute('href', '/concepts')
  expect(screen.getByRole('link', { name: 'Features' })).toHaveAttribute('href', '/features')
  expect(screen.queryByRole('link', { name: 'Drafts' })).not.toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Change history' })).toHaveAttribute('href', '/audit')
  expect(screen.getByRole('switch', { name: 'Language' })).toHaveAttribute('aria-checked', 'true')

  await user.click(screen.getByRole('switch', { name: 'Language' }))
  expect(screen.getByRole('link', { name: 'Koncepty' })).toHaveAttribute('href', '/concepts')
  expect(screen.getByRole('switch', { name: 'Jazyk' })).toHaveAttribute('aria-checked', 'false')
})

it('toggles Slovak dictation app-wide and lets Escape cancel it before closing the assistant', () => {
  mocks.assistantOpen = false
  mocks.voiceListening = false
  const { rerender } = renderLayout()

  fireEvent.keyDown(window, { key: 'm', metaKey: true, shiftKey: true })
  expect(mocks.setAssistantOpen).toHaveBeenCalledWith(true)
  expect(mocks.startVoice).toHaveBeenCalledOnce()

  mocks.assistantOpen = true
  mocks.voiceListening = true
  rerender(<Layout><p>Obsah</p></Layout>)
  fireEvent.keyDown(window, { key: 'M', ctrlKey: true, shiftKey: true })
  expect(mocks.stopVoice).toHaveBeenCalledOnce()

  fireEvent.keyDown(window, { key: 'Escape' })
  expect(mocks.cancelVoice).toHaveBeenCalledOnce()
  expect(mocks.setAssistantOpen).not.toHaveBeenCalledWith(false)
})

it('opens and closes the mobile navigation and follows search controls', async () => {
  const user = userEvent.setup()
  const view = renderLayout()

  await user.click(screen.getByRole('button', { name: 'Otvoriť navigáciu' }))
  expect(view.container.querySelector('.sidebar')).toHaveClass('mobile-open')
  await user.click(screen.getByRole('button', { name: 'Zavrieť navigáciu' }))
  expect(view.container.querySelector('.sidebar')).not.toHaveClass('mobile-open')

  await user.click(screen.getByRole('button', { name: 'Otvoriť navigáciu' }))
  await user.click(screen.getByRole('button', { name: 'Zavrieť' }))
  expect(view.container.querySelector('.sidebar')).not.toHaveClass('mobile-open')
  await user.click(screen.getByRole('button', { name: 'Otvoriť navigáciu' }))
  await user.click(screen.getByRole('link', { name: 'Funkcie' }))
  expect(view.container.querySelector('.sidebar')).not.toHaveClass('mobile-open')

  await user.click(screen.getByRole('button', { name: /Hľadať/ }))
  expect(mocks.navigate).toHaveBeenCalledWith('/search')
  fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
  expect(mocks.navigate).toHaveBeenLastCalledWith('/search')
})

it('closes transient UI on Escape when dictation is idle', () => {
  renderLayout()
  fireEvent.keyDown(window, { key: 'Escape' })

  expect(mocks.setAssistantOpen).toHaveBeenCalledWith(false)
  expect(mocks.closeNewPage).toHaveBeenCalledOnce()
  expect(mocks.cancelVoice).not.toHaveBeenCalled()
})

it('toggles the assistant launcher, cancels active dictation, logs out, and renders overlays', async () => {
  const user = userEvent.setup()
  mocks.assistantOpen = true
  mocks.voiceListening = true
  mocks.newPageKind = 'feature'
  renderLayout()

  expect(screen.getByText('Assistant panel')).toBeVisible()
  expect(screen.getByText('New feature')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Zavrieť asistenta' }))
  expect(mocks.cancelVoice).toHaveBeenCalledOnce()
  expect(mocks.setAssistantOpen).toHaveBeenCalledWith(false)
  await user.click(screen.getByRole('button', { name: 'Odhlásiť' }))
  expect(mocks.logout).toHaveBeenCalledOnce()
})

it('marks each current navigation destination active', () => {
  for (const [pathname, name] of [['/', 'Koncepty'], ['/features', 'Funkcie'], ['/audit', 'História zmien']] as const) {
    mocks.pathname = pathname
    const view = renderLayout()
    expect(screen.getByRole('link', { name })).toHaveClass('active')
    view.unmount()
  }
})
