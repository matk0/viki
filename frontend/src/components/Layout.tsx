import { useEffect, useState, type ReactNode } from 'react';
import {
  Box,
  Clock3,
  LogOut,
  Menu,
  Search,
  Workflow,
  X,
} from 'lucide-react';
import { useAuth } from '../auth';
import { useAssistant } from '../assistant';
import { Link, useRouter } from '../router';
import { useWorkspace } from '../workspace';
import { AssistantPanel } from './assistant/AssistantPanel';
import { NewPageDialog } from './NewPageDialog';
import { LanguageSwitcher, useI18n } from '../i18n';
import { isPublicDemo } from '../deployment';
import { PublicAssistantPanel } from './assistant/PublicAssistantPanel';

export function Layout({ children }: { children: ReactNode }) {
  const publicDemo = isPublicDemo();
  const { t } = useI18n();
  const { user, logout } = useAuth();
  const { voice } = useAssistant();
  const {
    assistantOpen,
    setAssistantOpen,
    newPageKind,
    newPageParentId,
    closeNewPage,
  } = useWorkspace();
  const { pathname, navigate } = useRouter();
  const assistantVisible = assistantOpen;
  const [mobileOpen, setMobileOpen] = useState(false);
  const go = (path: string) => {
    navigate(path);
    setMobileOpen(false);
  };
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.shiftKey && !event.altKey && event.key.toLowerCase() === 'm') {
        event.preventDefault();
        setAssistantOpen(true);
        if (publicDemo) return;
        if (voice.listening) voice.stop();
        else voice.start();
        return;
      }
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        navigate('/search');
      }
      if (event.key === 'Escape') {
        if (voice.listening) {
          event.preventDefault();
          voice.cancel();
          return;
        }
        setAssistantOpen(false);
        setMobileOpen(false);
        closeNewPage();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [navigate, setAssistantOpen, closeNewPage, voice.cancel, voice.listening, voice.start, voice.stop, publicDemo]);
  return (
    <div className={`app-shell ${assistantVisible ? 'assistant-visible' : ''} ${publicDemo ? 'public-demo' : ''}`}>
      <button
        className="mobile-menu"
        onClick={() => setMobileOpen(true)}
        aria-label={t('nav.open')}
      >
        <Menu size={19} />
      </button>
      {mobileOpen && (
        <button
          className="sidebar-scrim"
          onClick={() => setMobileOpen(false)}
          aria-label={t('nav.close')}
        />
      )}
      <aside className={`sidebar ${mobileOpen ? 'mobile-open' : ''}`}>
        <div className="sidebar-brand">
          <Link to="/concepts" className="wordmark">
            viki
          </Link>
          {!publicDemo && <LanguageSwitcher className="sidebar-language" />}
          <button
            className="icon-button mobile-only"
            onClick={() => setMobileOpen(false)}
            aria-label={t('common.close')}
          >
            <X size={17} />
          </button>
        </div>
        <button className="sidebar-search" onClick={() => go('/search')}>
          <Search size={15} />
          <span>{t('nav.search')}</span>
          <kbd>⌘ K</kbd>
        </button>
        <nav
          className="sidebar-nav"
          aria-label={t('nav.main')}
          onClick={() => setMobileOpen(false)}
        >
          <NavItem
            to="/concepts"
            active={pathname === '/' || pathname === '/concepts'}
            icon={<Box size={16} />}
          >
            {t('kind.concepts')}
          </NavItem>
          <NavItem
            to="/features"
            active={pathname === '/features'}
            icon={<Workflow size={16} />}
          >
            {t('kind.features')}
          </NavItem>
          {!publicDemo && <NavItem
            to="/audit"
            active={pathname === '/audit'}
            icon={<Clock3 size={16} />}
          >
            {t('nav.audit')}
          </NavItem>}
        </nav>
        {!publicDemo && <div className="sidebar-user">
          <div className="avatar">{user?.displayName.slice(0, 1)}</div>
          <div>
            <strong>{user?.displayName}</strong>
            <span>{user?.email}</span>
          </div>
          <button
            className="icon-button"
            onClick={() => void logout()}
            aria-label={t('nav.logout')}
          >
            <LogOut size={16} />
          </button>
        </div>}
      </aside>
      <main className="main-content">{children}</main>
      <button
        className={`assistant-fab ${assistantVisible ? 'active' : ''}`}
        onClick={() => {
          if (assistantOpen && voice.listening) voice.cancel();
          setAssistantOpen(!assistantOpen);
        }}
        aria-label={assistantOpen ? t('assistant.close') : t('assistant.open')}
      >
        <img src="/assistant-stars.svg" alt="" aria-hidden="true" />
      </button>
      {publicDemo ? <aside className="assistant-drawer" hidden={!assistantVisible}><PublicAssistantPanel /></aside> : assistantVisible && (
        <aside className="assistant-drawer">
          <AssistantPanel />
        </aside>
      )}
      {newPageKind && <NewPageDialog initialKind={newPageKind} parentId={newPageParentId} onClose={closeNewPage} />}
    </div>
  );
}

function NavItem({
  to,
  active,
  icon,
  children,
}: {
  to: string;
  active: boolean;
  icon: ReactNode;
  children: ReactNode;
}) {
  return (
    <Link to={to} className={`nav-item ${active ? 'active' : ''}`}>
      {icon}
      <span>{children}</span>
    </Link>
  );
}
