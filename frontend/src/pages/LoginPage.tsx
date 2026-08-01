import { useState, type FormEvent } from 'react'
import { ArrowRight } from 'lucide-react'
import { APIError } from '../api/client'
import { useAuth } from '../auth'
import { LanguageSwitcher, useI18n } from '../i18n'

export function LoginPage() {
  const { t } = useI18n()
  const { login } = useAuth()
  const [email, setEmail] = useState('matej@matejlukasik.com')
  const [password, setPassword] = useState('password')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setLoading(true); setError('')
    try { await login(email, password) }
    catch (caught) { setError(caught instanceof APIError ? caught.message : t('login.failed')) }
    finally { setLoading(false) }
  }
  return <div className="login-page">
    <section className="login-story">
      <div className="login-wordmark">viki</div>
    </section>
    <section className="login-form-wrap">
      <LanguageSwitcher className="login-language" />
      <form className="login-form" onSubmit={submit}>
        <span className="mobile-login-brand">viki</span>
        <span className="eyebrow">{t('login.welcome')}</span>
        <h2>{t('login.title')}</h2>
        <label>{t('login.email')}<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="username" required /></label>
        <label>{t('login.password')}<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required /></label>
        {error && <div className="form-error" role="alert">{error}</div>}
        <button className="primary-button login-submit" disabled={loading}>{loading ? t('login.submitting') : <>{t('login.submit')} <ArrowRight size={17} /></>}</button>
      </form>
    </section>
  </div>
}
