import { expect, it } from 'vitest'
import { localizedDetail, localizedPage, localizedResult, localizedRevision, translationPending } from './localization'
import type { Page, PageDetail, Revision } from './api/types'

const sk = { revisionId: 'r1', sourceHash: 'hash', version: 1, title: 'Zachovať odvolanie súhlasu', bodyMd: 'Zmena e-mailu neobnoví odber.', stepTexts: ['zákazník odvolal súhlas', 'odber zostáva zrušený'] }
const page = { id: 'p1', repositoryRevisionId: 'r1', title: 'Preserve withdrawal', localizations: { sk } } as Page
const revision = { id: 'r1', title: page.title, status: 'repository_baseline', bodyMd: 'Editing email does not resubscribe.', steps: [{ keyword: 'given', text: 'a customer withdrew' }, { keyword: 'then', text: 'subscription remains withdrawn' }], localizations: { sk } } as Revision

it('switches the entire reading projection while keeping original content and identities intact', () => {
  expect(localizedPage(page, 'sk').title).toBe(sk.title)
  expect(localizedPage(page, 'en')).toBe(page)
  const displayed = localizedRevision(revision, 'sk')
  expect(displayed.title).toBe(sk.title)
  expect(displayed.steps.map(step => step.text)).toEqual(sk.stepTexts)
  expect(displayed.steps.map(step => step.keyword)).toEqual(['given', 'then'])
  expect(displayed.id).toBe(revision.id)
  expect(revision.bodyMd).toBe('Editing email does not resubscribe.')
  expect(localizedRevision(revision, 'en')).toBe(revision)
  expect(localizedDetail({ page, children: [page] } as PageDetail, 'sk').children[0].title).toBe(sk.title)
  const result = { page, revisionId: 'r1', excerpt: 'English', score: 1, draft: false }
  expect(localizedResult(result, 'sk').excerpt).toBe(sk.bodyMd)
  expect(localizedResult(result, 'en').excerpt).toBe('English')
  expect(localizedResult({ ...result, page: { ...page, localizations: { sk: { ...sk, bodyMd: '' } } } }, 'sk').excerpt).toBe(sk.stepTexts.join(' '))
  expect(translationPending(revision, 'sk')).toBe(false)
})

it('never presents another revision’s translation or incomplete steps as the current specification', () => {
  const changed = { ...revision, id: 'r2' }
  expect(localizedRevision(changed, 'sk')).toBe(changed)
  expect(translationPending(changed, 'sk')).toBe(true)
  expect(translationPending(changed, 'en')).toBe(false)
  expect(translationPending({ ...changed, status: 'approved' }, 'sk')).toBe(false)
  expect(localizedPage({ ...page, repositoryRevisionId: 'r2' }, 'sk').title).toBe(page.title)
  expect(localizedPage({ ...page, localizations: undefined }, 'sk').title).toBe(page.title)
  const incomplete = { ...revision, localizations: { sk: { ...sk, stepTexts: [] } } }
  expect(localizedRevision(incomplete, 'sk')).toBe(incomplete)
  expect(translationPending(incomplete, 'sk')).toBe(true)
})
