import { render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { isPublicDemo } from './deployment'
import { I18nProvider, useI18n } from './i18n'

afterEach(() => vi.unstubAllEnvs())
function Locale() { return <output>{useI18n().locale}</output> }
it('uses an explicit public build flag and defaults the English-only demo to English', () => {
  vi.stubEnv('VITE_PUBLIC_DEMO', '')
  expect(isPublicDemo()).toBe(false)
  vi.stubEnv('VITE_PUBLIC_DEMO', 'true')
  expect(isPublicDemo()).toBe(true)
  render(<I18nProvider><Locale /></I18nProvider>)
  expect(screen.getByRole('status')).toHaveTextContent('en')
})
