import { Box, FileCheck2, Pencil, Workflow } from 'lucide-react'
import type { Page } from '../api/types'

export function PageIcon({ page, size = 17, draft = false }: { page: Pick<Page, 'kind' | 'primitiveKind'>; size?: number; draft?: boolean }) {
  if (draft) return <Pencil size={size} />
  if (page.kind === 'scenario') return <Workflow size={size} />
  if (page.kind === 'subscenario') return <FileCheck2 size={size} />
  return <Box size={size} />
}
