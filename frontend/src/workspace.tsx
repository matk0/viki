import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { api } from './api/client'
import type { Page } from './api/types'

interface WorkspaceValue {
  pages: Page[]
  loadingPages: boolean
  reloadPages: () => Promise<void>
  assistantOpen: boolean
  setAssistantOpen: (open: boolean) => void
  newPageKind: 'concept' | 'feature' | null
  openNewPage: (kind: 'concept' | 'feature') => void
  closeNewPage: () => void
}

const WorkspaceContext = createContext<WorkspaceValue | null>(null)

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const [pages, setPages] = useState<Page[]>([])
  const [loadingPages, setLoadingPages] = useState(true)
  const [assistantOpen, setAssistantOpen] = useState(false)
  const [newPageKind, setNewPageKind] = useState<'concept' | 'feature' | null>(null)
  const openNewPage = useCallback((kind: 'concept' | 'feature') => setNewPageKind(kind), [])
  const closeNewPage = useCallback(() => setNewPageKind(null), [])
  const reloadPages = useCallback(async () => {
    try {
      const result = await api.pages()
      setPages(result.pages)
    } catch {
      return
    } finally {
      setLoadingPages(false)
    }
  }, [])
  useEffect(() => { void reloadPages() }, [reloadPages])
  const value = useMemo(() => ({
    pages, loadingPages, reloadPages,
    assistantOpen, setAssistantOpen,
    newPageKind, openNewPage, closeNewPage,
  }), [pages, loadingPages, reloadPages, assistantOpen, newPageKind, openNewPage, closeNewPage])
  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>
}

export function useWorkspace(): WorkspaceValue {
  const value = useContext(WorkspaceContext)
  if (!value) throw new Error('useWorkspace must be used inside WorkspaceProvider')
  return value
}
