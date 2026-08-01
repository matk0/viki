import { render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { AuditPage } from './AuditPage'

const mocks = vi.hoisted(() => ({ audit: vi.fn() }))
vi.mock('../api/client', () => ({ api: mocks }))

beforeEach(() => { mocks.audit.mockReset() })

it('shows loading then every known audit label and unknown actions', async () => {
  const actions = [
    'page.created', 'revision.saved', 'revision.published', 'comment.created', 'comment.resolved', 'vote.recorded',
    'assistant.drafts_created', 'assistant.proposal_created', 'assistant.proposal_published', 'assistant.proposal.discarded',
    'assistant.proposal_discarded', 'ai.drafts_created', 'custom.action',
  ]
  mocks.audit.mockResolvedValue({ events: actions.map((action, index) => ({
    id: `event-${index}`, action, entityType: 'page', entityId: index === 0 ? '1234567890' : '',
    actor: index === 1 ? undefined : { id: 'user-1', displayName: 'Matej', email: 'a@b.c', createdAt: '2026-07-31T10:00:00Z' },
    createdAt: '2026-07-31T10:00:00Z', metadata: {},
  })) })
  render(<AuditPage />)
  expect(document.querySelector('.spinner')).toBeInTheDocument()
  expect(await screen.findByText('custom.action')).toBeInTheDocument()
  expect(screen.getByText(/12345678/)).toBeInTheDocument()
  expect(screen.getByText(/Systém/)).toBeInTheDocument()
})

it('renders API failures as an empty state', async () => {
  mocks.audit.mockReturnValue({
    then: () => ({ catch: (reject: (error: Error) => void) => reject(new Error('database offline')) }),
  })
  render(<AuditPage />)
  expect(await screen.findByText('database offline')).toBeInTheDocument()
  expect(screen.getByText('História sa nedá načítať')).toBeInTheDocument()
})
