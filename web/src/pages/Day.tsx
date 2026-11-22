import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { addDays, fmtDate, todayISO, weekday } from '../lib/format'
import { useI18n } from '../lib/i18n'
import { MOODS, MOOD_ICON, tagList, type DayPage, type Entry } from '../lib/types'
import { Card, Confirm, Loading, Segmented, modalOpen, useAsync, useToast } from '../components/ui'
import { Icon, SourceIcon } from '../components/icons'
import { announceChange } from '../components/Layout'
import { Timeline } from '../components/Timeline'
import { DictateButton, useAI } from '../components/ai'


export default function Day() {
  const params = useParams()
  const navigate = useNavigate()
  const { user } = useApp()
  const { t } = useI18n()
  const { toast } = useToast()
  const today = todayISO()
  let date = params.date || today
  if (date === 'today') date = today
  if (date === 'yesterday') date = addDays(today, -1)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) date = today
  const { data, setData, error, loading, reload } = useAsync(() => api.get<DayPage>(`${V1}/days/${date}`), [date])
  const [confirmDel, setConfirmDel] = useState(false)
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return
      if (e.metaKey || e.ctrlKey || e.altKey || modalOpen()) return
      if (e.key === 'ArrowLeft') navigate(`/day/${addDays(date, -1)}`)
      if (e.key === 'ArrowRight' && date < today) navigate(`/day/${addDays(date, 1)}`)
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [date, navigate, today])

  // the shell's Write menu, n and m land here with the entry or the moment field to focus
  const loc = useLocation()
  const focus = (loc.state as { focus?: 'entry' | 'moment' } | null)?.focus
  useEffect(() => {
    if (!focus || !data || data.date !== date) return
    const el = document.querySelector<HTMLElement>(focus === 'moment' ? '.moment-add input[name="moment"]' : '.highlight-input')
    if (!el) return
    el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    el.focus({ preventScroll: true })
    navigate(loc.pathname + loc.search, { replace: true, state: null })
  }, [loc.key, focus, data, date]) // eslint-disable-line react-hooks/exhaustive-deps

  const onUnpin = async (id: number) => {
    try { await api.del(`${V1}/pins/${id}`); setData(d => d ? { ...d, pins: d.pins.filter(p => p.id !== id) } : d) } catch (e) { toast((e as Error).message, 'err') }
  }
  const deleteEntry = async () => {
    setConfirmDel(false)
    try { await api.del(`${V1}/days/${date}`); toast(t('day.remove')); reload() } catch (e) { toast((e as Error).message, 'err') }
  }

  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return null
  const isToday = data.today
  const first = (user?.name || '').split(' ')[0]
  const hour = new Date().getHours()
  const greet = hour < 12 ? t('day.good_morning') : hour < 18 ? t('day.good_afternoon') : t('day.good_evening')
  return (
    <>
      <div className="day-head">
        <div className="day-nav">
          <Link className="btn ghost icon-only" to={`/day/${addDays(date, -1)}`} title={t('day.previous')} aria-label={t('day.previous')}><Icon name="chevron-left" size={20} /></Link>
          <div className="day-title">
            <h1>{isToday ? `${greet}${first ? ', ' + first : ''}` : weekday(date, true)}</h1>
            <div className="sub row wrap" style={{ gap: 8 }}>
              <span>{isToday ? `${weekday(date, true)}, ${fmtDate(date)}` : fmtDate(date)}</span>
              {!isToday && <Link className="status-pill accent" to="/"><Icon name="today" size={13} />{t('day.today')}</Link>}
              {data.streak > 0 && isToday && <span className="status-pill" title={t('day.streak_title')}><Icon name="flame" size={13} />{t('day.streak', { n: data.streak })}</span>}
            </div>
          </div>
          {date < today ? <Link className="btn ghost icon-only" to={`/day/${addDays(date, 1)}`} title={t('day.next')} aria-label={t('day.next')}><Icon name="chevron-right" size={20} /></Link> : <span className="btn ghost icon-only" style={{ visibility: 'hidden' }} aria-hidden="true" />}
        </div>
        <div className="actions row">
          <input type="date" className="date-jump" value={date} max={today} onChange={e => e.target.value && navigate(`/day/${e.target.value}`)} aria-label={t('day.jump')} />
        </div>
      </div>

      {data.future ? <Card><p className="muted" style={{ margin: 0 }}>{t('day.future')}</p></Card> : (
        <div className="day-grid">
          <div className="day-main">
            <EntryEditor date={date} entry={data.entry} isToday={isToday} prompt={isToday ? data.prompt : t('day.default_prompt')} onSaved={e => setData(d => d ? { ...d, entry: e } : d)} onDelete={() => setConfirmDel(true)} />
            <Timeline date={date} moments={data.moments} isToday={isToday} onMoments={ms => setData(d => d ? { ...d, moments: ms } : d)} />
            {data.pins.length > 0 && <Card title={<><Icon name="star" size={16} />{t('day.kept_from')}</>} className="mb">
              <div className="pins">{data.pins.map(p => <div key={p.id} className="pin">{p.image ? <img src={p.image} alt="" /> : <span className="pin-ico"><SourceIcon size={34} /></span>}<div className="grow"><b>{p.title}</b>{p.subtitle && <div className="muted small">{p.subtitle}</div>}</div><button type="button" className="btn ghost icon-only sm" onClick={() => onUnpin(p.id)} title={t('day.unpin')} aria-label={t('day.unpin')}><Icon name="x" size={15} /></button></div>)}</div>
            </Card>}
            {data.memories.length > 0 && <Card title={<><Icon name="repeat" size={16} />{t('day.on_this_day')}</>} className="mb">
              <div className="inline-list">{data.memories.map(m => <Link key={m.id} className="li" to={`/day/${m.date}`} style={{ color: 'inherit' }}><span className="badge">{m.date.slice(0, 4)}</span><span className="grow"><b>{m.highlight || firstLine(m.text)}</b>{m.highlight && m.text && <div className="muted small clamp2">{m.text}</div>}</span>{m.mood > 0 && <span title={MOODS[m.mood]}>{MOOD_ICON[m.mood]}</span>}</Link>)}</div>
            </Card>}
            <div className="row muted small" style={{ justifyContent: 'space-between' }}>
              <span>{data.prev_entry ? <Link className="row" style={{ gap: 2 }} to={`/day/${data.prev_entry}`} title={t('day.prev_entry')}><Icon name="chevron-left" size={14} />{fmtDate(data.prev_entry)}</Link> : ''}</span>
              <span>{data.next_entry ? <Link className="row" style={{ gap: 2 }} to={`/day/${data.next_entry}`} title={t('day.next_entry')}>{fmtDate(data.next_entry)}<Icon name="chevron-right" size={14} /></Link> : ''}</span>
            </div>
          </div>
        </div>
      )}
      {confirmDel && <Confirm title={t('day.remove_title')} message={t('day.remove_msg')} onCancel={() => setConfirmDel(false)} onConfirm={deleteEntry} />}
    </>
  )
}

function firstLine(t: string) { return (t || '').split('\n')[0] || '' }

/** The user's words: highlight, text, mood, tags. Autosaves a second after typing stops. */
function EntryEditor({ date, entry, prompt, isToday, onSaved, onDelete }: { date: string; entry: Entry | null; prompt: string; isToday: boolean; onSaved: (e: Entry | null) => void; onDelete: () => void }) {
  const { toast } = useToast()
  const { settings } = useApp()
  const { t } = useI18n()
  const aiOn = useAI()
  const [f, setF] = useState({ highlight: entry?.highlight || '', text: entry?.text || '', mood: entry?.mood || 0, tags: entry?.tags || '', pinned: entry?.pinned || false })
  const [state, setState] = useState<'idle' | 'dirty' | 'saving' | 'saved'>('idle')
  const [question, setQuestion] = useState('')
  const [draft, setDraft] = useState<{ highlight: string; text: string } | null>(null)
  const [sugg, setSugg] = useState<{ highlight: string; tags: string[]; mood: number; why: string } | null>(null)
  const [busy, setBusy] = useState('')
  const timer = useRef<number | null>(null)
  const latest = useRef(f)
  latest.current = f
  useEffect(() => { setF({ highlight: entry?.highlight || '', text: entry?.text || '', mood: entry?.mood || 0, tags: entry?.tags || '', pinned: entry?.pinned || false }); setState('idle'); setDraft(null); setSugg(null); setQuestion('') }, [date]) // eslint-disable-line react-hooks/exhaustive-deps
  const loadQuestion = useCallback((refresh = false) => {
    if (!aiOn || !settings?.ai_question) return
    api.get<{ question: string }>(`${V1}/ai/days/${date}/question${refresh ? '?refresh=1' : ''}`).then(r => setQuestion(r.question)).catch(() => {})
  }, [aiOn, settings?.ai_question, date])
  useEffect(() => { if (isToday || !entry) loadQuestion() }, [date]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = useCallback(async () => {
    setState('saving')
    try {
      const r = await api.put<{ entry: Entry | null }>(`${V1}/days/${date}`, latest.current)
      onSaved(r.entry)
      setState('saved')
      announceChange()
    } catch (e) { toast((e as Error).message, 'err'); setState('dirty') }
  }, [date, onSaved, toast])
  const change = (patch: Partial<typeof f>) => {
    setF(x => ({ ...x, ...patch })); setState('dirty')
    if (timer.current) window.clearTimeout(timer.current)
    timer.current = window.setTimeout(save, 900)
  }
  useEffect(() => () => { if (timer.current) window.clearTimeout(timer.current) }, [])
  const run = async (what: 'draft' | 'suggest') => {
    setBusy(what)
    try {
      if (what === 'draft') { setSugg(null); setDraft(await api.post(`${V1}/ai/days/${date}/draft`)) }
      else { setDraft(null); setSugg(await api.post(`${V1}/ai/days/${date}/suggest`, { highlight: f.highlight, text: f.text })) }
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  const append = (t: string) => change({ text: f.text.trim() ? f.text.replace(/\s+$/, '') + '\n\n' + t : t })
  const words = f.text.trim() ? f.text.trim().split(/\s+/).length : 0
  const title = question
    ? <span className="ai-q" title={t('day.tonight_question')}><Icon name="sparkles" size={15} />{question}<button type="button" className="btn ghost icon-only sm" onClick={() => loadQuestion(true)} title={t('day.another_question')} aria-label={t('day.another_question')}><Icon name="refresh" size={14} /></button></span>
    : <span className="muted" style={{ fontWeight: 500 }}>{prompt}</span>
  return (
    <Card className="mb entry-card" title={title} actions={<span className={`save-state ${state}`}>{state === 'saving' ? <><span className="spinner" style={{ width: 11, height: 11 }} />{t('day.saving')}</> : state === 'saved' ? <><Icon name="check" size={13} />{t('day.saved')}</> : state === 'dirty' ? t('day.unsaved') : ''}</span>}>
      <input className="highlight-input" value={f.highlight} onChange={e => change({ highlight: e.target.value })} placeholder={t('day.highlight_ph')} maxLength={200} />
      <textarea className="entry-text" value={f.text} onChange={e => change({ text: e.target.value })} placeholder={t('day.text_ph')} rows={Math.max(4, Math.min(24, f.text.split('\n').length + 1))} />
      {aiOn && <div className="ai-bar">
        <button type="button" className="btn ghost sm" onClick={() => run('draft')} disabled={!!busy} title={t('ai.what_draft')}><Icon name="sparkles" size={14} />{busy === 'draft' ? t('day.drafting') : f.text.trim() ? t('day.fill_rest') : t('day.draft')}</button>
        <button type="button" className="btn ghost sm" onClick={() => run('suggest')} disabled={!!busy || !(f.text.trim() || f.highlight.trim())} title={t('ai.what_suggest')}><Icon name="tag" size={14} />{busy === 'suggest' ? t('day.thinking') : t('day.suggest')}</button>
      </div>}
      <DictateButton onText={append} />
      {draft && <div className="ai-panel">
        {draft.highlight && <div className="row mb-sm"><b className="grow">“{draft.highlight}”</b><button type="button" className="btn sm" onClick={() => change({ highlight: draft.highlight })}>{t('day.use_as_highlight')}</button></div>}
        {draft.text ? <div className="t">{draft.text}</div> : <p className="muted small">{t('day.draft_covers')}</p>}
        <div className="row mt-sm">{draft.text && <button type="button" className="btn primary sm" onClick={() => { change({ text: f.text.trim() ? f.text.replace(/\s+$/, '') + '\n\n' + draft.text : draft.text, ...(!f.highlight.trim() && draft.highlight ? { highlight: draft.highlight } : {}) }); setDraft(null) }}>{f.text.trim() ? t('day.add_below') : t('day.use_draft')}</button>}<button type="button" className="btn ghost sm" onClick={() => run('draft')}>{t('day.another')}</button><button type="button" className="btn ghost sm" onClick={() => setDraft(null)}>{t('day.dismiss')}</button><span className="spacer" /><span className="muted small">{t('day.draft_note')}</span></div>
      </div>}
      {sugg && <div className="ai-panel">
        {sugg.highlight && <div className="row mb-sm"><span className="grow">“{sugg.highlight}”</span><button type="button" className="btn sm" onClick={() => change({ highlight: sugg.highlight })}>{t('day.use')}</button></div>}
        {sugg.tags.length > 0 && <div className="row mb-sm wrap"><span className="chips grow">{sugg.tags.map(tag => <span key={tag} className="chip sm">#{tag}</span>)}</span><button type="button" className="btn sm" onClick={() => change({ tags: [f.tags, ...sugg.tags].filter(Boolean).join(',') })}>{t('day.add_tags')}</button></div>}
        {sugg.mood > 0 && <div className="row"><span className="grow">{MOOD_ICON[sugg.mood]} {MOODS[sugg.mood]} <span className="muted small">· {sugg.why}</span></span><button type="button" className="btn sm" onClick={() => change({ mood: sugg.mood })}>{t('day.set_mood')}</button></div>}
        <div className="row mt-sm"><button type="button" className="btn ghost sm" onClick={() => { change({ highlight: sugg.highlight || f.highlight, tags: [f.tags, ...sugg.tags].filter(Boolean).join(','), mood: sugg.mood || f.mood }); setSugg(null) }}>{t('day.apply_all')}</button><button type="button" className="btn ghost sm" onClick={() => setSugg(null)}>{t('day.dismiss')}</button></div>
      </div>}
      <div className="entry-foot">
        <Segmented size="sm" value={f.mood} onChange={v => change({ mood: v === f.mood ? 0 : v })} options={[1, 2, 3, 4, 5].map(m => ({ value: m, label: MOOD_ICON[m], title: MOODS[m] }))} />
        <input className="tags-input" value={f.tags} onChange={e => change({ tags: e.target.value })} placeholder={t('day.tags_ph')} />
        <button type="button" className={`btn ghost sm ${f.pinned ? 'on' : ''}`} onClick={() => change({ pinned: !f.pinned })} title={t('day.keep_title')} aria-pressed={f.pinned}><Icon name="star" size={14} />{f.pinned ? t('day.kept') : t('day.keep')}</button>
        <span className="spacer" />
        {words > 0 && <span className="muted small">{t('day.words', { n: words })}</span>}
        {entry && <button type="button" className="btn ghost sm danger-text" onClick={onDelete}>{t('day.remove')}</button>}
      </div>
      {f.tags && <div className="chips mt-sm">{tagList(f.tags).map(t => <Link key={t} className="chip" to={`/journal?tag=${encodeURIComponent(t)}`}>#{t}</Link>)}</div>}
    </Card>
  )
}

