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
  await expect(page.getByRole('heading', { name: 'Koncepty', exact: true })).toBeVisible()
}

async function createDraftConcept(page: Page, title: string) {
  await page.getByRole('button', { name: 'Pridať koncept' }).click()
  await page.getByLabel('Názov').fill(title)
  await page.getByRole('button', { name: 'Vytvoriť draft' }).click()
  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible()
}

async function proposeInitialScenarioSteps(page: Page, steps: readonly string[]) {
  for (const [index, text] of steps.entries()) {
    await page.getByRole('button', { name: 'Navrhnúť nový krok' }).first().click()
    await page.getByRole('textbox', { name: `Nová definícia kroku ${index + 1}` }).fill(text)
  }
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
      if (element.matches('.page-icon-box.approved, .status-badge.approved, .approve-revision-button, .reject-button, .rejection-button')) return
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

test('uses grayscale throughout except for approved review-state accents', async ({ page }) => {
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

  for (const destination of ['Koncepty', 'Funkcie', 'História zmien']) {
    await page.getByRole('link', { name: destination, exact: true }).click()
    await expect(page.locator('main header .eyebrow')).toHaveCount(0)
  }

  await page.locator('.sidebar-search').click()
  await expect(page.locator('main header .eyebrow')).toHaveCount(0)

  await page.getByRole('link', { name: 'Koncepty', exact: true }).click()
  await page.getByRole('button', { name: 'Pridať koncept' }).click()
  await page.getByLabel('Názov').fill(`Stránka bez štítku ${Date.now()}`)
  await page.getByRole('button', { name: 'Vytvoriť draft' }).click()
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

test('matches change history bottom spacing to the sidebar search button', async ({ page }) => {
  await login(page)

  const historyLink = page.getByRole('link', { name: 'História zmien', exact: true })
  const searchButton = page.locator('.sidebar > .sidebar-search')
  const [historyMargin, searchMargin] = await Promise.all([
    historyLink.evaluate((element) => getComputedStyle(element).marginBottom),
    searchButton.evaluate((element) => getComputedStyle(element).marginBottom),
  ])

  expect(historyMargin).toBe(searchMargin)
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
  await expect(page.locator('.login-submit')).toHaveCSS('border-radius', '13px')
  await page.screenshot({ path: '../outputs/viki-friendly-login.png', fullPage: true })

  await login(page)
  await expect(page.locator('.nav-item').first()).toHaveCSS('height', '44px')
  await expect(page.locator('.page-heading h1')).toHaveCSS('font-weight', '800')
  expect(await page.locator('.page-heading h1').evaluate((element) => parseFloat(getComputedStyle(element).fontSize))).toBeGreaterThanOrEqual(48)
  await expect(page.locator('.primary-button').first()).toHaveCSS('border-radius', '13px')
  await expect(page.locator('.panel').first()).toHaveCSS('border-top-width', '2px')
  await expect(page.locator('.panel').first()).toHaveCSS('border-radius', '18px')
  expect(await page.locator('.panel').first().evaluate((element) => getComputedStyle(element).boxShadow)).not.toBe('none')
  await page.screenshot({ path: '../outputs/viki-friendly-koncepty.png', fullPage: true })
})

test('gives the library search field the same raised shading as the status filter', async ({ page }) => {
  await login(page)

  const searchShadow = await page.locator('.search-field').evaluate((element) => getComputedStyle(element).boxShadow)
  const filterShadow = await page.locator('.filter-select .viki-select-trigger').evaluate((element) => getComputedStyle(element).boxShadow)

  expect(searchShadow).toBe(filterShadow)
  expect(searchShadow).not.toBe('none')
  await page.screenshot({ path: '../outputs/viki-shaded-search-field.png', fullPage: true })
})

test('matches each library add button to the status-filter width', async ({ page }) => {
  await login(page)

  for (const [destination, addLabel] of [['Koncepty', 'Pridať koncept'], ['Funkcie', 'Pridať funkciu']] as const) {
    await page.getByRole('link', { name: destination, exact: true }).click()
    const addButton = page.getByRole('button', { name: addLabel, exact: true })
    const filterButton = page.getByRole('button', { name: /Filtrovať podľa stavu/ })
    const [addBox, filterBox] = await Promise.all([addButton.boundingBox(), filterButton.boundingBox()])

    expect(addBox).not.toBeNull()
    expect(filterBox).not.toBeNull()
    expect(addBox!.width).toBeCloseTo(filterBox!.width, 0)
    await page.screenshot({ path: `../outputs/viki-${destination === 'Koncepty' ? 'concepts' : 'features'}-matching-control-widths.png`, fullPage: true })
  }

  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/concepts')
  const [mobileAdd, mobileFilter] = await Promise.all([
    page.getByRole('button', { name: 'Pridať koncept', exact: true }).boundingBox(),
    page.getByRole('button', { name: /Filtrovať podľa stavu/ }).boundingBox(),
  ])
  expect(mobileAdd).not.toBeNull()
  expect(mobileFilter).not.toBeNull()
  expect(mobileAdd!.width).toBeCloseTo(mobileFilter!.width, 0)
})

test('keeps select option panels the same width as their triggers', async ({ page }) => {
  const conversationId = 'select-width-conversation'
  const now = new Date().toISOString()
  const conversation = {
    id: conversationId,
    title: 'Šírka výberu',
    primaryMode: 'qa',
    lastMode: 'qa',
    state: 'idle',
    createdAt: now,
    updatedAt: now,
  }
  await page.route('**/api/v1/assistant/**', async (route) => {
    const request = route.request()
    const pathname = new URL(request.url()).pathname
    if (pathname === '/api/v1/assistant/status') {
      return route.fulfill({ json: { available: true, qa: { mode: 'qa', connected: true, configured: true, ready: true }, edit: { mode: 'edit', connected: true, configured: true, ready: true } } })
    }
    if (pathname === '/api/v1/assistant/conversations' && request.method() === 'GET') return route.fulfill({ json: { conversations: [conversation] } })
    if (pathname === `/api/v1/assistant/conversations/${conversationId}` && request.method() === 'GET') return route.fulfill({ json: { ...conversation, messages: [] } })
    if (pathname === `/api/v1/assistant/conversations/${conversationId}/events`) return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 60000\n\n' })
    return route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'not found' } } })
  })
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
  await page.getByRole('button', { name: 'Pridať koncept' }).click()

  await expect(page.locator('select')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Vytvoriť koncept' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Typ stránky' })).toHaveCount(0)
  const trigger = page.getByRole('button', { name: 'Druh konceptu' })
  await expect(trigger).toHaveText('Podstatné meno')
  await trigger.click()

  const menu = page.getByRole('listbox', { name: 'Druhy konceptov' })
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

  for (const destination of ['Koncepty', 'Funkcie']) {
    await page.getByRole('link', { name: destination, exact: true }).click()
    await expect(page.locator('.lucide-chevron-right')).toHaveCount(0)
    if (destination === 'Koncepty') await page.screenshot({ path: '../outputs/viki-concepts-without-row-chevrons.png', fullPage: true })
  }
})

test('does not expose provenance fields on wiki pages or in the editor', async ({ page }) => {
  await login(page)
  const title = `Koncept bez pôvodu ${Date.now()}`
  await createDraftConcept(page, title)

  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible()
  await expect(page.locator('.source-card')).toHaveCount(0)
  await expect(page.getByText('Pôvod obsahu')).toHaveCount(0)
  await page.screenshot({ path: '../outputs/viki-page-without-provenance.png', fullPage: true })

  await page.getByRole('button', { name: 'Zmeniť', exact: true }).click()
  await expect(page.getByLabel('Zdroje')).toHaveCount(0)
  await expect(page.getByLabel('Pôvod informácie')).toHaveCount(0)
  await page.screenshot({ path: '../outputs/viki-editor-without-provenance.png', fullPage: true })
})

test('edits pages inline from the right side of revision metadata', async ({ page }) => {
  await login(page)
  await createDraftConcept(page, `Koncept na úpravu ${Date.now()}`)

  const metadata = page.locator('.document-header .revision-meta')
  const edit = page.locator('.document-header').getByRole('button', { name: 'Zmeniť', exact: true })
  const metadataTextBox = await metadata.evaluate((element) => {
    const range = document.createRange()
    range.selectNodeContents(element)
    const box = range.getBoundingClientRect()
    return { x: box.x, width: box.width }
  })
  const editBox = await edit.boundingBox()

  expect(editBox).not.toBeNull()
  expect(editBox!.x).toBeGreaterThan(metadataTextBox.x + metadataTextBox.width)
  await page.screenshot({ path: '../outputs/viki-edit-action-in-document-header.png', fullPage: true })

  await edit.click()
  await expect(page.locator('.document-page > .page-editor')).toBeVisible()
  await expect(page.locator('.document-tools .page-editor')).toHaveCount(0)
  await expect(page.locator('.document-tools .review-panel')).toBeVisible()
  await page.screenshot({ path: '../outputs/viki-inline-page-editor.png', fullPage: true })
})

test('reveals the new-version action from an approved status row', async ({ page }) => {
  const now = '2026-08-02T10:00:00Z'
  const pageId = 'approved-hover-page'
  const revisionId = 'approved-hover-version'
  await page.route(`**/api/v1/pages/${pageId}`, (route) => route.fulfill({ json: {
    page: {
      id: pageId,
      kind: 'concept',
      conceptKind: 'noun',
      slug: 'schvaleny-koncept',
      title: 'Schválený koncept',
      approvedRevisionId: revisionId,
      approved: true,
      hasDraft: false,
      unresolvedObjections: 0,
      createdAt: now,
      updatedAt: now,
    },
    approvedRevision: {
      id: revisionId,
      pageId,
      number: 1,
      status: 'approved',
      title: 'Schválený koncept',
      bodyMd: 'Schválený obsah.',
            steps: [],
      references: [],
      createdBy: { id: 'user-1', email: initialUser, displayName: 'Matej', createdAt: now },
      createdAt: now,
    },
    revisions: [],
    comments: [],
    objections: [],
    children: [],
    reviewStates: [{ revisionId, state: 'approved', blockers: [] }],
  } }))
  await login(page)
  await page.goto(`/page/${pageId}`)

  const status = page.locator('.review-status-line.approved.current')
  const statusAction = status.getByRole('button', { name: 'Nová verzia' })
  await expect(statusAction).toHaveCSS('opacity', '0')
  await status.hover()
  await expect(statusAction).toHaveCSS('opacity', '1')
  await page.screenshot({ path: '../outputs/viki-approved-new-version-hover.png', fullPage: true })
  await statusAction.click()
  await expect(page.locator('.document-page > .page-editor')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Uložiť novú verziu' })).toBeVisible()
})

test('centers the assistant mode controls', async ({ page }) => {
  await login(page)
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()

  await expect(page.locator('.assistant-controls')).toHaveCSS('justify-content', 'center')
  await page.screenshot({ path: '../outputs/viki-centered-assistant-controls.png', fullPage: true })
})

test('starts and stops Slovak dictation from every logged-in screen with the keyboard shortcut', async ({ page }) => {
  const conversationId = 'voice-shortcut-conversation'
  const now = new Date().toISOString()
  const conversation = {
    id: conversationId,
    title: 'Hlasový vstup',
    primaryMode: 'qa',
    lastMode: 'qa',
    state: 'idle',
    createdAt: now,
    updatedAt: now,
  }
  await page.route('**/api/v1/assistant/**', async (route) => {
    const request = route.request()
    const pathname = new URL(request.url()).pathname
    if (pathname === '/api/v1/assistant/status') {
      return route.fulfill({ json: { available: true, qa: { mode: 'qa', connected: true, configured: true, ready: true }, edit: { mode: 'edit', connected: true, configured: true, ready: true } } })
    }
    if (pathname === '/api/v1/assistant/conversations' && request.method() === 'GET') return route.fulfill({ json: { conversations: [conversation] } })
    if (pathname === `/api/v1/assistant/conversations/${conversationId}` && request.method() === 'GET') return route.fulfill({ json: { ...conversation, messages: [] } })
    if (pathname === `/api/v1/assistant/conversations/${conversationId}/events`) return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 60000\n\n' })
    return route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'not found' } } })
  })
  await page.addInitScript(() => {
    class FakeSpeechRecognition {
      static latest: FakeSpeechRecognition | null = null
      lang = ''
      continuous = false
      interimResults = false
      starts = 0
      stops = 0
      aborts = 0
      onresult: ((event: { resultIndex: number; results: ArrayLike<{ readonly 0: { transcript: string } }> }) => void) | null = null
      onerror: ((event: { error: string }) => void) | null = null
      onend: (() => void) | null = null

      constructor() { FakeSpeechRecognition.latest = this }
      start() { this.starts += 1 }
      stop() { this.stops += 1; this.onend?.() }
      abort() { this.aborts += 1; this.onend?.() }
      emit(transcript: string) {
        this.onresult?.({ resultIndex: 0, results: [{ 0: { transcript } }] })
      }
    }

    Object.defineProperty(window, 'SpeechRecognition', { configurable: true, value: FakeSpeechRecognition })
    Object.defineProperty(window, 'webkitSpeechRecognition', { configurable: true, value: FakeSpeechRecognition })
    Object.defineProperty(window, '__vikiSpeech', {
      configurable: true,
      value: {
        state: () => {
          const instance = FakeSpeechRecognition.latest
          return instance && { lang: instance.lang, starts: instance.starts, stops: instance.stops, aborts: instance.aborts }
        },
        emit: (transcript: string) => FakeSpeechRecognition.latest?.emit(transcript),
      },
    })
  })
  await login(page)

  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()
  await expect(page.getByRole('button', { name: 'Začať hlasový vstup' })).toBeEnabled()
  await page.getByRole('button', { name: 'Zavrieť asistenta' }).click()

  await page.keyboard.press('Meta+Shift+M')
  await expect(page.locator('.assistant-drawer')).toBeVisible()
  await expect(page.getByText('Počúvam po slovensky…')).toBeVisible()
  expect(await page.evaluate(() => (window as typeof window & { __vikiSpeech: { state: () => unknown } }).__vikiSpeech.state())).toMatchObject({ lang: 'sk-SK', starts: 1 })

  await page.keyboard.press('Control+Shift+M')
  await expect(page.getByText('⌘⇧M diktuje · Enter odosiela')).toBeVisible()
  expect(await page.evaluate(() => (window as typeof window & { __vikiSpeech: { state: () => unknown } }).__vikiSpeech.state())).toMatchObject({ stops: 1 })

  await page.keyboard.press('Meta+Shift+M')
  await page.evaluate(() => (window as typeof window & { __vikiSpeech: { emit: (transcript: string) => void } }).__vikiSpeech.emit('Skúšobný text'))
  await expect(page.locator('.assistant-composer textarea')).toHaveValue('Skúšobný text')
  await page.keyboard.press('Escape')
  await expect(page.locator('.assistant-composer textarea')).toHaveValue('')
  await expect(page.locator('.assistant-drawer')).toBeVisible()
  await page.screenshot({ path: '../outputs/viki-app-wide-dictation-shortcut.png', fullPage: true })
})

test('does not show preset assistant prompts', async ({ page }) => {
  await login(page)
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()

  await expect(page.getByRole('button', { name: 'Čo treba pre zmluvu?' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Vytvoriť scenár' })).toHaveCount(0)
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

test('localizes concept pages as Koncepty and Koncept', async ({ page }) => {
  await login(page)
  await expect(page.getByRole('link', { name: 'Koncepty', exact: true })).toBeVisible()
  await expect(page.locator('body')).not.toContainText(/primitív/i)

  await page.getByRole('link', { name: 'Koncepty', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Koncepty', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Pridať koncept' })).toBeVisible()
  await expect(page.getByPlaceholder('Hľadať v konceptoch…')).toBeVisible()

  await page.getByRole('button', { name: 'Pridať koncept' }).click()
  await expect(page.getByRole('heading', { name: 'Vytvoriť koncept' })).toBeVisible()
  await expect(page.getByLabel('Typ stránky')).toHaveCount(0)
  await expect(page.getByLabel('Druh konceptu')).toBeVisible()
  await page.getByLabel('Názov').fill(`Testovací koncept ${Date.now()}`)
  await page.getByRole('button', { name: 'Vytvoriť draft' }).click()
  await expect(page.locator('.document-breadcrumb')).toContainText('Koncepty')
  await expect(page.locator('.document-header .eyebrow')).toHaveCount(0)
  await page.screenshot({ path: '../outputs/viki-koncepty.png', fullPage: true })
})

test('the single initial user can authenticate', async ({ page }) => {
  await login(page)
  await expect(page.locator('.sidebar-user')).toContainText(initialUser)
})

test('removes Prehľad and uses Koncepty as the root screen', async ({ page }) => {
  await page.goto('/')
  await page.getByLabel('E-mail').fill(initialUser)
  await page.getByLabel('Heslo').fill(password)
  await page.getByRole('button', { name: /Prihlásiť sa/ }).click()

  await expect(page.getByRole('heading', { name: 'Koncepty', exact: true })).toBeVisible()
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

test('removes the duplicate feature shortcut section from the sidebar', async ({ page }) => {
  await login(page)

  await expect(page.locator('.sidebar-section')).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Funkcie', exact: true })).toBeVisible()
  await page.screenshot({ path: '../outputs/viki-without-scenario-shortcuts.png', fullPage: true })
})

test('removes the global new-page button from the sidebar', async ({ page }) => {
  await login(page)

  await expect(page.getByRole('button', { name: 'Nová stránka' })).toHaveCount(0)
  await expect(page.locator('.new-page-button')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Pridať koncept' })).toBeVisible()
  await page.screenshot({ path: '../outputs/viki-sidebar-without-new-page.png', fullPage: true })
})

test('assistant draft receipts in chat link to real page revisions', async ({ page }) => {
  const conversationId = '00000000-0000-4000-8000-000000000071'
  const revisionId = '00000000-0000-4000-8000-000000000073'
  const pageId = '00000000-0000-4000-8000-000000000074'
  const now = '2026-07-31T10:00:00Z'
  const summary = {
    id: conversationId,
    title: 'Zmluva',
    primaryMode: 'edit',
    lastMode: 'edit',
    state: 'idle',
    createdAt: now,
    updatedAt: now,
  }
  const receipt = { revisionId, pageId, pageTitle: 'Zmluva' }
  const content = {
    title: 'Zmluva',
    bodyMd: 'Dohoda medzi spoločnosťou a zákazníkom.',
        steps: [],
    references: [],
  }

  await page.route('**/api/v1/assistant/**', async (route) => {
    const request = route.request()
    const pathname = new URL(request.url()).pathname
    if (pathname === '/api/v1/assistant/status') {
      return route.fulfill({ json: { available: true, qa: { mode: 'qa', connected: true, configured: true, ready: true }, edit: { mode: 'edit', connected: true, configured: true, ready: true } } })
    }
    if (pathname === '/api/v1/assistant/conversations' && request.method() === 'GET') return route.fulfill({ json: { conversations: [summary] } })
    if (pathname === `/api/v1/assistant/conversations/${conversationId}` && request.method() === 'GET') {
      return route.fulfill({ json: {
        ...summary,
        messages: [{
          id: 'message-1',
          role: 'assistant',
          mode: 'edit',
          content: 'Vytvoril som draft.',
          citations: [],
          drafts: [receipt],
          createdAt: now,
        }],
      } })
    }
    if (pathname === `/api/v1/assistant/conversations/${conversationId}/events`) {
      return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 60000\n\n' })
    }
    return route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'not found' } } })
  })
  await page.route(`**/api/v1/pages/${pageId}`, (route) => route.fulfill({ json: {
    page: {
      id: pageId, kind: 'concept', conceptKind: 'noun', slug: 'zmluva', title: 'Zmluva',
      approved: false, hasDraft: true, unresolvedObjections: 0,
      latestDraftRevisionId: revisionId, createdAt: now, updatedAt: now,
    },
    draftRevision: {
      ...content,
      id: revisionId, pageId, number: 1, status: 'draft',
      createdBy: { id: '00000000-0000-4000-8000-000000000011', email: initialUser, displayName: 'Matej', createdAt: now },
      createdAt: now,
    },
    revisions: [], comments: [], objections: [], children: [],
    reviewStates: [{ revisionId, state: 'ready', blockers: [] }],
  } }))

  await login(page)
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()

  const receiptLink = page.locator('.created-draft')
  await expect(receiptLink).toContainText('Draft vytvorený')
  await expect(receiptLink).toContainText('Zmluva')
  await expect(receiptLink).toHaveAttribute('href', `/page/${pageId}?revision=${revisionId}`)
  await expect(page).not.toHaveURL(/\/drafts(?:\/|$)/)

  await receiptLink.click()
  await expect(page).toHaveURL(`/page/${pageId}?revision=${revisionId}`)
  await expect(page.getByRole('heading', { name: 'Zmluva', exact: true })).toBeVisible()
})

test('assistant Edit turn streams progress and then opens the new Feature draft', async ({ page }) => {
  const conversationId = '00000000-0000-4000-8000-000000000081'
  const turnId = '00000000-0000-4000-8000-000000000082'
  const concept = { revisionId: '00000000-0000-4000-8000-000000000083', pageId: '00000000-0000-4000-8000-000000000084', pageTitle: 'Zákazník' }
  const scenario = { revisionId: '00000000-0000-4000-8000-000000000085', pageId: '00000000-0000-4000-8000-000000000086', pageTitle: 'Úspešná rezervácia' }
  const feature = { revisionId: '00000000-0000-4000-8000-000000000087', pageId: '00000000-0000-4000-8000-000000000088', pageTitle: 'Rezervácia internetovej služby' }
  const now = '2026-08-03T08:00:00Z'
  const summary = {
    id: conversationId,
    title: 'Rezervácia služby',
    primaryMode: 'edit',
    lastMode: 'edit',
    state: 'idle',
    createdAt: now,
    updatedAt: now,
  }
  const pages = [
    { id: concept.pageId, kind: 'concept', conceptKind: 'noun', slug: 'zakaznik', title: concept.pageTitle },
    { id: scenario.pageId, kind: 'scenario', parentId: feature.pageId, slug: 'uspesna-rezervacia', title: scenario.pageTitle },
    { id: feature.pageId, kind: 'feature', slug: 'rezervacia-internetovej-sluzby', title: feature.pageTitle },
  ].map((item) => ({ ...item, approved: false, hasDraft: true, unresolvedObjections: 0, createdAt: now, updatedAt: now }))
  let releaseProgress!: () => void
  let releaseCompletion!: () => void
  const progressReady = new Promise<void>((resolve) => { releaseProgress = resolve })
  const completionReady = new Promise<void>((resolve) => { releaseCompletion = resolve })
  let streamConnection = 0
  let submitted = false
  let finished = false

  await page.route('**/api/v1/assistant/**', async (route) => {
    const request = route.request()
    const pathname = new URL(request.url()).pathname
    if (pathname === '/api/v1/assistant/status') {
      return route.fulfill({ json: { available: true, qa: { mode: 'qa', connected: true, configured: true, ready: true }, edit: { mode: 'edit', connected: true, configured: true, ready: true } } })
    }
    if (pathname === '/api/v1/assistant/conversations' && request.method() === 'GET') return route.fulfill({ json: { conversations: [summary] } })
    if (pathname === `/api/v1/assistant/conversations/${conversationId}` && request.method() === 'GET') {
      return route.fulfill({ json: {
        ...summary,
        state: submitted && !finished ? 'running' : 'idle',
        messages: finished ? [{
          id: `${turnId}-assistant`, role: 'assistant', mode: 'edit', content: 'Pripravil som funkciu rezervácie a jej prvý scenár.',
          citations: [], drafts: [concept, scenario, feature], createdAt: now,
        }] : [],
      } })
    }
    if (pathname === `/api/v1/assistant/conversations/${conversationId}/messages` && request.method() === 'POST') {
      submitted = true
      releaseProgress()
      return route.fulfill({ status: 202, json: { turnId, mode: 'edit' } })
    }
    if (pathname === `/api/v1/assistant/conversations/${conversationId}/events`) {
      streamConnection += 1
      if (streamConnection === 1) {
        await progressReady
        await new Promise((resolve) => setTimeout(resolve, 80))
        const events = [
          ['activity', { turnId, mode: 'edit', state: 'submitted', label: '' }],
          ['activity', { turnId, mode: 'edit', state: 'searching', label: '' }],
          ['activity', { turnId, mode: 'edit', state: 'searched', label: '' }],
          ['activity', { turnId, mode: 'edit', state: 'reading', label: '' }],
          ['activity', { turnId, mode: 'edit', state: 'read', label: '' }],
          ['activity', { turnId, mode: 'edit', state: 'drafting', label: '' }],
          ['message_delta', { turnId, mode: 'edit', delta: 'Pripravil som funkciu rezervácie a jej prvý scenár.' }],
          ['draft_created', { turnId, mode: 'edit', draft: concept }],
          ['draft_created', { turnId, mode: 'edit', draft: scenario }],
          ['draft_created', { turnId, mode: 'edit', draft: feature }],
          ['activity', { turnId, mode: 'edit', state: 'drafted', label: '' }],
        ].map(([event, data], index) => `id: ${index + 1}\nevent: ${event}\ndata: ${JSON.stringify(data)}\n`).join('\n')
        return route.fulfill({ status: 200, contentType: 'text/event-stream', body: `${events}\nretry: 100\n\n` })
      }
      if (streamConnection === 2) {
        await completionReady
        finished = true
        return route.fulfill({ status: 200, contentType: 'text/event-stream', body: `id: 12\nevent: completed\ndata: ${JSON.stringify({ turnId, mode: 'edit' })}\n\nretry: 60000\n\n` })
      }
      return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 60000\n\n' })
    }
    return route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'not found' } } })
  })
  await page.route('**/api/v1/pages**', async (route) => {
    const pathname = new URL(route.request().url()).pathname
    if (pathname === '/api/v1/pages') return route.fulfill({ json: { pages } })
    if (pathname === `/api/v1/pages/${feature.pageId}`) return route.fulfill({ json: {
      page: { ...pages[2], latestDraftRevisionId: feature.revisionId },
      draftRevision: {
        id: feature.revisionId, pageId: feature.pageId, number: 1, status: 'draft', title: feature.pageTitle,
        bodyMd: 'Rezervácia internetovej služby.', steps: [], references: [],
        createdBy: { id: 'user-1', email: initialUser, displayName: 'Matej', createdAt: now }, createdAt: now,
      },
      revisions: [], comments: [], objections: [], children: [],
      reviewStates: [{ revisionId: feature.revisionId, state: 'ready', blockers: [] }],
    } })
    return route.continue()
  })

  await login(page)
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()
  await page.getByPlaceholder('Opíšte, čo má viki pridať alebo zmeniť…').fill('Pridaj funkciu rezervácie internetovej služby.')
  await page.getByRole('button', { name: 'Odoslať' }).click()

  await expect(page).toHaveURL(`/assistant/turns/${turnId}`)
  await expect(page.getByRole('heading', { name: 'Viki pripravuje zmeny' })).toBeVisible()
  await expect(page.getByText('Našiel som súvisiace stránky')).toBeVisible()
  await expect(page.getByText('Pripravil som funkciu rezervácie a jej prvý scenár.').first()).toBeVisible()
  await expect(page.locator('.assistant-turn-drafts a')).toHaveCount(3)
  expect(await page.locator('.assistant-turn-grid').evaluate((element) => element.scrollWidth)).toBeLessThanOrEqual(
    await page.locator('.assistant-turn-grid').evaluate((element) => element.clientWidth),
  )
  await page.screenshot({ path: '../outputs/viki-assistant-live-edit.png', fullPage: true })

  releaseCompletion()
  await expect(page).toHaveURL(`/page/${feature.pageId}?revision=${feature.revisionId}`)
  await expect(page.getByRole('heading', { name: feature.pageTitle, exact: true })).toBeVisible()
})

test('does not seed the AIRNET wiki corpus', async ({ page }) => {
  await login(page)
  const pages = await (await page.context().request.get('/api/v1/pages')).json()
  expect(pages.pages.map((item: { title: string }) => item.title)).not.toContain('Rezervácia internetovej služby')
})

test('keeps related pages collapsed until their accordion is opened', async ({ page }) => {
  const now = '2026-08-01T10:00:00Z'
  const pageId = 'accordion-page'
  const revisionId = 'accordion-revision'
  await page.route(`**/api/v1/pages/${pageId}`, (route) => route.fulfill({ json: {
    page: {
      id: pageId,
      kind: 'feature',
      slug: 'accordion-feature',
      title: 'Accordion feature',
      approvedRevisionId: revisionId,
      approved: true,
      hasDraft: false,
      unresolvedObjections: 0,
      createdAt: now,
      updatedAt: now,
    },
    approvedRevision: {
      id: revisionId,
      pageId,
      number: 1,
      status: 'approved',
      title: 'Accordion feature',
      bodyMd: 'Feature body.',
            steps: [],
      references: [{ targetPageId: 'contract-page', targetTitle: 'Zmluva', relation: 'uses' }],
      createdBy: { id: 'user-1', email: initialUser, displayName: 'Matej', createdAt: now },
      createdAt: now,
    },
    revisions: [],
    comments: [],
    objections: [],
    children: [],
    reviewStates: [{ revisionId, state: 'approved', blockers: [] }],
  } }))
  await login(page)
  await page.goto(`/page/${pageId}`)

  const toggle = page.getByRole('button', { name: 'Súvisiace stránky' })
  const content = page.locator(`#related-pages-${revisionId}`)
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await expect(content).toBeHidden()

  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await expect(content).toBeVisible()
  await expect(content.getByRole('link', { name: /Zmluva/ })).toBeVisible()
})

test('rejection blocks approval, resolution permits it, and approved content stays visible beside a draft', async ({ page }) => {
  await login(page)
  const suffix = Date.now()
  await page.getByRole('button', { name: 'Pridať koncept' }).click()
  await page.getByLabel('Názov').fill(`Testovacie pravidlo ${suffix}`)
  await page.getByRole('button', { name: 'Vytvoriť draft' }).click()
  await expect(page.getByRole('heading', { name: 'Kontrola' })).toBeVisible()
  const headerStateIcon = page.locator('.document-title-row > .document-icon')
  await expect(headerStateIcon).toHaveAttribute('aria-label', 'Koncept · Draft')
  await expect(headerStateIcon).toHaveClass(/draft/)
  await expect(headerStateIcon).toHaveCSS('background-color', 'rgb(243, 243, 243)')
  await expect(page.locator('.document-title-main .document-icon')).toHaveCount(0)
  await expect(page.locator('.document-title-row > .status-badge')).toHaveCount(0)

  await page.getByRole('button', { name: 'Vzniesť námietku' }).click()
  const reject = page.getByRole('button', { name: 'Odoslať námietku' })
  await expect(reject).toBeDisabled()
  await page.getByLabel('Dôvod námietky').fill('Chýba presná definícia konceptu.')
  await reject.click()
  await expect(page.getByText('Chýba presná definícia konceptu.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Schváliť' })).toBeDisabled()
  await page.locator('.review-status-accordion > summary').click()
  await page.getByRole('button', { name: 'Označiť ako vyriešené' }).click()
  await page.getByRole('button', { name: 'Schváliť' }).click()
  await expect(page.getByText('Verzia #1')).toBeVisible()
  await expect(headerStateIcon).toHaveAttribute('aria-label', 'Koncept · Schválené')
  await expect(headerStateIcon).toHaveClass(/approved/)
  await expect(headerStateIcon).toHaveCSS('background-color', 'rgb(199, 237, 137)')
  await expect(headerStateIcon).toHaveCSS('border-color', 'rgb(167, 203, 104)')

  await page.locator('.document-header').getByRole('button', { name: 'Nová verzia' }).click()
  await expect(page.locator('.page-editor > label')).toHaveCount(2)
  await expect(page.getByText('Obsahuje ilustračné, neoverené pravidlá')).toHaveCount(0)
  await page.getByLabel('Obsah stránky').fill('Toto je nový obsah, ktorý ešte nebol schválený.')
  await page.getByRole('button', { name: 'Uložiť novú verziu' }).click()
  await expect(page.getByRole('tab', { name: 'Schválené' })).toBeVisible()
  await page.getByRole('tab', { name: 'Schválené' }).click()
  await expect(page.getByText('Táto stránka zatiaľ nemá opis.')).toBeVisible()
  await page.getByRole('tab', { name: /Draft #2/ }).click()
  await expect(page.getByText('Toto je nový obsah, ktorý ešte nebol schválený.')).toBeVisible()
})

test('optimistic concurrency returns a recoverable 409', async ({ page }) => {
  await login(page)
  const suffix = Date.now()
  const csrf = await page.evaluate(() => decodeURIComponent(document.cookie.split('; ').find((cookie) => cookie.startsWith('viki_csrf='))?.split('=').slice(1).join('=') ?? ''))
  const content = { title: `Súbežný koncept ${suffix}`, bodyMd: '', steps: [], references: [] }
  const created = await page.context().request.post('/api/v1/pages', { headers: { 'X-CSRF-Token': csrf }, data: { kind: 'concept', conceptKind: 'noun', slug: `subezny-koncept-${suffix}`, content } })
  expect(created.status()).toBe(201)
  const detail = await created.json()
  const baseRevisionId = detail.draftRevision.id
  const first = await page.context().request.post(`/api/v1/pages/${detail.page.id}/revisions`, { headers: { 'X-CSRF-Token': csrf }, data: { baseRevisionId, content: { ...content, bodyMd: 'Prvý zápis.' } } })
  expect(first.status()).toBe(201)
  const stale = await page.context().request.post(`/api/v1/pages/${detail.page.id}/revisions`, { headers: { 'X-CSRF-Token': csrf }, data: { baseRevisionId, content: { ...content, bodyMd: 'Konfliktný zápis.' } } })
  expect(stale.status()).toBe(409)
  await expect(stale.json()).resolves.toMatchObject({ error: { code: 'revision_conflict' } })
})

test('users can manually create a feature and its independently versioned scenario', async ({ page }) => {
  await login(page)
  const suffix = Date.now()
  const featureTitle = `E2E funkcia ${suffix}`
  await page.getByLabel('Hlavná navigácia').getByRole('link', { name: 'Funkcie', exact: true }).click()
  await page.getByRole('button', { name: 'Pridať funkciu' }).click()
  await expect(page.getByRole('heading', { name: 'Vytvoriť funkciu' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Typ stránky' })).toHaveCount(0)
  await page.getByLabel('Názov', { exact: true }).fill(featureTitle)
  const scenarioTitle = `E2E scenár ${suffix}`
  await page.getByLabel('Názov scenára').fill(scenarioTitle)
  await proposeInitialScenarioSteps(page, [
    'zákazník má pripravenú zmluvu',
    'zákazník zmluvu podpíše',
    'systém uloží podpis',
  ])
  await page.getByRole('button', { name: 'Vytvoriť draft' }).click()
  await expect(page.getByRole('heading', { name: featureTitle })).toBeVisible()
  const featureUrl = page.url()

  await page.getByRole('link', { name: new RegExp(scenarioTitle) }).click()
  await expect(page.getByRole('heading', { name: scenarioTitle })).toBeVisible()
  await expect(page.getByRole('link', { name: featureTitle })).toHaveAttribute('href', /\/page\//)
  const scenarioUrl = page.url()
  await page.locator('.review-status-accordion > summary').click()
  await expect(page.getByText('Najprv schváľte funkciu')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Schváliť' })).toBeDisabled()

  await page.goto(featureUrl)
  await page.getByRole('button', { name: 'Schváliť' }).click()
  await expect(page.getByRole('img', { name: 'Funkcia · Schválené' })).toBeVisible()

  await page.goto(scenarioUrl)
  await expect(page.getByText('Najprv schváľte funkciu')).toHaveCount(0)
  await page.getByRole('button', { name: 'Schváliť' }).click()
  await expect(page.getByRole('img', { name: 'Scenár · Schválené' })).toBeVisible()

  await page.goto(featureUrl)
  await page.locator('.document-header').getByRole('button', { name: 'Nová verzia', exact: true }).click()
  await page.getByLabel('Obsah stránky').fill('Nezávislý draft nadradenej funkcie.')
  await page.getByRole('button', { name: 'Uložiť novú verziu' }).click()
  await expect(page.getByRole('tab', { name: /Draft #2/ })).toBeVisible()

  await page.goto(scenarioUrl)
  await expect(page.getByRole('img', { name: 'Scenár · Schválené' })).toBeVisible()
  await expect(page.getByRole('tab', { name: /Draft/ })).toHaveCount(0)
})

test('page detail actions use one rendered height', async ({ page }) => {
  await login(page)
  const suffix = Date.now()
  const featureTitle = `E2E výška tlačidiel ${suffix}`
  await page.getByLabel('Hlavná navigácia').getByRole('link', { name: 'Funkcie', exact: true }).click()
  await page.getByRole('button', { name: 'Pridať funkciu' }).click()
  await page.getByLabel('Názov', { exact: true }).fill(featureTitle)
  await page.getByLabel('Názov scenára').fill(`E2E scenár ${suffix}`)
  await proposeInitialScenarioSteps(page, [
    'existuje funkcia',
    'používateľ otvorí detail',
    'akcie majú rovnakú výšku',
  ])
  await page.getByRole('button', { name: 'Vytvoriť draft' }).evaluate((button: HTMLButtonElement) => button.click())
  await expect(page.getByRole('heading', { name: featureTitle })).toBeVisible()

  const buttons = [
    page.locator('.document-header-actions').getByRole('button', { name: 'Zmeniť' }),
    page.locator('.document-section-heading').getByRole('button', { name: 'Pridať scenár' }),
    page.locator('.document-header-actions').getByRole('button', { name: 'História' }),
  ]
  const heights = await Promise.all(buttons.map((button) => button.evaluate((element) => element.getBoundingClientRect().height)))

  expect(new Set(heights).size).toBe(1)
})

test('wiki creation remains usable while the assistant is unavailable', async ({ page }) => {
  await page.route('**/api/v1/assistant/status', (route) => route.fulfill({ json: {
    available: false,
    qa: { mode: 'qa', connected: false, configured: false, ready: false },
    edit: { mode: 'edit', connected: false, configured: false, ready: false },
  } }))
  await login(page)
  await page.getByRole('button', { name: 'Otvoriť asistenta' }).click()
  await expect(page.getByText('Asistent je momentálne nedostupný.')).toBeVisible()

  const sourceTitle = `Koncept bez asistenta ${Date.now()}`
  await page.getByRole('button', { name: 'Pridať koncept' }).click()
  await page.getByLabel('Názov').fill(sourceTitle)
  await page.getByRole('button', { name: 'Vytvoriť draft' }).click()
  await expect(page.getByRole('heading', { name: sourceTitle })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Kontrola' })).toBeVisible()
})

test('mobile workspace navigation remains usable', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await login(page)
  await page.getByRole('button', { name: 'Otvoriť navigáciu' }).click()
  await page.getByRole('link', { name: 'Funkcie', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Funkcie', exact: true })).toBeVisible()
  await page.waitForFunction(() => document.querySelector('.sidebar')?.getBoundingClientRect().right === 0)
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  expect(overflow).toBeLessThanOrEqual(1)
  await page.screenshot({ path: '../outputs/viki-mobile.png', fullPage: true })
})

test('uses the concept and Gherkin feature routes and switches the application language', async ({ page }) => {
  await login(page)
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByRole('link', { name: 'Koncepty', exact: true })).toHaveAttribute('href', '/concepts')
  await page.goto('/features')

  await expect(page).toHaveURL(/\/features$/)
  await expect(page.getByRole('heading', { name: 'Funkcie', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Funkcie', exact: true })).toHaveAttribute('href', '/features')

  const brand = page.locator('.sidebar-brand')
  const wordmark = page.getByRole('link', { name: 'viki', exact: true })
  const languageToggle = page.locator('.sidebar-language')
  await expect(languageToggle).toHaveRole('switch')
  await expect(languageToggle).toHaveAttribute('aria-checked', 'false')
  const brandBox = await brand.boundingBox()
  const wordmarkBox = await wordmark.boundingBox()
  const toggleBox = await languageToggle.boundingBox()
  expect(brandBox).not.toBeNull()
  expect(wordmarkBox).not.toBeNull()
  expect(toggleBox).not.toBeNull()
  expect(toggleBox!.x).toBeGreaterThan(wordmarkBox!.x + wordmarkBox!.width)
  expect(toggleBox!.x + toggleBox!.width).toBeLessThanOrEqual(brandBox!.x + brandBox!.width)

  await languageToggle.click({ position: { x: 2, y: toggleBox!.height / 2 } })
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  await expect(languageToggle).toHaveAttribute('aria-checked', 'true')
  await expect(page.getByRole('heading', { name: 'Features', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Concepts', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Drafts', exact: true })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Change history', exact: true })).toBeVisible()

  await page.reload()
  await expect(page.getByRole('heading', { name: 'Features', exact: true })).toBeVisible()

  const reloadedToggleBox = await languageToggle.boundingBox()
  expect(reloadedToggleBox).not.toBeNull()
  await languageToggle.click({ position: { x: reloadedToggleBox!.width - 2, y: reloadedToggleBox!.height / 2 } })
  await expect(page.locator('html')).toHaveAttribute('lang', 'sk')
  await expect(languageToggle).toHaveAttribute('aria-checked', 'false')
  await expect(page.getByRole('heading', { name: 'Funkcie', exact: true })).toBeVisible()

  await page.goto('/scenarios')
  await expect(page.getByRole('heading', { name: 'Stránka sa nenašla' })).toBeVisible()

  await page.goto('/primitives')
  await expect(page.getByRole('heading', { name: 'Stránka sa nenašla' })).toBeVisible()
})
