import { useState, type FormEvent } from 'react'
import { ArrowRight } from 'lucide-react'
import { APIError } from '../api/client'
import { useAuth } from '../auth'

export function LoginPage() {
  const { login } = useAuth()
  const [email, setEmail] = useState('matej@matejlukasik.com')
  const [password, setPassword] = useState('password')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setLoading(true); setError('')
    try { await login(email, password) }
    catch (caught) { setError(caught instanceof APIError ? caught.message : 'Prihlásenie sa nepodarilo.') }
    finally { setLoading(false) }
  }
  return <div className="login-page">
    <section className="login-story">
      <div className="login-wordmark">viki</div>
    </section>
    <section className="login-form-wrap">
      <form className="login-form" onSubmit={submit}>
        <span className="mobile-login-brand">viki</span>
        <span className="eyebrow">Vitajte späť</span>
        <h2>Prihlásenie do pracovného priestoru</h2>
        <label>E-mail<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="username" required /></label>
        <label>Heslo<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required /></label>
        {error && <div className="form-error" role="alert">{error}</div>}
        <button className="primary-button login-submit" disabled={loading}>{loading ? 'Prihlasujem…' : <>Prihlásiť sa <ArrowRight size={17} /></>}</button>
      </form>
    </section>
  </div>
}
