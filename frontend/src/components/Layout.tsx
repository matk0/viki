import { useEffect, useState, type ReactNode } from 'react';
import {
  Box,
  Clock3,
  Files,
  LogOut,
  Menu,
  Search,
  Star,
  Workflow,
  X,
} from 'lucide-react';
import { useAuth } from '../auth';
import { Link, useRouter } from '../router';
import { useWorkspace } from '../workspace';
import { AssistantPanel } from './assistant/AssistantPanel';
import { NewPageDialog } from './NewPageDialog';

export function Layout({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const {
    assistantOpen,
    setAssistantOpen,
    newPageKind,
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
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        navigate('/search');
      }
      if (event.key === 'Escape') {
        setAssistantOpen(false);
        setMobileOpen(false);
        closeNewPage();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [navigate, setAssistantOpen, closeNewPage]);
  return (
    <div className={`app-shell ${assistantVisible ? 'assistant-visible' : ''}`}>
      <button
        className="mobile-menu"
        onClick={() => setMobileOpen(true)}
        aria-label="Otvoriť navigáciu"
      >
        <Menu size={19} />
      </button>
      {mobileOpen && (
        <button
          className="sidebar-scrim"
          onClick={() => setMobileOpen(false)}
          aria-label="Zavrieť navigáciu"
        />
      )}
      <aside className={`sidebar ${mobileOpen ? 'mobile-open' : ''}`}>
        <div className="sidebar-brand">
          <Link to="/primitives" className="wordmark">
            viki
          </Link>
          <button
            className="icon-button mobile-only"
            onClick={() => setMobileOpen(false)}
            aria-label="Zavrieť"
          >
            <X size={17} />
          </button>
        </div>
        <button className="sidebar-search" onClick={() => go('/search')}>
          <Search size={15} />
          <span>Hľadať</span>
          <kbd>⌘ K</kbd>
        </button>
        <nav
          className="sidebar-nav"
          aria-label="Hlavná navigácia"
          onClick={() => setMobileOpen(false)}
        >
          <NavItem
            to="/primitives"
            active={pathname === '/' || pathname === '/primitives'}
            icon={<Box size={16} />}
          >
            Pojmy
          </NavItem>
          <NavItem
            to="/scenarios"
            active={pathname === '/scenarios'}
            icon={<Workflow size={16} />}
          >
            Scenáre
          </NavItem>
          <NavItem
            to="/drafts"
            active={pathname.startsWith('/drafts')}
            icon={<Files size={16} />}
          >
            Drafty
          </NavItem>
          <NavItem
            to="/audit"
            active={pathname === '/audit'}
            icon={<Clock3 size={16} />}
          >
            História zmien
          </NavItem>
        </nav>
        <div className="sidebar-user">
          <div className="avatar">{user?.displayName.slice(0, 1)}</div>
          <div>
            <strong>{user?.displayName}</strong>
            <span>{user?.email}</span>
          </div>
          <button
            className="icon-button"
            onClick={() => void logout()}
            aria-label="Odhlásiť"
          >
            <LogOut size={16} />
          </button>
        </div>
      </aside>
      <main className="main-content">{children}</main>
      <button
        className={`assistant-fab ${assistantVisible ? 'active' : ''}`}
        onClick={() => setAssistantOpen(!assistantOpen)}
        aria-label={assistantOpen ? 'Zavrieť asistenta' : 'Otvoriť asistenta'}
      >
        <Star size={19} />
      </button>
      {assistantVisible && (
        <aside className="assistant-drawer">
          <AssistantPanel />
        </aside>
      )}
      {newPageKind && <NewPageDialog initialKind={newPageKind} onClose={closeNewPage} />}
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
