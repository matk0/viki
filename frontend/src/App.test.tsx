import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, expect, it, vi } from 'vitest'
import { App } from './App'

const state = vi.hoisted(() => ({ loading: false, user: null as null | { id: string }, pathname: '/' }))

vi.mock('./auth', () => ({ useAuth: () => ({ user: state.user, loading: state.loading }) }))
vi.mock('./i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('./router', () => ({ useRouter: () => ({ pathname: state.pathname }) }))
vi.mock('./workspace', () => ({ WorkspaceProvider: ({ children }: { children: ReactNode }) => <>{children}</> }))
vi.mock('./assistant', () => ({ AssistantProvider: ({ children }: { children: ReactNode }) => <>{children}</> }))
vi.mock('./components/Layout', () => ({ Layout: ({ children }: { children: ReactNode }) => <main>{children}</main> }))
vi.mock('./pages/LoginPage', () => ({ LoginPage: () => <p>login</p> }))
vi.mock('./pages/LibraryPage', () => ({ LibraryPage: ({ kind }: { kind: string }) => <p>library-{kind}</p> }))
vi.mock('./pages/PagePage', () => ({ PagePage: ({ pageId }: { pageId: string }) => <p>page-{pageId}</p> }))
vi.mock('./pages/AuditPage', () => ({ AuditPage: () => <p>audit</p> }))
vi.mock('./pages/SearchPage', () => ({ SearchPage: () => <p>search</p> }))
vi.mock('./pages/NotFoundPage', () => ({ NotFoundPage: () => <p>not-found</p> }))

beforeEach(() => {
  state.loading = false
  state.user = null
  state.pathname = '/'
})

it('shows bootstrap loading and login states', () => {
  state.loading = true
  const view = render(<App />)
  expect(screen.getByText('app.loading')).toBeInTheDocument()
  state.loading = false
  view.rerender(<App />)
  expect(screen.getByText('login')).toBeInTheDocument()
})

it('routes authenticated users across every workspace screen', () => {
  state.user = { id: 'user-1' }
  const view = render(<App />)
  const routes: Array<[string, string]> = [
    ['/', 'library-concept'],
    ['/concepts', 'library-concept'],
    ['/features', 'library-feature'],
    ['/audit', 'audit'],
    ['/search', 'search'],
    ['/drafts', 'not-found'],
    ['/drafts/proposal%201', 'not-found'],
    ['/page/page%201', 'page-page 1'],
    ['/unknown', 'not-found'],
  ]
  for (const [pathname, expected] of routes) {
    state.pathname = pathname
    view.rerender(<App />)
    expect(screen.getByText(expected)).toBeInTheDocument()
  }
})
