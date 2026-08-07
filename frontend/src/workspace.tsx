import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { api } from './api/client'
import type { Page, PageKind } from './api/types'

interface WorkspaceValue {
  pages: Page[]
  loadingPages: boolean
  reloadPages: () => Promise<void>
  assistantOpen: boolean
  setAssistantOpen: (open: boolean) => void
  newPageKind: PageKind | null
  newPageParentId?: string
  openNewPage: (kind: PageKind, parentId?: string) => void
  closeNewPage: () => void
}

const WorkspaceContext = createContext<WorkspaceValue | null>(null)

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const [pages, setPages] = useState<Page[]>([])
  const [loadingPages, setLoadingPages] = useState(true)
  const [assistantOpen, setAssistantOpen] = useState(false)
  const [newPageKind, setNewPageKind] = useState<PageKind | null>(null)
  const [newPageParentId, setNewPageParentId] = useState<string>()
  const openNewPage = useCallback((kind: PageKind, parentId?: string) => {
    setNewPageKind(kind)
    setNewPageParentId(parentId)
  }, [])
  const closeNewPage = useCallback(() => {
    setNewPageKind(null)
    setNewPageParentId(undefined)
  }, [])
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
    newPageKind, newPageParentId, openNewPage, closeNewPage,
  }), [pages, loadingPages, reloadPages, assistantOpen, newPageKind, newPageParentId, openNewPage, closeNewPage])
  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>
}

export function useWorkspace(): WorkspaceValue {
  const value = useContext(WorkspaceContext)
  if (!value) throw new Error('useWorkspace must be used inside WorkspaceProvider')
  return value
}
