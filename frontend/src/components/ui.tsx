import { Children, type ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { Page, RevisionStatus } from '../api/types'
import { translate, useI18n, type Locale, type Translate } from '../i18n'

export function Spinner({ label }: { label?: string }) {
  const { t } = useI18n()
  return <div className="loading-state"><span className="spinner" /><span>{label ?? t('common.loading')}</span></div>
}

export function EmptyState({ icon, title, body, action }: { icon?: ReactNode; title: string; body: string; action?: ReactNode }) {
  return <div className="empty-state">{icon}<h3>{title}</h3><p>{body}</p>{action}</div>
}

export function StatusBadge({ status, page }: { status?: RevisionStatus; page?: Page }) {
  const { t } = useI18n()
  const resolved: RevisionStatus = status ?? (page?.hasDraft ? 'draft' : page?.approved ? 'approved' : 'draft')
  return <span className={`status-badge ${resolved}`}>{statusLabel(resolved, t)}</span>
}

export function statusLabel(status: RevisionStatus, t: Translate): string {
  return status === 'approved' ? t('status.approved') : status === 'superseded' ? t('status.superseded') : t('status.draft')
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
    return `${accentInsensitivePattern(stem)}[\\p{L}\\p{M}]*`
  })
  return new RegExp(`(?<![\\p{L}\\p{M}\\p{N}_])${fragments.join('[\\s-]+')}(?![\\p{L}\\p{M}\\p{N}_])`, 'iu')
}

function accentInsensitivePattern(value: string): string {
  const variants: Record<string, string> = {
    a: 'aáä', c: 'cč', d: 'dď', e: 'eé', i: 'ií', l: 'lĺľ', n: 'nň',
    o: 'oóô', r: 'rŕ', s: 'sš', t: 'tť', u: 'uú', y: 'yý', z: 'zž',
  }
  return [...value].map((character) => {
    const base = character.normalize('NFD').replace(/\p{M}/gu, '')
    return variants[base] ? `[${variants[base]}]` : escapeRegExp(character)
  }).join('')
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export function formatDate(value: string, includeTime = true, locale: Locale = 'sk'): string {
  return new Intl.DateTimeFormat(locale === 'en' ? 'en-GB' : 'sk-SK', {
    day: 'numeric', month: 'short', year: 'numeric',
    ...(includeTime ? { hour: '2-digit', minute: '2-digit' } : {}),
  }).format(new Date(value))
}

export function kindLabel(page: Pick<Page, 'kind' | 'conceptKind'>, locale: Locale = 'sk'): string {
  if (page.kind === 'feature') return translate(locale, 'kind.feature')
  if (page.kind === 'scenario') return translate(locale, 'kind.scenario')
  return translate(locale, 'kind.concept')
}
