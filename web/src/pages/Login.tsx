import { useState } from 'react'
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useI18n } from '../lib/i18n'
import { Field } from '../components/ui'
import { Icon } from '../components/icons'
import { Scramble } from '../lib/fx'

export default function Login({ mode }: { mode: 'login' | 'setup' | 'register' }) {
  const { status, user, refresh } = useApp()
  const { t } = useI18n()
  const navigate = useNavigate()
  const loc = useLocation()
  const [form, setForm] = useState({ email: '', password: '', name: '' })
  const [err, setErr] = useState(() => new URLSearchParams(window.location.search).get('error') || '')
  const [busy, setBusy] = useState(false)

  const from = (loc.state as { from?: string } | null)?.from || '/'
  if (status && user) return <Navigate to={from} replace />
  if (status && mode === 'setup' && !status.needs_setup) return <Navigate to="/login" replace />
  if (status && mode === 'login' && status.needs_setup) return <Navigate to="/setup" replace />
  if (status && mode === 'register' && status.needs_setup) return <Navigate to="/setup" replace />

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true); setErr('')
    try {
      await api.post(`${V1}/auth/${mode}`, form)
      await refresh()
      navigate(from, { replace: true })
    } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
  }
  const sso = mode === 'login' && !!status?.oidc
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm(f => ({ ...f, [k]: e.target.value }))
  const title = status?.title || '残像 Zanzō'

  return (
    <div className="login-wrap">
      <div className="login-card">
        <div className="login-brand">
          <span className="login-seal seal-stamp" aria-hidden>残</span>
          <h1><Scramble text={title} /></h1>
          <p className="login-tagline">{mode === 'setup' ? t('l.setup') : mode === 'register' ? t('l.register') : t('app.tagline')}</p>
        </div>
        {sso && <>
          <a className="btn primary lg block" href={`${V1}/auth/oidc/start?next=${encodeURIComponent(from)}`}>
            <Icon name="key" size={16} />{t('l.sign_in_with', { p: status?.oidc_name || 'single sign-on' })}
          </a>
          <div className="login-or"><span>{t('l.or_pass')}</span></div>
        </>}
        <form onSubmit={submit} className="stack">
          {(mode === 'setup' || mode === 'register') && <Field label={t('l.your_name')}><input value={form.name} onChange={set('name')} autoFocus autoComplete="name" /></Field>}
          <Field label={t('l.email')}><input type="email" value={form.email} onChange={set('email')} required autoFocus={mode === 'login' && !sso} autoComplete="username" /></Field>
          <Field label={t('l.password')} help={mode === 'setup' || mode === 'register' ? t('l.at_least_8') : undefined}>
            <input type="password" value={form.password} onChange={set('password')} required minLength={mode === 'setup' || mode === 'register' ? 8 : 1} autoComplete={mode === 'setup' || mode === 'register' ? 'new-password' : 'current-password'} />
          </Field>
          {err && <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div>}
          <button type="submit" className={`btn lg block ${sso ? '' : 'primary'}`} disabled={busy}>{busy ? t('l.wait') : mode === 'register' ? t('l.register') : mode === 'setup' ? t('l.create') : t('l.sign_in')}</button>
        </form>
        {mode === 'setup' && <p className="login-note">{t('l.note')}</p>}
        {mode === 'login' && !status?.needs_setup && <p className="login-note">{t('l.no_account')} <Link to="/register">{t('l.register')}</Link></p>}
        {mode === 'register' && <p className="login-note">{t('l.have_account')} <Link to="/login">{t('l.sign_in')}</Link></p>}
      </div>
      {status?.version && <p className="login-version">残像 Zanzō {/^v?\d/.test(status.version) ? 'v' + status.version.replace(/^v/, '') : status.version}</p>}
    </div>
  )
}
