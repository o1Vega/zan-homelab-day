import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1, upload } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, Settings as SettingsT, UsageRow, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { LAST_PAGE_KEY } from '../components/Layout'
import { useI18n } from '../lib/i18n'

type TabId = 'general' | 'connect' | 'notifications' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TAB_KEY: Record<TabId, [string, string]> = {
  general: ['st.general', 'st.general_sub'],
  connect: ['st.connect', 'st.connect_sub'],
  notifications: ['st.notifications', 'st.notifications_sub'],
  ai: ['st.ai', 'st.ai_sub'],
  profile: ['st.profile', 'st.profile_sub'],
  people: ['st.people', 'st.people_sub'],
  tokens: ['st.tokens', 'st.tokens_sub'],
  data: ['st.data', 'st.data_sub'],
  about: ['st.about', 'st.about_sub'],
}
const TABS: { id: TabId; icon: string }[] = [
  { id: 'general', icon: 'settings' },
  { id: 'connect', icon: 'layers' },
  { id: 'notifications', icon: 'bell' },
  { id: 'ai', icon: 'sparkles' },
  { id: 'profile', icon: 'user' },
  { id: 'people', icon: 'users' },
  { id: 'tokens', icon: 'key' },
  { id: 'data', icon: 'database' },
  { id: 'about', icon: 'info' },
]
// the old ?tab= names
const ALIAS: Record<string, TabId> = { notify: 'notifications', users: 'people', api: 'tokens', sources: 'connect' }

export default function Settings() {
  const { t } = useI18n()
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const legacy = new URLSearchParams(window.location.search).get('tab') || ''
  const want = raw || legacy
  const tab: TabId = (TABS.find(x => x.id === (ALIAS[want] || want))?.id) || 'general'
  const [tabLabel, tabSub] = TAB_KEY[tab]
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  const back = () => { const last = sessionStorage.getItem(LAST_PAGE_KEY); navigate(last && !last.startsWith('/settings') ? last : '/') }

  // old links (?tab=notify …) land on /settings/<tab>
  useEffect(() => { if (!raw && legacy) navigate(`/settings/${tab}`, { replace: true }) }, [raw, legacy, tab, navigate])
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop, .palette-backdrop')) back() }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label={t('st.back')} title={t('st.back')} onClick={back}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>{t('st.settings')}</h1>
        <span style={{ flex: 1 }} />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(x => ({ id: x.id, label: t(TAB_KEY[x.id][0]), icon: x.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label={t('st.settings')}>
            {TABS.map(x => (
              <button key={x.id} type="button" className={`set-nav-item ${x.id === tab ? 'active' : ''}`} aria-current={x.id === tab ? 'page' : undefined} onClick={() => go(x.id)}>
                <Icon name={x.icon} size={16} />{t(TAB_KEY[x.id][0])}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title">
              <h2>{t(tabLabel)}</h2>
              <p>{t(tabSub)}</p>
            </div>
            {tab === 'general' && <General />}
            {tab === 'connect' && <Connect />}
            {tab === 'notifications' && <Notify />}
            {tab === 'ai' && <AISettings />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

/** A settings card: title, explanation, actions on the right. */
function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

/** "✓ ok" / "✕ failed" test results. */
const TestResult = ({ text }: { text?: string }) => text ? <span className={`small ${text.startsWith('✓') ? 'ok' : text.startsWith('✕') ? 'bad' : 'muted'}`}>{text}</span> : null

function useSettingsForm() {
  const { t } = useI18n()
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast(t('st.saved')) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function General() {
  const { t } = useI18n()
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  const [q, setQ] = useState('')
  const [hits, setHits] = useState<{ name: string; region: string; lat: number; lon: number }[]>([])
  useEffect(() => {
    if (q.trim().length < 2) { setHits([]); return }
    const h = setTimeout(() => api.get<typeof hits>(`${V1}/geo/search?q=${encodeURIComponent(q)}`).then(setHits).catch(() => {}), 300)
    return () => clearTimeout(h)
  }, [q])
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title={t('st.writing')} sub={t('st.writing_sub')}>
        <SettingRow title={t('st.prompt')} help={t('st.prompt_help')} stack><input value={f.prompt} onChange={e => setF({ ...f, prompt: e.target.value })} maxLength={120} /></SettingRow>
      </Section>
      <Section title={t('st.datetime')}>
        <SettingRow title={t('st.date_format')}><Select value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title={t('st.time_format')}><Select value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title={t('st.week_starts')}><Segmented value={f.week_start} onChange={v => setF({ ...f, week_start: v as 'mon' | 'sun' })} options={[{ value: 'mon', label: t('st.monday') }, { value: 'sun', label: t('st.sunday') }]} size="sm" /></SettingRow>
        <SettingRow title={t('st.distances')}><Segmented value={f.distance_unit} onChange={v => setF({ ...f, distance_unit: v as 'km' | 'mi' })} options={[{ value: 'km', label: 'km' }, { value: 'mi', label: t('st.miles') }]} size="sm" /></SettingRow>
      </Section>
      <Section title={t('st.home')}>
        <SettingRow title={t('st.home_location')} help={t('st.home_help')} stack>
          <div>
            <input value={q} onChange={e => setQ(e.target.value)} placeholder={f.home_place ? t('st.type_change', { p: f.home_place, lat: f.home_lat.toFixed(3), lon: f.home_lon.toFixed(3) }) : t('st.search_town')} aria-label={t('st.search_town')} />
            {hits.length > 0 && <div className="geo-hits">{hits.map((h, i) => <button type="button" key={i} className="geo-hit" onClick={() => { setF({ ...f, home_place: h.name, home_lat: h.lat, home_lon: h.lon }); setQ(''); setHits([]) }}><Icon name="pin" size={15} className="muted" /><span className="grow">{h.name}</span><span className="muted small">{h.region}</span></button>)}</div>}
          </div>
        </SettingRow>
      </Section>
      <Section title={t('st.box')} sub={isAdmin ? t('st.box_admin') : t('st.box_member')}>
        <SettingRow title={t('st.app_title')} help={t('st.app_title_help')}><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} disabled={!isAdmin} title={isAdmin ? undefined : t('st.only_admin')} style={{ width: 220 }} /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>{t('st.save')}</button></div>
      <Section title={t('st.fits')}>
        <dl className="set-dl">
          <div><dt>{t('st.fits_words_t')}</dt><dd className="muted">{t('st.fits_words')}</dd></div>
          <div><dt>{t('st.fits_pins_t')}</dt><dd className="muted">{t('st.fits_pins')}</dd></div>
          <div><dt>{t('st.fits_mem_t')}</dt><dd className="muted">{t('st.fits_mem')}</dd></div>
          <div><dt>{t('st.fits_year_t')}</dt><dd className="muted">{t('st.fits_year')}</dd></div>
          <div><dt>{t('st.fits_keys_t')}</dt><dd className="muted">{t('st.fits_keys')}</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Notify() {
  const { t } = useI18n()
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const [test, setTest] = useState('')
  if (!f) return <Loading />
  const run = async () => {
    setTest(t('n.testing'))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which: 'notify', url: f.notify_url, format: f.notify_format }); setTest(r.ok ? `✓ ${r.message}` : `✕ ${r.error}`) } catch (e) { setTest(`✕ ${(e as Error).message}`) }
  }
  const send = async (which: 'evening' | 'morning' | 'weekly') => { try { const r = await api.post<{ ok: boolean; sent: number; error?: string }>(`${V1}/notify/run`, { which }); toast(r.ok ? (r.sent ? t('n.sent') : t('n.nothing_send')) : r.error || t('toast.failed'), r.ok ? 'ok' : 'err') } catch (e) { toast((e as Error).message, 'err') } }
  const hours = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))
  return (
    <div className="stack loose">
      <Section title={t('n.where')} icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? t('n.connected') : t('n.not_set')}</span>}
        sub={t('n.where_sub')}>
        <SettingRow title={t('n.url')} stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/day" /></SettingRow>
        <SettingRow title={t('n.format')}><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
        <SettingRow title={t('n.check')} help={t('n.check_help')}><button type="button" className="btn sm" onClick={run} disabled={!f.notify_url}><Icon name="mail" size={14} />{t('n.send_test')}</button><TestResult text={test} /></SettingRow>
      </Section>
      <Section title={t('n.when')} icon="clock">
        <SettingRow title={t('n.evening')} help={t('n.evening_help', { q: f.prompt })}><Switch checked={f.notify_evening} onChange={v => setF({ ...f, notify_evening: v })} label={t('n.evening_push')} /></SettingRow>
        <SettingRow title={t('n.evening_hour')}><Select value={String(f.evening_hour)} onChange={v => setF({ ...f, evening_hour: Number(v) })} options={hours} /></SettingRow>
        <SettingRow title={t('n.morning')} help={t('n.morning_help')}><Switch checked={f.notify_morning} onChange={v => setF({ ...f, notify_morning: v })} label={t('n.morning_push')} /></SettingRow>
        <SettingRow title={t('n.morning_hour')}><Select value={String(f.morning_hour)} onChange={v => setF({ ...f, morning_hour: Number(v) })} options={hours} /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>{t('st.save')}</button>
        <button type="button" className="btn" onClick={() => send('evening')} disabled={!f.notify_configured}><Icon name="moon" size={15} />{t('n.send_evening')}</button>
        <button type="button" className="btn" onClick={() => send('morning')} disabled={!f.notify_configured}><Icon name="sun" size={15} />{t('n.send_morning')}</button>
        {f.ai_configured && <button type="button" className="btn" onClick={() => send('weekly')} disabled={!f.notify_configured}><Icon name="book" size={15} />{t('n.send_week')}</button>}
      </div>
    </div>
  )
}

const AI_MODELS = ['gpt-5.4-mini', 'gpt-5.4', 'gpt-5.4-nano', 'gpt-5.5', 'gpt-4.1-mini', 'gpt-4o-mini']
const FEATURE_KEY: Record<string, string> = { draft: 'feat.draft', question: 'feat.question', suggest: 'feat.suggest', dictation: 'feat.dictation', 'story-week': 'feat.story-week', 'story-month': 'feat.story-month', 'story-year': 'feat.story-year', ask: 'feat.ask', 'ask-plan': 'feat.ask-plan' }

function AISettings() {
  const { t } = useI18n()
  const [showDictation, setShowDictation] = useState(false)
  const { f, setF, save, busy } = useSettingsForm()
  const [test, setTest] = useState('')
  const { data: usage } = useAsync(() => api.get<{ last_30_days: UsageRow[]; all_time: UsageRow[] }>(`${V1}/ai/usage`), [])
  if (!f) return <Loading />
  const run = async () => {
    setTest(t('n.testing'))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which: 'ai', url: f.ai_base_url, key: f.ai_key, extra: f.ai_model }); setTest(r.ok ? `✓ ${r.message}` : `✕ ${r.error}`) } catch (e) { setTest(`✕ ${(e as Error).message}`) }
  }
  const tot = (rows: UsageRow[] = []) => rows.reduce((a, r) => ({ calls: a.calls + r.calls, p: a.p + r.prompt_tokens, c: a.c + r.completion_tokens }), { calls: 0, p: 0, c: 0 })
  const m30 = tot(usage?.last_30_days), all = tot(usage?.all_time)
  return (
    <div className="stack loose" style={{ gridTemplateColumns: 'minmax(0, 1fr)' }}>
            <Section title={t('ai.provider')} icon="sparkles" actions={<span className={`badge ${f.ai_configured ? 'ok' : ''}`}>{f.ai_configured ? t('n.connected') : t('n.not_set')}</span>}
                sub={t('ai.provider_sub')}>
        <div className="stack">
                    <Field group label={t('ai.text_gen')} help={t('ai.text_gen_help')}><Select ariaLabel={t('ai.text_gen')} value={f.ai_provider === 'openai' ? 'openai' : ''} onChange={v => { setF({ ...f, ai_provider: v }); setShowDictation(false) }} options={[{ value: '', label: t('ai.auto') }, { value: 'openai', label: t('ai.openai') }]} /></Field>
          <div className="stack">
                        <Field label={t('ai.key')} help={t('ai.key_help')}><input value={f.ai_key} onChange={e => setF({ ...f, ai_key: e.target.value })} placeholder="sk-…" autoComplete="off" /></Field>
                        <Field label={t('ai.model')} help={t('ai.model_help', { m: f.ai_default_model })}><input list="ai-models" value={f.ai_model} onChange={e => setF({ ...f, ai_model: e.target.value })} placeholder={f.ai_default_model} /><datalist id="ai-models">{AI_MODELS.map(m => <option key={m} value={m} />)}</datalist></Field>
                        <Field label={t('ai.base')}><input value={f.ai_base_url} onChange={e => setF({ ...f, ai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
                        <Field label={t('ai.image_key')} help={t('ai.image_key_help')}><input value={f.ai_image_key} onChange={e => setF({ ...f, ai_image_key: e.target.value })} placeholder="sk-…" autoComplete="off" /></Field>
                        <Field label={t('ai.image_model')}><input value={f.ai_image_model} onChange={e => setF({ ...f, ai_image_model: e.target.value })} placeholder="text-to-image" /></Field>
                        <Field label={t('ai.tts_model')} help={t('ai.tts_model_help', { m: f.ai_default_tts })}><input value={f.ai_tts_model} onChange={e => setF({ ...f, ai_tts_model: e.target.value })} placeholder={f.ai_default_tts} /></Field>
                        <Field label={t('ai.tts_key')} help={t('ai.tts_key_help')}><input value={f.ai_tts_key} onChange={e => setF({ ...f, ai_tts_key: e.target.value })} placeholder="sk-…" autoComplete="off" /></Field>
                        <Field label={t('ai.tts_voice')} help={t('ai.tts_voice_help')}><input value={f.ai_tts_voice} onChange={e => setF({ ...f, ai_tts_voice: e.target.value })} placeholder="alloy" /></Field>
                        <div className="row"><button type="button" className="btn sm" onClick={run} disabled={!f.ai_key}>{t('ai.test')}</button><TestResult text={test} /></div>
          </div>
                    <Field label={t('ai.write_in')} help={t('ai.write_in_help')}><input value={f.ai_language} onChange={e => setF({ ...f, ai_language: e.target.value })} placeholder="e.g. Serbian (Latin script)" /></Field>
        </div>
      </Section>
            <Section title={t('ai.dictation')} icon="mic" actions={<span className={`badge ${f.ai_transcribe_key ? 'ok' : ''}`}>{f.ai_transcribe_key ? 'OpenAI' : t('n.not_set')}</span>}
                sub={t('ai.dictation_sub')}>
        <div className="stack">
                    <div><button type="button" className="btn sm" aria-expanded={showDictation} aria-controls="dictation-settings" onClick={() => setShowDictation(v => !v)}>{showDictation ? t('ai.dict_hide') : t('ai.dict_configure')}</button></div>
          {showDictation && <div id="dictation-settings" className="stack">
            {f.ai_provider !== 'openai' && <>
                            <Field label={t('ai.key')} help={t('ai.key_help')}><input value={f.ai_key} onChange={e => setF({ ...f, ai_key: e.target.value })} placeholder="sk-…" autoComplete="off" /></Field>
                            <Field label={t('ai.base')}><input value={f.ai_base_url} onChange={e => setF({ ...f, ai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </>}
                        <Field label={t('ai.dict_model')}><input value={f.ai_transcribe_model} onChange={e => setF({ ...f, ai_transcribe_model: e.target.value })} placeholder={f.ai_default_transcribe} /></Field>
                        <Field label={t('ai.transcribe_key')} help={t('ai.transcribe_key_help')}><input value={f.ai_transcribe_key} onChange={e => setF({ ...f, ai_transcribe_key: e.target.value })} placeholder="sk-…" autoComplete="off" /></Field>
          </div>}
        </div>
      </Section>
            <Section title={t('ai.options')} sub={t('ai.options_sub')}>
                <SettingRow title={t('ai.q')} help={t('ai.q_help')}><Switch checked={f.ai_question} onChange={v => setF({ ...f, ai_question: v })} label={t('ai.q')} /></SettingRow>
                <SettingRow title={t('ai.weekly')} help={t('ai.weekly_help')}><Switch checked={f.ai_weekly_story} onChange={v => setF({ ...f, ai_weekly_story: v })} label={t('ai.weekly')} /></SettingRow>
                <SettingRow title={t('ai.send_text')} help={t('ai.send_text_help')}><Switch checked={f.ai_send_text} onChange={v => setF({ ...f, ai_send_text: v })} label={t('ai.send_text')} /></SettingRow>
      </Section>
            <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>{t('st.save')}</button></div>
            <Section title={t('ai.what')}>
        <dl className="set-dl">
                    <div><dt>{t('feat.draft')}</dt><dd className="muted">{t('ai.what_draft')}</dd></div>
                    <div><dt>{t('feat.question')}</dt><dd className="muted">{t('ai.what_q')}</dd></div>
                    <div><dt>{t('ai.what_suggest_t')}</dt><dd className="muted">{t('ai.what_suggest')}</dd></div>
                    <div><dt>{t('feat.dictation')}</dt><dd className="muted">{t('ai.what_dict')}</dd></div>
                    <div><dt>{t('ai.what_stories_t')}</dt><dd className="muted">{t('ai.what_stories')}</dd></div>
                    <div><dt>{t('ai.what_ask_t')}</dt><dd className="muted">{t('ai.what_ask')}</dd></div>
        </dl>
      </Section>
            <Section title={t('ai.usage')} icon="activity">
        {usage && (usage.all_time.length > 0 ? <>
          <div style={{ overflowX: 'auto' }}><table className="table compact">
                        <thead><tr><th>{t('ai.feature')}</th><th className="right">{t('ai.calls')}</th><th className="right">{t('ai.tokens_in')}</th><th className="right">{t('ai.tokens_out')}</th></tr></thead>
                        <tbody>{usage.last_30_days.map(r => <tr key={r.feature + r.model}><td>{FEATURE_KEY[r.feature] ? t(FEATURE_KEY[r.feature]) : r.feature} <span className="muted small">{r.model}</span></td><td className="right">{r.calls}</td><td className="right">{r.prompt_tokens.toLocaleString()}</td><td className="right">{r.completion_tokens.toLocaleString()}</td></tr>)}</tbody>
          </table></div>
                    <p className="muted small mt-sm" style={{ margin: '8px 0 0' }}>{t('ai.usage_line', { c: m30.calls, t: (m30.p + m30.c).toLocaleString(), c2: all.calls, t2: (all.p + all.c).toLocaleString() })}</p>
                </> : <p className="muted" style={{ margin: 0 }}>{t('ai.nothing_used')}</p>)}
      </Section>
    </div>
  )
}

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { t } = useI18n()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title={t('u.title')} actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />{t('u.add')}</button>}
      sub={t('u.sub')}>
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">{t('u.you')}</span>}{u.role === 'admin' && <span className="badge accent">{t('u.admin')}</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>{t('u.edit')}</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>{t('u.remove')}</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">{t('u.auth_note', { m: String(status?.auth_mode) })}</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel={t('u.remove')} title={t('u.remove_title', { n: del.name || del.email })} message={t('u.remove_msg')} onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), t('u.removed')) }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const { t } = useI18n()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? t('u.saved') : t('u.added')); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? t('u.edit_person') : t('u.add_person')} onClose={onClose} size="sm">
      <form onSubmit={save} className="stack">
        <Field label={t('u.name')}><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label={t('u.email')} help={t('u.email_help')}><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? t('u.pass_new') : t('u.pass')} help={t('u.pass_help')}><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label={t('u.role')}><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: t('u.member') }, { value: 'admin', label: t('u.admin') }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>{t('u.cancel')}</button><button type="submit" className="btn primary" disabled={busy}>{user.id ? t('u.save') : t('u.add_person')}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { t } = useI18n()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast(t('p.saved')) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role === 'admin' ? t('u.admin') : t('u.member')}</p></div>
      </div>
      <Section title={t('p.you')}>
        <div className="stack">
          <Field label={t('u.name')}><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label={t('u.email')}><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title={t('p.password')} sub={t('p.keep_pass')}><div className="stack">
        <Field label={t('p.current')}><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label={t('p.new')} help={t('u.pass_help')}><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button type="submit" className="btn primary" disabled={busy}>{t('st.save')}</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { t } = useI18n()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title={t('t.title')} icon="key" sub={t('t.sub')}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder={t('t.name_ph')} /><button type="submit" className="btn primary"><Icon name="plus" size={15} />{t('t.create')}</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>{t('t.copy_now')}</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>{t('t.name')}</th><th>{t('t.prefix')}</th><th>{t('t.last_used')}</th><th>{t('t.created')}</th><th /></tr></thead>
            <tbody>{data.map(tok => <tr key={tok.id}><td>{tok.name}</td><td className="mono">{tok.prefix}…</td><td className="muted">{tok.last_used_at ? fmtDateTime(tok.last_used_at) : t('t.never')}</td><td className="muted">{fmtDateTime(tok.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(tok)}>{t('t.revoke')}</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted" style={{ margin: 0 }}>{t('t.none')}</p>}
        {del && <Confirm confirmLabel={t('t.revoke')} title={t('t.revoke_title', { n: del.name })} message={t('t.revoke_msg')} onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title={t('t.shortcuts')} icon="smartphone" sub={t('t.shortcuts_sub', { base: `${base}/api/v1` })}>
        <dl className="set-dl">
          <div><dt><code>POST /moments</code></dt><dd className="muted"><code>{'{"text":"coffee with A.","kind":"note"}'}</code> — {t('t.s1')}</dd></div>
          <div><dt><code>POST /entries</code></dt><dd className="muted"><code>{'{"highlight":"…","text":"…","mood":4,"tags":"walk"}'}</code> — {t('t.s2')}</dd></div>
          <div><dt><code>GET /today</code></dt><dd className="muted">{t('t.s3')}</dd></div>
          <div><dt><code>GET /dashboard</code></dt><dd className="muted">{t('t.s4')}</dd></div>
          <div><dt><code>GET /memories</code></dt><dd className="muted">{t('t.s5')}</dd></div>
          <div><dt><code>GET /export.md</code></dt><dd className="muted">{t('t.s6')}</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  const { t } = useI18n()
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const importJSON = async (file: File) => {
    setBusy(true)
    try { const r = await api.post<Record<string, number>>(`${V1}/import`, JSON.parse(await file.text())); toast(t('d.imported', { what: Object.entries(r).map(([k, v]) => `${v} ${k}`).join(', ') || t('d.nothing_new') })) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack loose">
      {user?.role === 'admin' && <Section title={t('d.backup')} icon="database" sub={t('d.backup_sub')}>
        <p className="muted">{t('d.backup_body')}</p>
        <a className="btn primary" href={`${V1}/backup`}><Icon name="download" size={15} />{t('d.download')}</a>
      </Section>}
      <Section title={t('d.export')} icon="archive" sub={t('d.export_sub')}>
        <div className="row wrap">
          <a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />{t('d.json')}</a>
          <a className="btn" href={`${V1}/export.md`}><Icon name="text" size={15} />{t('d.md')}</a>
          <label className="btn"><Icon name="upload" size={15} />{busy ? t('d.importing') : t('d.import')}<input type="file" accept="application/json,.json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
        </div>
      </Section>
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const { t } = useI18n()
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const tiles: [string, number | undefined, string][] = [[t('ab.entries'), counts.entries, 'book-open'], [t('ab.moments'), counts.moments, 'clock'], [t('ab.pins'), counts.pins, 'star']]
  const rows: [string, React.ReactNode][] = [
    [t('ab.version'), <code key="v">{String(data.version)}</code>],
    [t('ab.uptime'), String(data.uptime)],
    [t('ab.runtime'), `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    [t('ab.tz'), String(data.timezone)],
    [t('ab.signin'), String(data.auth_mode)],
    [t('ab.database'), <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    [t('ab.rows'), t('ab.rows_fmt', { u: counts.users ?? 0 })],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>残像 Zanzō</h3>
          <p className="muted small">{t('ab.desc')}</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{n ?? 0}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title={t('ab.server')}>
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <p className="muted small" style={{ margin: 0 }}>{t('ab.open_source')}</p>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => window.dispatchEvent(new CustomEvent('zan:shortcuts'))}><Icon name="keyboard" size={15} />{t('ab.shortcuts')}</button>
      </div>
    </div>
  )
}

function Connect() {
  const { t } = useI18n()
  const { settings, setSettings } = useApp()
  const { toast } = useToast()
  const f = settings
  const [clientId, setClientId] = useState('')
  const [clientSecret, setClientSecret] = useState('')
  const [busy, setBusy] = useState(false)
  const [zipBusy, setZipBusy] = useState(false)

  // once: read the ?google_ok / ?google_error flags left by the OAuth redirect
  useEffect(() => {
    const q = new URLSearchParams(window.location.search)
    const ok = q.get('google_ok')
    const err = q.get('google_error')
    if (!ok && !err) return
    q.delete('google_ok'); q.delete('google_error')
    const qs = q.toString()
    window.history.replaceState(null, '', qs ? `?${qs}` : window.location.pathname)
    if (err) toast(t('cn.google_err', { e: err }), 'err')
    else toast(t('cn.connected_back'), 'ok')
  }, [toast, t])

  if (!f) return <Loading />
  const sync = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ ok: boolean; fetched: number; new: number }>(`${V1}/integrations/google/import`)
      const s = await api.get<SettingsT>(`${V1}/settings`)
      setSettings(s)
      toast(t('cn.synced', { f: r.fetched, n: r.new }), 'ok')
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const disconnect = async () => {
    setBusy(true)
    try {
      await api.del(`${V1}/integrations/google`)
      const s = await api.get<SettingsT>(`${V1}/settings`)
      setSettings(s)
      toast(t('cn.disconnected'), 'ok')
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const connect = async () => {
    if (!clientId.trim() || !clientSecret.trim()) { toast(t('cn.need_creds'), 'err'); return }
    setBusy(true)
    try {
      const s = await api.put<SettingsT>(`${V1}/settings`, { google_client_id: clientId.trim(), google_client_secret: clientSecret.trim() })
      setSettings(s)
      window.location.href = `${V1}/integrations/google/start`
    } catch (e) { toast((e as Error).message, 'err'); setBusy(false) }
  }
  const onZip = async (file: File) => {
    setZipBusy(true)
    try {
      const r = await upload<{ ok: boolean; imported: number; skipped: number }>(`${V1}/integrations/import/zip`, 'file', file)
      toast(t('cn.zip_done', { n: r.imported, m: r.skipped }), 'ok')
    } catch (e) { toast((e as Error).message, 'err') } finally { setZipBusy(false) }
  }

  return (
    <div className="stack loose">
      <Section title={t('cn.google')} icon="camera" actions={<span className={`badge ${f.google_connected ? 'ok' : ''}`}>{f.google_connected ? t('n.connected') : t('n.not_set')}</span>} sub={t('cn.google_sub')}>
        {f.google_connected ? (
          <div className="stack">
            <SettingRow title={t('cn.account')}><code>{f.google_account || '—'}</code></SettingRow>
            <SettingRow title={t('cn.last_sync')}><span className="muted">{f.google_last_sync ? fmtDateTime(f.google_last_sync) : t('t.never')}</span></SettingRow>
            <div className="row wrap">
              <button type="button" className="btn primary" onClick={sync} disabled={busy}><Icon name="refresh" size={15} />{busy ? t('cn.syncing') : t('cn.sync')}</button>
              <button type="button" className="btn ghost" onClick={disconnect} disabled={busy}>{t('cn.disconnect')}</button>
            </div>
          </div>
        ) : (
          <div className="stack">
            <p className="muted small" style={{ margin: 0 }}>{t('cn.google_help', { base: f.base_url })}</p>
            <Field label={t('cn.client_id')}><input value={clientId} onChange={e => setClientId(e.target.value)} placeholder="1234…-.apps.googleusercontent.com" autoComplete="off" /></Field>
            <Field label={t('cn.client_secret')}><input type="password" value={clientSecret} onChange={e => setClientSecret(e.target.value)} placeholder="GOCSPX-…" autoComplete="off" /></Field>
            <div className="row"><button type="button" className="btn primary" onClick={connect} disabled={busy}>{t('cn.connect_btn')}</button></div>
          </div>
        )}
      </Section>
      <Section title={t('cn.zip')} icon="upload" sub={t('cn.zip_sub')}>
        <div className="stack">
          <p className="muted small" style={{ margin: 0 }}>{t('cn.zip_help')}</p>
          <label className="btn">
            <Icon name="upload" size={15} />{zipBusy ? t('cn.zip_importing') : t('cn.zip_btn')}
            <input type="file" accept=".zip,application/zip" hidden disabled={zipBusy} onChange={e => { const file = e.target.files?.[0]; e.target.value = ''; if (file) onZip(file) }} />
          </label>
        </div>
      </Section>
    </div>
  )
}
