import { useAuth } from './auth'
import { WorkspaceProvider } from './workspace'
import { useRouter } from './router'
import { LoginPage } from './pages/LoginPage'
import { Layout } from './components/Layout'
import { LibraryPage } from './pages/LibraryPage'
import { PagePage } from './pages/PagePage'
import { AuditPage } from './pages/AuditPage'
import { SearchPage } from './pages/SearchPage'
import { DraftPage } from './pages/DraftPage'
import { DraftsPage } from './pages/DraftsPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { AssistantProvider } from './assistant'

export function App() {
  const { user, loading } = useAuth()
  if (loading) return <div className="app-loading"><div className="brand-mark">v</div><span>Načítavam viki…</span></div>
  if (!user) return <LoginPage />
  return (
    <WorkspaceProvider>
      <AssistantProvider>
        <Layout><CurrentRoute /></Layout>
      </AssistantProvider>
    </WorkspaceProvider>
  )
}

function CurrentRoute() {
  const { pathname } = useRouter()
  if (pathname === '/') return <LibraryPage kind="primitive" />
  if (pathname === '/primitives') return <LibraryPage kind="primitive" />
  if (pathname === '/scenarios') return <LibraryPage kind="scenario" />
  if (pathname === '/audit') return <AuditPage />
  if (pathname === '/search') return <SearchPage />
  if (pathname === '/drafts') return <DraftsPage />
  if (pathname.startsWith('/drafts/')) return <DraftPage proposalId={decodeURIComponent(pathname.slice('/drafts/'.length))} />
  if (pathname.startsWith('/page/')) return <PagePage pageId={decodeURIComponent(pathname.slice('/page/'.length))} />
  return <NotFoundPage />
}
