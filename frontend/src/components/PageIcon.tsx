import { Box, FileCheckCorner, Pencil, Workflow } from 'lucide-react'
import type { Page } from '../api/types'

export function PageIcon({ page, size = 17, draft = false }: { page: Pick<Page, 'kind' | 'conceptKind'>; size?: number; draft?: boolean }) {
  if (draft) return <Pencil size={size} />
  if (page.kind === 'feature') return <Workflow size={size} />
  if (page.kind === 'scenario') return <FileCheckCorner size={size} />
  return <Box size={size} />
}
