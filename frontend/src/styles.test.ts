import { expect, it } from 'vitest'
import styles from './styles.css?raw'

it('uses a readable application-wide type scale without changing h1 display sizes', () => {
  const pixelSizes = [...styles.matchAll(/font-size:\s*(\d+)px/g)].map((match) => Number(match[1]))

  expect(Math.min(...pixelSizes)).toBeGreaterThanOrEqual(11)
  expect(styles).toMatch(/body\s*\{[^}]*font-size:\s*17px;/s)
  expect(styles).toMatch(/\.assistant-panel\s*\{[^}]*font-size:\s*14px;/s)
  expect(styles).toMatch(/\.comment-thread\s*>\s*p,[^{]*\{[^}]*font-size:\s*13px;/s)

  expect(styles).toMatch(/\.page-heading\s+h1\s*\{[^}]*font-size:\s*clamp\(48px,\s*5vw,\s*64px\);/s)
  expect(styles).toMatch(/\.document-title-row\s+h1\s*\{[^}]*font-size:\s*clamp\(48px,\s*5vw,\s*62px\);/s)
  expect(styles).toMatch(/\.markdown\s+h1\s*\{[^}]*font-size:\s*34px;/s)
})

it('highlights the whole feature card instead of only its heading link', () => {
  expect(styles).toMatch(/\.feature-card:hover\s*\{[^}]*background:\s*#f2f2f2;/s)
  expect(styles).not.toMatch(/\.feature-card-heading:hover\s*\{/)
})

it('uses the same title size for the review and discussion sidebar panels', () => {
  const reviewTitleSize = styles.match(/\.panel-heading\s+h2\s*\{[^}]*font-size:\s*(\d+px);/s)?.[1]
  const discussionTitleSize = styles.match(/\.discussion-accordion\s*>\s*summary\s*\{[^}]*font-size:\s*(\d+px);/s)?.[1]

  expect(discussionTitleSize).toBe(reviewTitleSize)
})

it('keeps the new-version header action on one line', () => {
  expect(styles).toMatch(/\.document-edit-button\s*\{[^}]*width:\s*124px;[^}]*white-space:\s*nowrap;/s)
})

it('reserves enough width for full localized Gherkin keywords', () => {
  expect(styles).toMatch(/\.bdd-edit-row\s*\{[^}]*grid-template-columns:\s*120px 1fr auto;/s)
})

it('stacks Gherkin step controls into readable rows on phones', () => {
  expect(styles).toMatch(/@media \(max-width:\s*600px\)[\s\S]*?\.bdd-edit-row\s*\{[^}]*grid-template-columns:\s*1fr;/s)
  expect(styles).toMatch(/@media \(max-width:\s*600px\)[\s\S]*?\.bdd-edit-row \.row-actions\s*\{[^}]*justify-self:\s*end;/s)
})

it('uses the sidebar search radius for every button', () => {
  expect(styles).toMatch(/--button-radius:\s*13px;/)
  expect(styles).toMatch(/button\s*\{[^}]*border-radius:\s*var\(--button-radius\)\s*!important;/s)
  expect(styles).toMatch(/\.sidebar-search\s*\{[^}]*border-radius:\s*var\(--button-radius\);/s)
})

it('keeps modal backdrops above every application overlay', () => {
  const modalLayer = Number(styles.match(/\.modal-backdrop\s*\{[^}]*z-index:\s*(\d+);/s)?.[1])
  const selectLayer = Number(styles.match(/\.viki-select-menu\s*\{[^}]*z-index:\s*(\d+);/s)?.[1])
  const assistantLayer = Number(styles.match(/\.assistant-fab\s*\{[^}]*z-index:\s*(\d+);/s)?.[1])

  expect(modalLayer).toBeGreaterThan(selectLayer)
  expect(modalLayer).toBeGreaterThan(assistantLayer)
})
