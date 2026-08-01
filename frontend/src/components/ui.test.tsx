import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import type { Page } from '../api/types'
import { EmptyState, formatDate, kindLabel, Markdown, Spinner, StatusBadge } from './ui'

const page = (overrides: Partial<Page> = {}): Page => ({
  id: 'page', kind: 'concept', conceptKind: 'noun', slug: 'page', title: 'Page', accepted: false, hasDraft: false,
  unresolvedRejections: 0, createdAt: '', updatedAt: '', ...overrides,
})

it('renders spinner and empty-state defaults and optional content', () => {
  const { rerender } = render(<Spinner />)
  expect(screen.getByText('Načítavam…')).toBeVisible()
  rerender(<EmptyState icon={<span>icon</span>} title="Empty" body="Nothing" action={<button>Act</button>} />)
  expect(screen.getByText('icon')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Act' })).toBeVisible()
})

it('resolves every explicit and page-derived status', () => {
  const { container, rerender } = render(<StatusBadge status="accepted" />)
  expect(container.firstChild).toHaveClass('accepted')
  rerender(<StatusBadge status="superseded" />)
  expect(container.firstChild).toHaveTextContent('Nahradené')
  rerender(<StatusBadge page={page({ unresolvedRejections: 1, accepted: true, hasDraft: true })} />)
  expect(container.firstChild).toHaveClass('rejected')
  rerender(<StatusBadge page={page({ hasDraft: true })} />)
  expect(container.firstChild).toHaveClass('draft')
  rerender(<StatusBadge page={page({ accepted: true })} />)
  expect(container.firstChild).toHaveClass('accepted')
  rerender(<StatusBadge page={page()} />)
  expect(container.firstChild).toHaveClass('draft')
  rerender(<StatusBadge status="draft" />)
  expect(container.firstChild).toHaveTextContent('Draft')
  rerender(<StatusBadge />)
  expect(container.firstChild).toHaveClass('draft')
})

it('links inflected accent-insensitive terms inline and preserves non-text markdown children', () => {
  const { container } = render(<Markdown inlineLinks={[
    { href: '/short', label: 'Zmluva', className: 'short' },
    { href: '/long', label: 'Zmluva pre domácnosť', className: 'long' },
    { href: '/number', label: 'Číslo účtu' },
    { href: '/code', label: 'C++' },
  ]}>{'Pred **dôležitým** podpisom zmluvy pre domácnosť over cislo uctu v C++. Bez zhody.'}</Markdown>)

  expect(screen.getByText('dôležitým').tagName).toBe('STRONG')
  expect(container.querySelector('a.long')).toHaveAttribute('href', '/long')
  expect(container.querySelector('a.short')).toBeNull()
  expect(screen.getByRole('link', { name: 'cislo uctu' })).toHaveAttribute('href', '/number')
  expect(screen.getByRole('link', { name: 'C++' })).toHaveAttribute('href', '/code')
  expect(screen.getByText(/Bez zhody/)).toBeVisible()
})

it('renders plain markdown and formats dates and every page kind in both locales', () => {
  render(<Markdown>Plain text</Markdown>)
  expect(screen.getByText('Plain text')).toBeVisible()
  expect(formatDate('2026-07-31T10:00:00Z', false, 'en')).toMatch(/31/)
  expect(formatDate('2026-07-31T10:00:00Z', true, 'sk')).toMatch(/2026/)
  expect(kindLabel(page({ kind: 'feature', conceptKind: undefined }), 'en')).toBe('Feature')
  expect(kindLabel(page({ kind: 'scenario', conceptKind: undefined }))).toBe('Scenár')
  expect(kindLabel(page())).toBe('Koncept')
})
