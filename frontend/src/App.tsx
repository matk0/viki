import { useAuth } from './auth'
import { WorkspaceProvider } from './workspace'
import { useRouter } from './router'
import { LoginPage } from './pages/LoginPage'
import { Layout } from './components/Layout'
import { LibraryPage } from './pages/LibraryPage'
import { PagePage } from './pages/PagePage'
import { AuditPage } from './pages/AuditPage'
import { SearchPage } from './pages/SearchPage'
import { AssistantTurnPage } from './pages/AssistantTurnPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { AssistantProvider } from './assistant'
import { useI18n } from './i18n'

export function App() {
  const { user, loading } = useAuth()
  const { t } = useI18n()
  if (loading) return <div className="app-loading"><div className="brand-mark">v</div><span>{t('app.loading')}</span></div>
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
  if (pathname === '/') return <LibraryPage kind="concept" />
  if (pathname === '/concepts') return <LibraryPage kind="concept" />
  if (pathname === '/features') return <LibraryPage kind="feature" />
  if (pathname === '/audit') return <AuditPage />
  if (pathname === '/search') return <SearchPage />
  if (pathname.startsWith('/assistant/turns/')) return <AssistantTurnPage turnId={decodeURIComponent(pathname.slice('/assistant/turns/'.length))} />
  if (pathname.startsWith('/page/')) return <PagePage pageId={decodeURIComponent(pathname.slice('/page/'.length))} />
  return <NotFoundPage />
}
