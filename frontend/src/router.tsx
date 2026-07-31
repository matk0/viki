import { createContext, type MouseEvent, type ReactNode, useContext, useEffect, useMemo, useState } from 'react'

interface RouterValue {
  location: string
  pathname: string
  search: URLSearchParams
  navigate: (to: string, replace?: boolean) => void
}

const RouterContext = createContext<RouterValue | null>(null)

export function Router({ children }: { children: ReactNode }) {
  const [location, setLocation] = useState(() => window.location.pathname + window.location.search)
  useEffect(() => {
    const onPopState = () => setLocation(window.location.pathname + window.location.search)
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])
  const value = useMemo<RouterValue>(() => {
    const url = new URL(location, window.location.origin)
    return {
      location,
      pathname: url.pathname,
      search: url.searchParams,
      navigate: (to, replace = false) => {
        if (replace) window.history.replaceState({}, '', to)
        else window.history.pushState({}, '', to)
        setLocation(window.location.pathname + window.location.search)
        window.scrollTo({ top: 0, behavior: 'instant' })
      },
    }
  }, [location])
  return <RouterContext.Provider value={value}>{children}</RouterContext.Provider>
}

export function useRouter(): RouterValue {
  const value = useContext(RouterContext)
  if (!value) throw new Error('useRouter must be used inside Router')
  return value
}

export function Link({ to, children, className, ariaLabel }: { to: string; children: ReactNode; className?: string; ariaLabel?: string }) {
  const { navigate } = useRouter()
  const onClick = (event: MouseEvent<HTMLAnchorElement>) => {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
    event.preventDefault()
    navigate(to)
  }
  return <a href={to} onClick={onClick} className={className} aria-label={ariaLabel}>{children}</a>
}
