import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import { I18nProvider, LanguageSwitcher, translate, useI18n } from './i18n'

function Probe() {
  const { locale, setLocale, t } = useI18n()
  return <><output aria-label="locale">{locale}</output><output aria-label="message">{t('review.revision', { number: 4 })}</output><button onClick={() => setLocale('en')}>set English</button></>
}

beforeEach(() => {
  const values = new Map<string, string>()
  Object.defineProperty(window, 'localStorage', { configurable: true, value: {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    removeItem: (key: string) => values.delete(key),
    clear: () => values.clear(),
  } })
  document.documentElement.lang = ''
})

describe('i18n', () => {
  it('uses Slovak fallback translations outside a provider and preserves missing placeholders', () => {
    render(<Probe />)
    fireEvent.click(screen.getByRole('button', { name: 'set English' }))
    expect(screen.getByLabelText('locale')).toHaveTextContent('sk')
    expect(screen.getByLabelText('message')).toHaveTextContent('Kontrola revízie #4')
    expect(translate('sk', 'review.revision')).toBe('Kontrola revízie #{number}')
    expect(translate('sk', 'review.revision', {})).toBe('Kontrola revízie #{number}')
    expect(translate('en', 'review.revision', { number: 7 })).toBe('Review revision #7')
  })

  it('loads the stored locale, persists changes, and toggles with click, Enter, and Space', async () => {
    const user = userEvent.setup()
    window.localStorage.setItem('viki.locale', 'en')
    render(<I18nProvider><LanguageSwitcher className="custom" /><Probe /></I18nProvider>)

    const english = screen.getByRole('switch', { name: 'Language' })
    expect(english).toHaveClass('custom')
    expect(english).toHaveAttribute('aria-checked', 'true')
    expect(document.documentElement.lang).toBe('en')
    await user.click(english)
    const slovak = screen.getByRole('switch', { name: 'Jazyk' })
    expect(slovak).toHaveAttribute('aria-checked', 'false')
    expect(window.localStorage.getItem('viki.locale')).toBe('sk')

    fireEvent.keyDown(slovak, { key: 'x' })
    expect(screen.getByLabelText('locale')).toHaveTextContent('sk')
    fireEvent.keyDown(slovak, { key: 'Enter' })
    expect(screen.getByLabelText('locale')).toHaveTextContent('en')
    fireEvent.keyDown(screen.getByRole('switch', { name: 'Language' }), { key: ' ' })
    expect(screen.getByLabelText('locale')).toHaveTextContent('sk')
  })

  it('defaults unknown stored values to Slovak', () => {
    window.localStorage.setItem('viki.locale', 'de')
    render(<I18nProvider><Probe /></I18nProvider>)
    expect(screen.getByLabelText('locale')).toHaveTextContent('sk')
  })
})
