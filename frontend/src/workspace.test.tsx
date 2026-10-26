import { renderHook, waitFor } from '@testing-library/react'
import { act, type ReactNode } from 'react'
import { beforeEach, expect, it, vi } from 'vitest'
import { useWorkspace, WorkspaceProvider } from './workspace'

const mocks = vi.hoisted(() => ({ pages: vi.fn(), workspaceSyncStatus: vi.fn() }))
vi.mock('./api/client', () => ({ api: mocks }))

function wrapper({ children }: { children: ReactNode }) {
  return <WorkspaceProvider>{children}</WorkspaceProvider>
}

beforeEach(() => { mocks.pages.mockReset(); mocks.workspaceSyncStatus.mockResolvedValue({mode:'human_governed', pageCount:0, scenarioCount:0, stale:false}) })

it('loads and reloads pages while controlling assistant and new-page state', async () => {
  mocks.pages
    .mockResolvedValueOnce({ pages: [{ id: 'page-1', title: 'First' }] })
    .mockResolvedValueOnce({ pages: [{ id: 'page-2', title: 'Second' }] })
  const { result } = renderHook(() => useWorkspace(), { wrapper })
  expect(result.current.loadingPages).toBe(true)
  await waitFor(() => expect(result.current.pages[0]?.id).toBe('page-1'))
  expect(result.current.loadingPages).toBe(false)

  act(() => result.current.setAssistantOpen(true))
  expect(result.current.assistantOpen).toBe(true)
  act(() => result.current.openNewPage('feature'))
  expect(result.current.newPageKind).toBe('feature')
  act(() => result.current.openNewPage('scenario', 'feature-1'))
  expect(result.current.newPageKind).toBe('scenario')
  expect(result.current.newPageParentId).toBe('feature-1')
  act(() => result.current.closeNewPage())
  expect(result.current.newPageKind).toBeNull()
  expect(result.current.newPageParentId).toBeUndefined()
  await act(() => result.current.reloadPages())
  expect(result.current.pages[0]?.id).toBe('page-2')
})

it('clears loading even when page loading fails', async () => {
  mocks.pages.mockResolvedValueOnce({ pages: [] })
  const { result } = renderHook(() => useWorkspace(), { wrapper })
  await waitFor(() => expect(result.current.loadingPages).toBe(false))
  mocks.pages.mockRejectedValueOnce(new Error('offline'))
  await act(() => result.current.reloadPages())
  expect(result.current.pages).toEqual([])
})

it('requires the provider', () => {
  expect(() => renderHook(() => useWorkspace())).toThrow('useWorkspace must be used inside WorkspaceProvider')
})

it('enforces Git-managed controls and preserves the last good catalogue on failed refresh', async () => {
 mocks.workspaceSyncStatus.mockResolvedValue({mode:'git_managed',pageCount:1,scenarioCount:0,stale:false})
 mocks.pages.mockResolvedValue({pages:[{id:'canonical',title:'Canonical'}]})
 const {result}=renderHook(()=>useWorkspace(),{wrapper})
 await waitFor(()=>expect(result.current.pages[0]?.id).toBe('canonical'))
 expect(result.current.readOnly).toBe(true)
 act(()=>result.current.openNewPage('concept'))
 expect(result.current.newPageKind).toBeNull()
 mocks.workspaceSyncStatus.mockRejectedValueOnce(new Error('offline'))
 await act(()=>result.current.reloadPages())
 expect(result.current.pages[0]?.id).toBe('canonical')
 expect(result.current.syncError).toContain('could not be checked')
})

it('polls automatically and ignores success/failure from superseded in-flight refreshes', async () => {
 vi.useFakeTimers()
 const pending: {resolve:(value:{pages:never[]})=>void,reject:(error:Error)=>void}[]=[]
 mocks.pages.mockImplementation(()=>new Promise<{pages:never[]}>((resolve,reject)=>pending.push({resolve,reject})))
 const {result,unmount}=renderHook(()=>useWorkspace(),{wrapper})
 await act(async()=>{await vi.advanceTimersByTimeAsync(30000)})
 expect(pending).toHaveLength(2)
 await act(async()=>pending[0].resolve({pages:[]}))
 expect(result.current.loadingPages).toBe(true)
 await act(async()=>{await vi.advanceTimersByTimeAsync(30000)})
 expect(pending).toHaveLength(3)
 await act(async()=>pending[1].reject(new Error('old offline')))
 expect(result.current.syncError).toBe('')
 await act(async()=>pending[2].resolve({pages:[]}))
 expect(result.current.loadingPages).toBe(false)
 unmount();vi.useRealTimers()
})
