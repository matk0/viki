import { Children, type ReactNode } from 'react'
import { Pencil } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { Page, RevisionStatus } from '../api/types'

export function Spinner({ label = 'Načítavam…' }: { label?: string }) {
  return <div className="loading-state"><span className="spinner" /><span>{label}</span></div>
}

export function EmptyState({ icon, title, body, action }: { icon?: ReactNode; title: string; body: string; action?: ReactNode }) {
  return <div className="empty-state">{icon}<h3>{title}</h3><p>{body}</p>{action}</div>
}

export function StatusBadge({ status, page }: { status?: RevisionStatus; page?: Page }) {
  const resolved: RevisionStatus | 'rejected' = status ?? ((page?.unresolvedRejections ?? 0) > 0 ? 'rejected' : page?.hasDraft ? 'draft' : page?.accepted ? 'accepted' : 'draft')
  const label = resolved === 'accepted' ? 'Publikované' : resolved === 'rejected' ? 'Odmietnuté' : resolved === 'superseded' ? 'Nahradené' : 'Koncept'
  return <span className={`status-badge ${resolved}`}>{resolved === 'draft' ? <Pencil size={11} /> : <i />}{label}</span>
}

interface InlineMarkdownLink {
  href: string
  label: string
  className?: string
}

export function Markdown({ children, inlineLinks = [] }: { children: string; inlineLinks?: InlineMarkdownLink[] }) {
  const components = inlineLinks.length > 0
    ? { p: ({ children: paragraphChildren }: { children?: ReactNode }) => <p>{linkInlineChildren(paragraphChildren, inlineLinks)}</p> }
    : undefined
  return <div className="markdown"><ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>{children}</ReactMarkdown></div>
}

function linkInlineChildren(children: ReactNode, links: InlineMarkdownLink[]): ReactNode {
  return Children.map(children, (child) => typeof child === 'string' ? linkInlineText(child, links) : child)
}

function linkInlineText(text: string, links: InlineMarkdownLink[]): ReactNode[] {
  const patterns = links.map((link) => ({ link, pattern: inflectedTermPattern(link.label) }))
  const result: ReactNode[] = []
  let cursor = 0
  let key = 0
  while (cursor < text.length) {
    const remaining = text.slice(cursor)
    let selected: { link: InlineMarkdownLink; index: number; text: string } | null = null
    for (const candidate of patterns) {
      const match = candidate.pattern.exec(remaining)
      if (!match || match.index === undefined) continue
      if (!selected || match.index < selected.index || (match.index === selected.index && match[0].length > selected.text.length)) {
        selected = { link: candidate.link, index: match.index, text: match[0] }
      }
    }
    if (!selected) {
      result.push(remaining)
      break
    }
    if (selected.index > 0) result.push(remaining.slice(0, selected.index))
    result.push(<a className={selected.link.className} href={selected.link.href} key={`${selected.link.href}-${key++}`}>{selected.text}</a>)
    cursor += selected.index + selected.text.length
  }
  return result
}

function inflectedTermPattern(label: string): RegExp {
  const words = label.trim().split(/\s+/).filter(Boolean)
  const fragments = words.map((word) => {
    const lower = word.toLocaleLowerCase('sk-SK')
    const stem = lower.length >= 5 && /[aáäeéiíoóuúyý]$/u.test(lower) ? lower.slice(0, -1) : lower
    return `${escapeRegExp(stem)}[\\p{L}\\p{M}]*`
  })
  return new RegExp(`(?<![\\p{L}\\p{M}\\p{N}_])${fragments.join('[\\s-]+')}(?![\\p{L}\\p{M}\\p{N}_])`, 'iu')
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export function formatDate(value: string, includeTime = true): string {
  return new Intl.DateTimeFormat('sk-SK', {
    day: 'numeric', month: 'short', year: 'numeric',
    ...(includeTime ? { hour: '2-digit', minute: '2-digit' } : {}),
  }).format(new Date(value))
}

export function kindLabel(page: Pick<Page, 'kind' | 'primitiveKind'>): string {
  if (page.kind === 'scenario') return 'Scenár'
  if (page.kind === 'subscenario') return 'Podscenár'
  return 'Pojem'
}
