import { mkdir } from 'node:fs/promises'
import { expect, test } from '@playwright/test'

test('public reader supports search, cited Q&A, separate visitors, and mobile reading', async ({ page, browser }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  const response = await page.request.get('/api/v1/pages?kind=scenario')
  const { pages } = await response.json()
  const scenario = pages.find((value: { title: string }) => value.title === 'An unresolved objection blocks approval')
  expect(scenario).toBeTruthy()
  const citation = { pageId: scenario.id, revisionId: scenario.repositoryRevisionId, pageTitle: scenario.title, draft: false }
  let requests = 0
  await page.route('**/api/public/qa', async (route) => {
    requests++
    await route.fulfill({ json: { answer: 'An unresolved objection blocks approval until it is resolved.', citations: [citation] } })
  })
  await page.goto('/')
  await expect(page.locator('.public-demo-banner, .public-demo-footer')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Concepts', exact: true })).toBeVisible()
  await page.getByRole('button', { name: /^Search/ }).click()
  await page.getByRole('textbox').fill('unresolved objection')
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  await page.getByRole('link', { name: /An unresolved objection blocks approval/ }).click()
  await expect(page.getByRole('heading', { name: scenario.title })).toBeVisible()
  await expect(page.getByText('a draft revision has an unresolved objection', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Open assistant' }).click()
  const qa = page.getByRole('region', { name: 'Ask about Viki' })
  await qa.getByLabel('Your question').fill('What blocks approval?')
  await qa.getByRole('button', { name: 'Ask', exact: true }).click()
  await expect(qa.getByText('An unresolved objection blocks approval until it is resolved.')).toBeVisible()
  await expect(qa.getByRole('link', { name: scenario.title })).toHaveAttribute('href', '/page/' + scenario.id + '?revision=' + scenario.repositoryRevisionId)
  expect(requests).toBe(1)
  await page.locator('.main-content').evaluate((element) => Promise.all(element.getAnimations().map((animation) => animation.finished)))
  await mkdir('../outputs/public-demo', { recursive: true })
  await page.screenshot({ path: '../outputs/public-demo/desktop.png', fullPage: true })

  const second = await browser.newContext()
  const other = await second.newPage()
  await other.goto('http://127.0.0.1:8790/')
  await other.getByRole('button', { name: 'Open assistant' }).click()
  const otherQA = other.getByRole('region', { name: 'Ask about Viki' })
  await expect(otherQA.getByText('Let Viki explain itself.')).toBeVisible()
  await expect(otherQA.getByText('What blocks approval?', { exact: true })).toHaveCount(0)
  await second.close()

  await qa.getByRole('button', { name: 'Close assistant' }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.reload()
  await expect(page.getByRole('heading', { name: scenario.title })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: '../outputs/public-demo/mobile.png', fullPage: true })
  expect(errors).toEqual([])
})

test('quota failures remain visible and do not prevent reading the catalogue', async ({ page }) => {
  await page.route('**/api/public/qa', (route) => route.fulfill({
    status: 429, json: { error: { message: 'The free Q&A allowance is used up for now. Browsing and search are still available.' } },
  }))
  await page.goto('/')
  await page.getByRole('button', { name: 'Open assistant' }).click()
  const qa = page.getByRole('region', { name: 'Ask about Viki' })
  await qa.getByLabel('Your question').fill('What is Viki?')
  await qa.getByRole('button', { name: 'Ask', exact: true }).click()
  await expect(qa.getByRole('alert')).toContainText('free Q&A allowance')
  await qa.getByRole('button', { name: 'Close assistant' }).click()
  await page.getByRole('link', { name: 'Features', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Features', exact: true })).toBeVisible()
})
