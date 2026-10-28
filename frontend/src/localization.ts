import type { Localizations, Page, PageDetail, Revision, SearchResult } from './api/types'
import type { Locale } from './i18n'

function translation(localizations: Localizations | undefined, revisionId: string | undefined, locale: Locale) {
  if (locale === 'en') return undefined
  const content = localizations?.[locale]
  return content?.revisionId === revisionId ? content : undefined
}

export function localizedPage(page: Page, locale: Locale): Page {
  const content = translation(page.localizations, page.repositoryRevisionId, locale)
  return content ? { ...page, title: content.title } : page
}

export function localizedRevision(revision: Revision, locale: Locale): Revision {
  const content = translation(revision.localizations, revision.id, locale)
  if (!content || content.stepTexts.length !== revision.steps.length) return revision
  return { ...revision, title: content.title, bodyMd: content.bodyMd, steps: revision.steps.map((step, index) => ({ ...step, text: content.stepTexts[index] })) }
}

export function localizedDetail(detail: PageDetail, locale: Locale): PageDetail {
  return { ...detail, page: localizedPage(detail.page, locale), children: detail.children.map((page) => localizedPage(page, locale)) }
}

export function localizedResult(result: SearchResult, locale: Locale): SearchResult {
  const content = translation(result.page.localizations, result.revisionId, locale)
  return { ...result, page: localizedPage(result.page, locale), excerpt: content ? content.bodyMd || content.stepTexts.join(' ') : result.excerpt }
}

export function translationPending(revision: Revision, locale: Locale): boolean {
  const content = translation(revision.localizations, revision.id, locale)
  return locale === 'sk' && revision.status === 'repository_baseline' && (!content || content.stepTexts.length !== revision.steps.length)
}
