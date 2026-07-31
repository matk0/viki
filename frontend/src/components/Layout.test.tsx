import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect, it, vi } from 'vitest'
import { Layout } from './Layout'

const mocks = vi.hoisted(() => ({
  assistantOpen: false,
  setAssistantOpen: vi.fn(),
  closeNewPage: vi.fn(),
  logout: vi.fn(),
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
    newPageKind: null,
    closeNewPage: mocks.closeNewPage,
  }),
}))

vi.mock('../router', () => ({
  Link: ({ to, className, children }: { to: string; className?: string; children: ReactNode }) => <a href={to} className={className}>{children}</a>,
  useRouter: () => ({ pathname: '/primitives', navigate: vi.fn() }),
}))

vi.mock('./assistant/AssistantPanel', () => ({ AssistantPanel: () => null }))
vi.mock('./NewPageDialog', () => ({ NewPageDialog: () => null }))

it('uses only one single-star icon for the assistant launcher in both states', () => {
  mocks.assistantOpen = false
  const { rerender } = render(<Layout><p>Obsah</p></Layout>)

  const closedLauncher = screen.getByRole('button', { name: 'Otvoriť asistenta' })
  expect(closedLauncher.querySelectorAll('svg')).toHaveLength(1)
  expect(closedLauncher.querySelector('.lucide-star')).toBeInTheDocument()
  expect(closedLauncher.querySelector('.lucide-sparkles')).not.toBeInTheDocument()

  mocks.assistantOpen = true
  rerender(<Layout><p>Obsah</p></Layout>)

  const openLauncher = screen.getByRole('button', { name: 'Zavrieť asistenta' })
  expect(openLauncher.querySelectorAll('svg')).toHaveLength(1)
  expect(openLauncher.querySelector('.lucide-star')).toBeInTheDocument()
  expect(openLauncher.querySelector('.lucide-x')).not.toBeInTheDocument()
})
