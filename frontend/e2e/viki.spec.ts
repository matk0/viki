import { expect, test, type Page } from '@playwright/test'

const password = process.env.INITIAL_USER_PASSWORD ?? 'password'
const initialUser = 'matej@matejlukasik.com'
const browserErrors = new WeakMap<Page, string[]>()

test.beforeEach(async ({ page }) => {
  const errors: string[] = []
  browserErrors.set(page, errors)
  page.on('console', (message) => {
    const text = message.text()
    if (message.type() === 'error' && !text.includes('status of 401 (Unauthorized)')) errors.push(`console: ${text}`)
  })
  page.on('pageerror', (error) => errors.push(`page: ${error.message}`))
  page.on('response', (response) => { if (response.status() >= 500) errors.push(`network: ${response.status()} ${response.url()}`) })
})

test.afterEach(async ({ page }) => {
  expect(browserErrors.get(page) ?? []).toEqual([])
})

async function login(page: Page, user = initialUser) {
  await page.goto('/')
  await page.getByLabel('E-mail').fill(user)
  await page.getByLabel('Heslo').fill(password)
  await page.getByRole('button', { name: /Prihlásiť sa/ }).click()
  await expect(page.getByRole('heading', { name: 'Pojmy', exact: true })).toBeVisible()
}

async function expectGrayscale(page: Page) {
  const violations = await page.locator('body').evaluate((body) => {
    const properties = [
      'color', 'backgroundColor', 'borderTopColor', 'borderRightColor',
      'borderBottomColor', 'borderLeftColor', 'outlineColor', 'fill', 'stroke',
      'caretColor', 'textDecorationColor', 'accentColor', 'boxShadow', 'backgroundImage',
    ] as const
    const results: string[] = []
    const inspect = (element: Element, pseudo?: string) => {
      const style = getComputedStyle(element, pseudo)
      for (const property of properties) {
        const value = style[property]
        for (const match of value.matchAll(/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)/g)) {
          const channels = match.slice(1, 4).map(Number)
          if (channels[0] !== channels[1] || channels[1] !== channels[2]) {
            const name = element instanceof HTMLElement && element.className
              ? `${element.tagName.toLowerCase()}.${String(element.className).replaceAll(' ', '.')}`
              : element.tagName.toLowerCase()
            results.push(`${name}${pseudo ?? ''} ${property}: ${value}`)
          }
        }
      }
    }
    for (const element of [body, ...body.querySelectorAll('*')]) {
      inspect(element)
      inspect(element, '::before')
      inspect(element, '::after')
    }
    return [...new Set(results)]
  })

  expect(violations).toEqual([])
}

async function expectNoSlantedElements(page: Page) {
  const slanted = await page.locator('body').evaluate((body) => {
    return [body, ...body.querySelectorAll('*')].flatMap((element) => {
      const transform = getComputedStyle(element).transform
      if (transform === 'none') return []
      const matrix = new DOMMatrixReadOnly(transform)
      if (Math.abs(matrix.b) < 0.0001 && Math.abs(matrix.c) < 0.0001) return []
      const name = element instanceof HTMLElement && element.className
        ? `${element.tagName.toLowerCase()}.${String(element.className).replaceAll(' ', '.')}`
        : element.tagName.toLowerCase()
      return [`${name}: ${transform}`]
    })
  })

  expect(slanted).toEqual([])
}

test('login story contains only the viki wordmark', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.login-story')).toHaveText(/^viki$/)
  await expect(page.getByText('Zdieľané porozumenie predtým, než vznikne nový systém.')).toHaveCount(0)
  await page.screenshot({ path: '../outputs/viki-login.png', fullPage: true })
})

test('login story always occupies one third of the viewport', async ({ page }) => {
  for (const width of [390, 1200, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/')
    const storyWidth = await page.locator('xpath=/html/body/div/div/section[1]').evaluate((element) => element.getBoundingClientRect().width)
    expect(storyWidth).toBeCloseTo(width / 3, 0)
  }
  await page.screenshot({ path: '../outputs/viki-login-one-third.png', fullPage: true })
})

test('login exposes only the requested account without demo controls', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByLabel('E-mail')).toHaveValue('matej@matejlukasik.com')
  await expect(page.getByLabel('Heslo')).toHaveValue('password')
  await expect(page.locator('xpath=/html/body/div/div/section[2]/form/p')).toHaveCount(0)
  await expect(page.locator('xpath=/html/body/div/div/section[2]/form/div[2]')).toHaveCount(0)
  await expect(page.locator('xpath=/html/body/div/div/section[2]/form/small')).toHaveCount(0)
})

test('uses the Notion application font stack across the workspace', async ({ page }) => {
  const notionFontSource = 'ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI Variable Display", "Segoe UI", Helvetica, "Apple Color Emoji", "Noto Sans Arabic", "Noto Sans Hebrew", Arial, sans-serif, "Segoe UI Emoji", "Segoe UI Symbol"'
  const notionFont = 'ui-sans-serif, -apple-system, "system-ui", "Segoe UI Variable Display", "Segoe UI", Helvetica, "Apple Color Emoji", "Noto Sans Arabic", "Noto Sans Hebrew", Arial, sans-serif, "Segoe UI Emoji", "Segoe UI Symbol"'

  await page.goto('/')
  expect(await page.locator('html').evaluate((element) => getComputedStyle(element).getPropertyValue('--notion-font').trim())).toBe(notionFontSource)
  await expect(page.locator('html')).toHaveCSS('font-family', notionFont)
  await expect(page.locator('.login-wordmark')).toHaveCSS('font-family', notionFont)
  await expect(page.locator('.login-form h2')).toHaveCSS('font-family', notionFont)

  await login(page)
  await expect(page.locator('.wordmark')).toHaveCSS('font-family', notionFont)
  await expect(page.locator('.page-heading h1')).toHaveCSS('font-family', notionFont)
  await expect(page.locator('.page-heading h1')).toHaveCSS('font-family', notionFont)
})

test('renders the interface using only black, white, and shades of gray', async ({ page }) => {
  await page.goto('/')
  await expectGrayscale(page)

  await login(page)
  await expectGrayscale(page)

  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()
  await expect(page.locator('.assistant-drawer')).toBeVisible()
  await expectGrayscale(page)
  await page.screenshot({ path: '../outputs/viki-grayscale.png', fullPage: true })
})

test('renders every interface element without a decorative slant', async ({ page }) => {
  await page.goto('/')
  await expectNoSlantedElements(page)

  await login(page)
  await expectNoSlantedElements(page)
  await page.screenshot({ path: '../outputs/viki-no-slant.png', fullPage: true })

  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()
  await expectNoSlantedElements(page)
})

test('omits eyebrow labels from every logged-in page header', async ({ page }) => {
  await login(page)
  await expect(page.locator('xpath=/html/body/div/div/main/div/header/div[1]/span')).toHaveCount(0)

  for (const destination of ['Pojmy', 'Scenáre', 'Drafty', 'História zmien']) {
    await page.getByRole('link', { name: destination, exact: true }).click()
    await expect(page.locator('main header .eyebrow')).toHaveCount(0)
  }

  await page.locator('.sidebar-search').click()
  await expect(page.locator('main header .eyebrow')).toHaveCount(0)

  await page.getByRole('link', { name: 'Pojmy', exact: true }).click()
  await page.getByRole('button', { name: 'Pridať pojem' }).click()
  await page.getByLabel('Názov').fill(`Stránka bez štítku ${Date.now()}`)
  await page.getByRole('button', { name: 'Vytvoriť koncept' }).click()
  await expect(page.locator('.document-header .eyebrow')).toHaveCount(0)
})

test('does not track login or logout activity in change history', async ({ page }) => {
  await login(page)
  await page.getByRole('link', { name: 'História zmien', exact: true }).click()

  await expect(page.getByRole('heading', { name: 'História zmien' })).toBeVisible()
  await expect(page.locator('.audit-list')).not.toContainText('sa prihlásil')
  await expect(page.locator('.audit-list')).not.toContainText('sa odhlásil')
  await page.screenshot({ path: '../outputs/viki-audit-without-authentication.png', fullPage: true })
})

test('opens the all-drafts view from primary navigation', async ({ page }) => {
  await login(page)

  await page.getByRole('link', { name: 'Drafty', exact: true }).click()
  await expect(page).toHaveURL(/\/drafts$/)
  await expect(page.getByRole('heading', { name: 'Drafty', exact: true })).toBeVisible()
  await page.screenshot({ path: '../outputs/viki-all-drafts.png', fullPage: true })

  await page.setViewportSize({ width: 390, height: 844 })
  await page.reload()
  await page.getByRole('button', { name: 'Otvoriť navigáciu' }).click()
  await expect(page.getByRole('link', { name: 'Drafty', exact: true })).toBeVisible()
  await expect(page.locator('.sidebar')).toHaveCSS('transform', 'matrix(1, 0, 0, 1, 0, 0)')
  await page.screenshot({ path: '../outputs/viki-all-drafts-mobile.png', fullPage: true })
})

test('places change history at the bottom of the sidebar navigation', async ({ page }) => {
  await login(page)

  const historyBox = await page.getByRole('link', { name: 'História zmien', exact: true }).boundingBox()
  const userBox = await page.locator('.sidebar-user').boundingBox()

  expect(historyBox).not.toBeNull()
  expect(userBox).not.toBeNull()
  expect(userBox!.y - (historyBox!.y + historyBox!.height)).toBeLessThanOrEqual(8)
  await page.screenshot({ path: '../outputs/viki-history-at-navigation-bottom.png', fullPage: true })
})

test('uses a bold, friendly, easy-to-use visual language', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.login-story')).toHaveCSS('background-color', 'rgb(0, 0, 0)')
  await expect(page.locator('.login-wordmark')).toHaveCSS('color', 'rgb(255, 255, 255)')
  await expect(page.locator('.login-form')).toHaveCSS('border-top-width', '2px')
  await expect(page.locator('.login-form')).toHaveCSS('border-radius', '28px')
  await expect(page.locator('.login-form h2')).toHaveCSS('font-weight', '800')
  await expect(page.locator('.login-submit')).toHaveCSS('height', '50px')
  await expect(page.locator('.login-submit')).toHaveCSS('border-radius', '999px')
  await page.screenshot({ path: '../outputs/viki-friendly-login.png', fullPage: true })

  await login(page)
  await expect(page.locator('.nav-item').first()).toHaveCSS('height', '44px')
  await expect(page.locator('.page-heading h1')).toHaveCSS('font-weight', '800')
  expect(await page.locator('.page-heading h1').evaluate((element) => parseFloat(getComputedStyle(element).fontSize))).toBeGreaterThanOrEqual(48)
  await expect(page.locator('.primary-button').first()).toHaveCSS('border-radius', '999px')
  await expect(page.locator('.panel').first()).toHaveCSS('border-top-width', '2px')
  await expect(page.locator('.panel').first()).toHaveCSS('border-radius', '18px')
  expect(await page.locator('.panel').first().evaluate((element) => getComputedStyle(element).boxShadow)).not.toBe('none')
  await page.screenshot({ path: '../outputs/viki-friendly-pojmy.png', fullPage: true })
})

test('gives the library search field the same raised shading as the status filter', async ({ page }) => {
  await login(page)

  const searchShadow = await page.locator('.search-field').evaluate((element) => getComputedStyle(element).boxShadow)
  const filterShadow = await page.locator('.filter-select .viki-select-trigger').evaluate((element) => getComputedStyle(element).boxShadow)

  expect(searchShadow).toBe(filterShadow)
  expect(searchShadow).not.toBe('none')
  await page.screenshot({ path: '../outputs/viki-shaded-search-field.png', fullPage: true })
})

test('keeps select option panels the same width as their triggers', async ({ page }) => {
  await login(page)

  const filterTrigger = page.getByRole('button', { name: /Filtrovať podľa stavu/ })
  await filterTrigger.click()
  const filterTriggerBox = await filterTrigger.boundingBox()
  const filterOptionsBox = await page.getByRole('listbox', { name: 'Filtrovať podľa stavu' }).boundingBox()

  expect(filterTriggerBox).not.toBeNull()
  expect(filterOptionsBox).not.toBeNull()
  expect(filterOptionsBox!.x).toBeCloseTo(filterTriggerBox!.x, 0)
  expect(filterOptionsBox!.width).toBeCloseTo(filterTriggerBox!.width, 0)
  await page.screenshot({ path: '../outputs/viki-select-options-match-trigger-width.png', fullPage: true })

  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()
  const conversationTrigger = page.getByRole('button', { name: /Rozhovor:/ })
  await conversationTrigger.click()
  const conversationTriggerBox = await conversationTrigger.boundingBox()
  const conversationOptionsBox = await page.getByRole('listbox', { name: 'Rozhovory' }).boundingBox()

  expect(conversationTriggerBox).not.toBeNull()
  expect(conversationOptionsBox).not.toBeNull()
  expect(conversationOptionsBox!.x).toBeCloseTo(conversationTriggerBox!.x, 0)
  expect(conversationOptionsBox!.width).toBeCloseTo(conversationTriggerBox!.width, 0)
})

test('keeps the page type fixed while styling the remaining modal select', async ({ page }) => {
  await login(page)
  await page.getByRole('button', { name: 'Pridať pojem' }).click()

  await expect(page.locator('select')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Vytvoriť pojem' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Typ stránky' })).toHaveCount(0)
  const trigger = page.getByRole('button', { name: 'Druh pojmu' })
  await expect(trigger).toHaveText('Podstatné meno')
  await trigger.click()

  const menu = page.getByRole('listbox', { name: 'Druhy pojmov' })
  const triggerBox = await trigger.boundingBox()
  const menuBox = await menu.boundingBox()
  expect(triggerBox).not.toBeNull()
  expect(menuBox).not.toBeNull()
  expect(menuBox!.x).toBeCloseTo(triggerBox!.x, 0)
  expect(menuBox!.width).toBeCloseTo(triggerBox!.width, 0)
  await expect(page.getByRole('option', { name: 'Podstatné meno' })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('option', { name: 'Sloveso', exact: true })).toHaveCSS('font-family', await trigger.evaluate((element) => getComputedStyle(element).fontFamily))
  await page.screenshot({ path: '../outputs/viki-uniform-modal-select.png', fullPage: true })
})

test('does not show trailing chevrons in navigable content rows', async ({ page }) => {
  await login(page)

  for (const destination of ['Pojmy', 'Scenáre', 'Drafty']) {
    await page.getByRole('link', { name: destination, exact: true }).click()
    await expect(page.locator('.lucide-chevron-right')).toHaveCount(0)
    if (destination === 'Pojmy') await page.screenshot({ path: '../outputs/viki-primitives-without-row-chevrons.png', fullPage: true })
  }
})

test('does not expose provenance fields on wiki pages or in the editor', async ({ page }) => {
  await login(page)
  await page.goto('/page/b0d2eac1-6f07-412e-a714-813e227550d2')

  await expect(page.getByRole('heading', { name: 'Zmluva', exact: true })).toBeVisible()
  await expect(page.locator('.source-card')).toHaveCount(0)
  await expect(page.getByText('Pôvod obsahu')).toHaveCount(0)
  await page.screenshot({ path: '../outputs/viki-page-without-provenance.png', fullPage: true })

  await page.getByRole('button', { name: 'Upraviť', exact: true }).click()
  await expect(page.getByLabel('Zdroje')).toHaveCount(0)
  await expect(page.getByLabel('Pôvod informácie')).toHaveCount(0)
  await page.screenshot({ path: '../outputs/viki-editor-without-provenance.png', fullPage: true })
})

test('edits pages inline from the right side of revision metadata', async ({ page }) => {
  await login(page)
  await page.goto('/page/b0d2eac1-6f07-412e-a714-813e227550d2')

  const metadata = page.locator('.document-header .revision-meta')
  const edit = page.locator('.document-header').getByRole('button', { name: 'Upraviť', exact: true })
  const metadataBox = await metadata.boundingBox()
  const editBox = await edit.boundingBox()

  expect(metadataBox).not.toBeNull()
  expect(editBox).not.toBeNull()
  expect(editBox!.x).toBeGreaterThan(metadataBox!.x + metadataBox!.width)
  await page.screenshot({ path: '../outputs/viki-edit-action-in-document-header.png', fullPage: true })

  await edit.click()
  await expect(page.locator('.document-page > .page-editor')).toBeVisible()
  await expect(page.locator('.document-tools .page-editor')).toHaveCount(0)
  await expect(page.locator('.document-tools .review-panel')).toBeVisible()
  await page.screenshot({ path: '../outputs/viki-inline-page-editor.png', fullPage: true })
})

test('centers the assistant mode controls', async ({ page }) => {
  await login(page)
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()

  await expect(page.locator('.assistant-controls')).toHaveCSS('justify-content', 'center')
  await page.screenshot({ path: '../outputs/viki-centered-assistant-controls.png', fullPage: true })
})

test('does not show preset assistant prompts or their publishing hint', async ({ page }) => {
  await login(page)
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()

  await expect(page.getByRole('button', { name: 'Čo treba pre zmluvu?' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Vytvoriť scenár' })).toHaveCount(0)
  await expect(page.getByText('Pred publikovaním uvidíte celý návrh.')).toHaveCount(0)
  await page.screenshot({ path: '../outputs/viki-assistant-without-preset-prompts.png', fullPage: true })
})

test('does not render or reserve space for the AIRNET demo banner', async ({ page }) => {
  await login(page)
  await expect(page.locator('xpath=/html/body/div/div/div')).toHaveCount(0)
  await expect(page.locator('.app-shell')).toHaveCSS('padding-top', '0px')
  await expect(page.locator('.sidebar')).toHaveCSS('top', '0px')
  expect(await page.locator('.main-content').evaluate((element) => getComputedStyle(element).minHeight)).toBe(`${await page.evaluate(() => innerHeight)}px`)
  await page.screenshot({ path: '../outputs/viki-without-banner.png', fullPage: true })

  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()
  await expect(page.locator('.assistant-drawer')).toHaveCSS('top', '0px')

  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.locator('.sidebar')).toHaveCSS('top', '0px')
  await expect(page.locator('.assistant-drawer')).toHaveCSS('top', '0px')
  await page.getByRole('button', { name: 'Zavrieť asistenta' }).click()
  await page.getByRole('button', { name: 'Otvoriť navigáciu' }).click()
  await expect(page.locator('.sidebar-scrim')).toHaveCSS('top', '0px')
})

test('localizes primitive pages as Pojmy and Pojem', async ({ page }) => {
  await login(page)
  await expect(page.getByRole('link', { name: 'Pojmy', exact: true })).toBeVisible()
  await expect(page.locator('body')).not.toContainText(/primitív/i)

  await page.getByRole('link', { name: 'Pojmy', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Pojmy', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Pridať pojem' })).toBeVisible()
  await expect(page.getByPlaceholder('Hľadať v pojmoch…')).toBeVisible()

  await page.getByRole('button', { name: 'Pridať pojem' }).click()
  await expect(page.getByRole('heading', { name: 'Vytvoriť pojem' })).toBeVisible()
  await expect(page.getByLabel('Typ stránky')).toHaveCount(0)
  await expect(page.getByLabel('Druh pojmu')).toBeVisible()
  await page.getByLabel('Názov').fill(`Testovací pojem ${Date.now()}`)
  await page.getByRole('button', { name: 'Vytvoriť koncept' }).click()
  await expect(page.locator('.document-breadcrumb')).toContainText('Pojmy')
  await expect(page.locator('.document-header .eyebrow')).toHaveCount(0)
  await page.screenshot({ path: '../outputs/viki-pojmy.png', fullPage: true })
})

test('the single initial user can authenticate', async ({ page }) => {
  await login(page)
  await expect(page.locator('.sidebar-user')).toContainText(initialUser)
})

test('removes Prehľad and uses Pojmy as the root screen', async ({ page }) => {
  await page.goto('/')
  await page.getByLabel('E-mail').fill(initialUser)
  await page.getByLabel('Heslo').fill(password)
  await page.getByRole('button', { name: /Prihlásiť sa/ }).click()

  await expect(page.getByRole('heading', { name: 'Pojmy', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Prehľad', exact: true })).toHaveCount(0)
  await expect(page.locator('.metric-grid, .dashboard-grid, .draft-callout')).toHaveCount(0)
  await page.screenshot({ path: '../outputs/viki-without-prehlad.png', fullPage: true })
})

test('removes the Asistent tab and screen while preserving the assistant drawer', async ({ page }) => {
  await login(page)

  await expect(page.getByRole('link', { name: 'Asistent', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()
  await expect(page.locator('.assistant-drawer')).toBeVisible()
  await expect(page.getByText('viki asistent')).toBeVisible()
  await page.screenshot({ path: '../outputs/viki-without-assistant-tab.png', fullPage: true })

  await page.goto('/assistant')
  await expect(page.getByRole('heading', { name: 'Stránka sa nenašla' })).toBeVisible()
})

test('removes the duplicate scenario shortcut section from the sidebar', async ({ page }) => {
  await login(page)

  await expect(page.locator('.sidebar-section')).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Scenáre', exact: true })).toBeVisible()
  await page.screenshot({ path: '../outputs/viki-without-scenario-shortcuts.png', fullPage: true })
})

test('removes the global new-page button from the sidebar', async ({ page }) => {
  await login(page)

  await expect(page.getByRole('button', { name: 'Nová stránka' })).toHaveCount(0)
  await expect(page.locator('.new-page-button')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Pridať pojem' })).toBeVisible()
  await page.screenshot({ path: '../outputs/viki-sidebar-without-new-page.png', fullPage: true })
})

test('Edit opens a live Draft proposal and publishes only after approval', async ({ page }) => {
  const conversationId = '00000000-0000-4000-8000-000000000071'
  const turnId = '00000000-0000-4000-8000-000000000072'
  const now = '2026-07-31T10:00:00Z'
  const summary = {
    id: conversationId,
    title: 'Návrh zmluvy',
    primaryMode: 'edit',
    lastMode: 'edit',
    state: 'idle',
    createdAt: now,
    updatedAt: now,
  }
  const proposal = {
    id: turnId,
    conversationId,
    turnId,
    summary: 'Pridať pojem Zmluva',
    status: 'awaiting_approval',
    operations: [{
      operation: 'create',
      clientKey: 'contract',
      kind: 'primitive',
      primitiveKind: 'noun',
      slug: 'zmluva',
      content: {
        title: 'Zmluva',
        bodyMd: 'Dohoda medzi spoločnosťou a zákazníkom.',
        aliases: [], steps: [], references: [],
      },
    }],
    publishedRevisions: [],
    createdAt: now,
    updatedAt: now,
  }

  await page.route('**/api/v1/assistant/**', async (route) => {
    const request = route.request()
    const pathname = new URL(request.url()).pathname
    if (pathname === '/api/v1/assistant/status') {
      return route.fulfill({ json: { available: true, qa: { mode: 'qa', connected: true, configured: true, ready: true }, edit: { mode: 'edit', connected: true, configured: true, ready: true } } })
    }
    if (pathname === '/api/v1/assistant/conversations' && request.method() === 'GET') return route.fulfill({ json: { conversations: [summary] } })
    if (pathname === `/api/v1/assistant/conversations/${conversationId}` && request.method() === 'GET') return route.fulfill({ json: { ...summary, messages: [] } })
    if (pathname === `/api/v1/assistant/conversations/${conversationId}/events`) return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 60000\n\n' })
    if (pathname === `/api/v1/assistant/conversations/${conversationId}/messages`) return route.fulfill({ status: 202, json: { turnId, mode: 'edit' } })
    return route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'not found' } } })
  })
  await page.route(`**/api/v1/draft-proposals/${turnId}`, (route) => route.fulfill({ json: proposal }))
  await page.route(`**/api/v1/draft-proposals/${turnId}/approve`, (route) => route.fulfill({ json: {
    ...proposal,
    status: 'published',
    publishedAt: now,
    publishedRevisions: [{
      ...proposal.operations[0].content,
      id: '00000000-0000-4000-8000-000000000073',
      pageId: '00000000-0000-4000-8000-000000000074',
      number: 1,
      status: 'accepted',
      createdBy: { id: '00000000-0000-4000-8000-000000000011', email: initialUser, displayName: 'Matej', createdAt: now },
      createdAt: now,
    }],
  } }))

  await login(page)
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()
  await page.getByRole('button', { name: 'Úpravy' }).click()
  await page.getByPlaceholder('Opíšte, čo má viki pridať alebo zmeniť…').fill('Pridaj pojem Zmluva')
  await page.getByRole('button', { name: 'Odoslať' }).click()

  await expect(page).toHaveURL(`/drafts/${turnId}`)
  await expect(page.getByRole('heading', { name: 'Zmluva' })).toBeVisible()
  await expect(page.getByText('Vo viki zatiaľ nebol vytvorený žiadny záznam.')).toBeVisible()
  const approveButton = page.getByRole('button', { name: 'Schváliť a publikovať' })
  await expect(approveButton).toHaveCSS('background-color', 'rgb(183, 228, 108)')
  await expect(approveButton).toHaveCSS('color', 'rgb(32, 32, 32)')
  await page.screenshot({ path: '../outputs/viki-live-draft.png', fullPage: true })

  const openRejectButton = page.getByRole('button', { name: 'Odmietnuť' })
  await expect(openRejectButton).toHaveCSS('background-color', 'rgb(255, 107, 107)')
  await expect(openRejectButton).toHaveCSS('color', 'rgb(32, 32, 32)')
  await openRejectButton.hover()
  await expect(openRejectButton).toHaveCSS('background-color', 'rgb(240, 90, 90)')
  await openRejectButton.click()
  await expect(page.getByRole('dialog', { name: 'Odmietnuť návrh?' })).toBeVisible()
  const rejectButton = page.getByRole('button', { name: 'Odmietnuť návrh' })
  await expect(rejectButton).toBeDisabled()
  await page.getByRole('textbox', { name: 'Dôvod odmietnutia' }).fill('Chýba presný spôsob výpočtu ceny.')
  await expect(rejectButton).toBeEnabled()
  await page.screenshot({ path: '../outputs/viki-reject-draft-modal.png', fullPage: true })
  await page.getByRole('button', { name: 'Zrušiť' }).click()
  await expect(page.getByRole('dialog', { name: 'Odmietnuť návrh?' })).toHaveCount(0)

  await approveButton.click()
  await expect(page.getByText('Publikované')).toBeVisible()
  await expect(page.getByText('1 prijatá revízia je teraz vo viki.')).toBeVisible()
})

test('does not seed the AIRNET wiki corpus', async ({ page }) => {
  await login(page)
  const pages = await (await page.context().request.get('/api/v1/pages')).json()
  expect(pages.pages.map((item: { title: string }) => item.title)).not.toContain('Rezervácia internetovej služby')
})

test('rejection blocks publication, resolution permits it, and accepted content stays visible beside a draft', async ({ page }) => {
  await login(page)
  const suffix = Date.now()
  await page.getByRole('button', { name: 'Pridať pojem' }).click()
  await page.getByLabel('Názov').fill(`Testovacie pravidlo ${suffix}`)
  await page.getByRole('button', { name: 'Vytvoriť koncept' }).click()
  await expect(page.getByText('Kontrola revízie #1')).toBeVisible()

  await page.getByRole('button', { name: 'Nesúhlasím' }).click()
  const reject = page.getByRole('button', { name: 'Odoslať námietku' })
  await expect(reject).toBeDisabled()
  await page.getByLabel('Dôvod nesúhlasu').fill('Chýba presná definícia pojmu.')
  await reject.click()
  await expect(page.getByText('1 otvorená námietka')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Publikovanie je zablokované' })).toBeDisabled()
  await page.getByRole('button', { name: 'Označiť ako vyriešené' }).click()
  await page.getByRole('button', { name: 'Publikovať revíziu' }).click()
  await expect(page.getByText('Revízia #1')).toBeVisible()

  await page.getByRole('button', { name: 'Upraviť' }).click()
  await expect(page.locator('.page-editor > label')).toHaveCount(3)
  await expect(page.getByText('Obsahuje ilustračné, neoverené pravidlá')).toHaveCount(0)
  await page.getByLabel('Obsah stránky').fill('Toto je nový obsah, ktorý ešte nebol publikovaný.')
  await page.getByRole('button', { name: 'Uložiť novú revíziu' }).click()
  await expect(page.getByRole('tab', { name: 'Publikované' })).toBeVisible()
  await expect(page.getByText('Táto stránka zatiaľ nemá opis.')).toBeVisible()
  await page.getByRole('tab', { name: /Koncept #2/ }).click()
  await expect(page.getByText('Toto je nový obsah, ktorý ešte nebol publikovaný.')).toBeVisible()
})

test('optimistic concurrency returns a recoverable 409', async ({ page }) => {
  await login(page)
  const suffix = Date.now()
  const csrf = await page.evaluate(() => decodeURIComponent(document.cookie.split('; ').find((cookie) => cookie.startsWith('viki_csrf='))?.split('=').slice(1).join('=') ?? ''))
  const content = { title: `Súbežný pojem ${suffix}`, bodyMd: '', aliases: [], steps: [], references: [] }
  const created = await page.context().request.post('/api/v1/pages', { headers: { 'X-CSRF-Token': csrf }, data: { kind: 'primitive', primitiveKind: 'noun', slug: `subezny-pojem-${suffix}`, content } })
  expect(created.status()).toBe(201)
  const detail = await created.json()
  const baseRevisionId = detail.draftRevision.id
  const first = await page.context().request.post(`/api/v1/pages/${detail.page.id}/revisions`, { headers: { 'X-CSRF-Token': csrf }, data: { baseRevisionId, content: { ...content, bodyMd: 'Prvý zápis.' } } })
  expect(first.status()).toBe(201)
  const stale = await page.context().request.post(`/api/v1/pages/${detail.page.id}/revisions`, { headers: { 'X-CSRF-Token': csrf }, data: { baseRevisionId, content: { ...content, bodyMd: 'Konfliktný zápis.' } } })
  expect(stale.status()).toBe(409)
  await expect(stale.json()).resolves.toMatchObject({ error: { code: 'revision_conflict' } })
})

test('users can manually create a scenario from the scenario index', async ({ page }) => {
  await login(page)
  const suffix = Date.now()
  const scenarioTitle = `E2E proces ${suffix}`
  await page.getByLabel('Hlavná navigácia').getByRole('link', { name: 'Scenáre', exact: true }).click()
  await page.getByRole('button', { name: 'Pridať scenár' }).click()
  await expect(page.getByRole('heading', { name: 'Vytvoriť scenár' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Typ stránky' })).toHaveCount(0)
  await page.getByLabel('Názov').fill(scenarioTitle)
  await page.getByRole('button', { name: 'Vytvoriť koncept' }).click()
  await expect(page.getByRole('heading', { name: scenarioTitle })).toBeVisible()
})

test('wiki creation remains usable while the assistant is unavailable', async ({ page }) => {
  await login(page)
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()
  await expect(page.getByText('Asistent je momentálne nedostupný.')).toBeVisible()

  const sourceTitle = `Pojem bez asistenta ${Date.now()}`
  await page.getByRole('button', { name: 'Pridať pojem' }).click()
  await page.getByLabel('Názov').fill(sourceTitle)
  await page.getByRole('button', { name: 'Vytvoriť koncept' }).click()
  await expect(page.getByRole('heading', { name: sourceTitle })).toBeVisible()
  await expect(page.getByText('Kontrola revízie #1')).toBeVisible()
})

test('mobile workspace navigation remains usable', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await login(page)
  await page.getByRole('button', { name: 'Otvoriť navigáciu' }).click()
  await page.getByRole('link', { name: 'Scenáre', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Scenáre', exact: true })).toBeVisible()
  await page.waitForFunction(() => document.querySelector('.sidebar')?.getBoundingClientRect().right === 0)
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  expect(overflow).toBeLessThanOrEqual(1)
  await page.screenshot({ path: '../outputs/viki-mobile.png', fullPage: true })
})
