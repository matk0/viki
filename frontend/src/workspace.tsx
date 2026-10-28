import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { api } from './api/client'
import { useI18n } from './i18n'
import { localizedPage } from './localization'
import type { Page, PageKind, WorkspaceSyncStatus } from './api/types'

interface WorkspaceValue {
 syncStatus: WorkspaceSyncStatus | null
 syncError: string
 readOnly: boolean
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
 const { locale } = useI18n()
  const loadSequence = useRef(0)
  const [syncStatus, setSyncStatus] = useState<WorkspaceSyncStatus | null>(null)
  const [syncError, setSyncError] = useState('')
  const readOnly = syncStatus === null || syncStatus.mode === 'git_managed' || Boolean(syncError)
  const [pages, setPages] = useState<Page[]>([])
  const [loadingPages, setLoadingPages] = useState(true)
  const [assistantOpen, setAssistantOpen] = useState(false)
  const [newPageKind, setNewPageKind] = useState<PageKind | null>(null)
  const [newPageParentId, setNewPageParentId] = useState<string>()
  const openNewPage = useCallback((kind: PageKind, parentId?: string) => {
    if (readOnly) return
    setNewPageKind(kind)
    setNewPageParentId(parentId)
  }, [readOnly])
  const closeNewPage = useCallback(() => {
    setNewPageKind(null)
    setNewPageParentId(undefined)
  }, [])
  const reloadPages = useCallback(async () => {
    const sequence = ++loadSequence.current
    try {
      const [status, result] = await Promise.all([api.workspaceSyncStatus(), api.pages()])
      if (sequence !== loadSequence.current) return
      setSyncStatus(status)
      setSyncError('')
      setPages(result.pages)
    } catch {
      if (sequence !== loadSequence.current) return
      setSyncError('Workspace authority or catalogue could not be checked.')
      return
    } finally {
      if (sequence === loadSequence.current) setLoadingPages(false)
    }
  }, [])
  useEffect(() => { void reloadPages(); const timer = window.setInterval(() => void reloadPages(), 30000); return () => {loadSequence.current++; window.clearInterval(timer)} }, [reloadPages])
  const displayedPages = useMemo(() => pages.map((page) => localizedPage(page, locale)), [pages, locale])
  const value = useMemo(() => ({
    syncStatus, syncError, readOnly, pages: displayedPages, loadingPages, reloadPages,
    assistantOpen, setAssistantOpen,
    newPageKind, newPageParentId, openNewPage, closeNewPage,
  }), [syncStatus, syncError, readOnly, displayedPages, loadingPages, reloadPages, assistantOpen, newPageKind, newPageParentId, openNewPage, closeNewPage])
  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>
}

export function useWorkspace(): WorkspaceValue {
  const value = useContext(WorkspaceContext)
  if (!value) throw new Error('useWorkspace must be used inside WorkspaceProvider')
  return value
}
