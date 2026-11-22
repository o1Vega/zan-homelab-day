import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { fmtDate, monthName, todayISO, weekdayLetters, weekday } from '../lib/format'
import { useI18n } from '../lib/i18n'
import { MOODS, MOOD_COLOR, MOOD_ICON, type CalendarDay, type DayPage, type YearRecap } from '../lib/types'
import { Loading, useAsync } from '../components/ui'
import { Icon } from '../components/icons'
import { StoryCard, useAI } from '../components/ai'

type Layer = 'written' | 'mood'
const LAYER_KEY: Record<Layer, string> = { written: 'cal.written', mood: 'layer.mood' }
const LAYER_ICON: Record<Layer, string> = { written: 'pen', mood: 'smile' }
const LAYER_IDS: Layer[] = ['written', 'mood']
const ym = (y: number, m: number) => `${y}-${String(m).padStart(2, '0')}`
const daysIn = (y: number, m: number) => new Date(y, m, 0).getDate()

export default function Calendar() {
  const { month: mp } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const { t } = useI18n()
  const aiOn = useAI()
  const today = todayISO()
  const month = mp && /^\d{4}-\d{2}$/.test(mp) ? mp : today.slice(0, 7)
  const [y, m] = month.split('-').map(Number)
  const { data, loading, error } = useAsync(() => api.get<{ days: CalendarDay[] }>(`${V1}/calendar?month=${month}`), [month])
  const { data: yearData } = useAsync(() => api.get<YearRecap>(`${V1}/years/${y}`), [y])
  const [layer, setLayerRaw] = useState<Layer>(() => { const v = new URLSearchParams(window.location.search).get('layer') || localStorage.getItem('zan-cal-layer'); return v === 'mood' ? 'mood' : 'written' })
  const setLayer = (l: Layer) => { setLayerRaw(l); localStorage.setItem('zan-cal-layer', l) }
  const [sel, setSel] = useState<string>('')
  const wide = useWide()
  useEffect(() => { setSel(month === today.slice(0, 7) ? today : '') }, [month, today])
  const go = (n: number) => { const d = new Date(y, m - 1 + n, 1); navigate(`/calendar/${ym(d.getFullYear(), d.getMonth() + 1)}`) }
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA')) return
      if (e.key === 'ArrowLeft' && e.shiftKey) go(-1)
      else if (e.key === 'ArrowRight' && e.shiftKey && month < today.slice(0, 7)) go(1)
      else if ((e.key === 'ArrowLeft' || e.key === 'ArrowRight') && sel) {
        const d = new Date(sel); d.setDate(d.getDate() + (e.key === 'ArrowLeft' ? -1 : 1))
        const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
        if (iso.slice(0, 7) === month && iso <= today) setSel(iso)
      } else if (e.key === 'Enter' && sel) navigate(`/day/${sel}`)
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const days = data?.days || []
  const past = days.filter(d => d.date <= today)
  const written = past.filter(d => d.entry).length
  const moods = past.filter(d => d.mood > 0)
  const mood = moods.length ? moods.reduce((a, d) => a + d.mood, 0) / moods.length : 0
  const layers = LAYER_IDS.map(id => ({ id, label: t(LAYER_KEY[id]), icon: LAYER_ICON[id] }))
  const available = layers.filter(l => days.some(d => l.id === 'written' ? d.entry : d.mood > 0))
  const active = available.find(l => l.id === layer)?.id || available[0]?.id || 'mood'
  const weekStart = settings?.week_start === 'sun' ? 0 : 1
  const lead = (new Date(y, m - 1, 1).getDay() - weekStart + 7) % 7
  const labels = weekdayLetters(weekStart as 0 | 1)
  const isCurrent = month === today.slice(0, 7)

  return (
    <div className="cal2">
      <div className="cal2-head">
        <div>
          <h1 className="cal2-title">{monthName(m)} <span>{y}</span></h1>
          <div className="cal2-sub">{past.length ? t('cal.days_written', { a: written, b: past.length }) : t('cal.not_started')}{isCurrent ? '' : ` · ${t('cal.days', { n: daysIn(y, m) })}`}</div>
        </div>
        <div className="cal2-nav">
          <button type="button" className="btn icon-only" onClick={() => go(-1)} title={t('cal.prev_month')} aria-label={t('cal.prev_month')}><Icon name="chevron-left" size={18} /></button>
          <Link className={`btn ${isCurrent ? 'primary' : ''}`} to="/calendar">{t('cal.today')}</Link>
          <button type="button" className="btn icon-only" onClick={() => go(1)} disabled={month >= today.slice(0, 7)} title={t('cal.next_month')} aria-label={t('cal.next_month')}><Icon name="chevron-right" size={18} /></button>
        </div>
      </div>

      <div className="month-strip">
        <Link className="ms-year" to={`/year/${y}`} title={t('cal.year_review', { y })}>{y}</Link>
        {Array.from({ length: 12 }, (_, i) => {
          const name = monthName(i + 1, false)
          const key = ym(y, i + 1)
          const r = yearData?.months?.[i]
          const total = key === today.slice(0, 7) ? Number(today.slice(8)) : daysIn(y, i + 1)
          const pct = r && total ? Math.min(100, r.entries / total * 100) : 0
          const future = key > today.slice(0, 7)
          return <Link key={key} to={future ? '#' : `/calendar/${key}`} onClick={e => future && e.preventDefault()} className={`ms ${key === month ? 'on' : ''} ${future ? 'future' : ''}`} title={r ? (r.mood ? t('cal.month_written_mood', { n: r.entries, m: r.mood }) : t('cal.month_written', { n: r.entries })) : name}>
            <span>{name}</span><i><b style={{ width: `${pct}%`, background: r?.mood ? MOOD_COLOR[Math.round(r.mood)] : undefined }} /></i>
          </Link>
        })}
      </div>

      <div className="cal2-stats">
        <div className="tile ring-tile"><Ring value={past.length ? written / past.length : 0} /><div><b>{written}<span>/{past.length}</span></b><small>{t('cal.written')}</small></div></div>
        {mood > 0 && <div className="tile"><span className="ti">{MOOD_ICON[Math.round(mood)]}</span><div><b>{mood.toFixed(1)}</b><small>{t('cal.avg_mood')}</small></div></div>}
      </div>

      <div className="cal2-toolbar">
        <div className="layer-switch" role="radiogroup" aria-label={t('cal.colored_by')}>
          {available.map(l => <button type="button" key={l.id} role="radio" aria-checked={active === l.id} className={active === l.id ? 'on' : ''} onClick={() => setLayer(l.id)}><Icon name={l.icon} size={14} />{l.label}</button>)}
        </div>
        <span className="muted small hide-sm">{t('cal.hint')}</span>
      </div>

      {loading && !data && <Loading />}
      {error && <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>}
      {data && <div className={`cal2-body ${wide && sel ? 'with-panel' : ''}`}>
        <div className="cal2-grid-wrap">
          <div className="cal2-wd">{labels.map(l => <div key={l}>{l}</div>)}</div>
          <div className="cal2-grid">
            {Array.from({ length: lead }).map((_, i) => <div key={'l' + i} className="c2 blank" />)}
            {days.map(d => <Cell key={d.date} d={d} layer={active} today={today} selected={d.date === sel}
              onClick={() => { if (d.date > today) return; if (!wide) navigate(`/day/${d.date}`); else if (sel === d.date) navigate(`/day/${d.date}`); else setSel(d.date) }} />)}
          </div>
          <Legend layer={active} />
        </div>
        {wide && sel && <Preview date={sel} onClose={() => setSel('')} />}
      </div>}

      {aiOn && month <= today.slice(0, 7) && <div className="mt"><StoryCard key={month} kind="month" period={month} label={`${monthName(m)} ${y}`} /></div>}
    </div>
  )
}

function useWide() {
  const [w, setW] = useState(() => window.matchMedia('(min-width: 1100px)').matches)
  useEffect(() => {
    const q = window.matchMedia('(min-width: 1100px)')
    const h = () => setW(q.matches)
    q.addEventListener('change', h)
    return () => q.removeEventListener('change', h)
  }, [])
  return w
}

export function Ring({ value }: { value: number }) {
  const r = 16, c = 2 * Math.PI * r
  return <svg className="ring" viewBox="0 0 40 40" width="40" height="40" aria-hidden><circle cx="20" cy="20" r={r} className="ring-bg" /><circle cx="20" cy="20" r={r} className="ring-fg" strokeDasharray={`${c * value} ${c}`} transform="rotate(-90 20 20)" /></svg>
}

function Cell({ d, layer, today, selected, onClick }: { d: CalendarDay; layer: Layer; today: string; selected: boolean; onClick: () => void }) {
  const { t } = useI18n()
  const future = d.date > today
  const n = Number(d.date.slice(8))
  let style: React.CSSProperties = {}
  if (!future) {
    if (layer === 'mood' && d.mood) style = { background: `color-mix(in srgb, ${MOOD_COLOR[d.mood]} 32%, var(--surface))` }
    else if (d.entry) style = { background: `color-mix(in srgb, var(--accent) 14%, var(--surface))` }
  }
  return (
    <button type="button" className={`c2 ${future ? 'future' : ''} ${d.date === today ? 'today' : ''} ${selected ? 'sel' : ''} ${d.entry ? 'written' : ''}`} style={style} onClick={onClick} disabled={future}>
      <span className="c2-top">
        <span className="c2-n">{n}</span>
        {d.entry && <span className="c2-dot" title={t('cal.written')} />}
        <span className="spacer" />
        {d.pinned && <span title={t('cal.kept')} style={{ display: 'inline-grid', color: 'var(--star)' }}><Icon name="star" size={12} strokeWidth={2.2} /></span>}
        {d.mood > 0 && <span className="c2-mood" title={MOODS[d.mood]}>{MOOD_ICON[d.mood]}</span>}
      </span>
      {d.highlight && <span className="c2-hl">{d.highlight}</span>}
      <span className="c2-foot">{d.moments > 0 && <span className="c2-mom">{t(d.moments === 1 ? 'cal.moment' : 'cal.moments', { n: d.moments })}</span>}<span className="spacer" /></span>
    </button>
  )
}

function Legend({ layer }: { layer: Layer }) {
  const { t } = useI18n()
  if (layer === 'mood') return <p className="cal2-legend">{[1, 2, 3, 4, 5].map(i => <span key={i}><i style={{ background: MOOD_COLOR[i] }} />{MOODS[i]}</span>)}</p>
  return <p className="cal2-legend"><i style={{ background: 'color-mix(in srgb, var(--accent) 14%, var(--surface))' }} />{t('cal.written')}</p>
}

/** The selected day at a glance, without leaving the month. */
function Preview({ date, onClose }: { date: string; onClose: () => void }) {
  const { t } = useI18n()
  const { data, loading } = useAsync(() => api.get<DayPage>(`${V1}/days/${date}`), [date])
  return (
    <aside className="cal2-panel">
      <div className="row"><div className="grow"><div className="muted small">{weekday(date, true)}</div><h2>{fmtDate(date)}</h2></div><button type="button" className="btn ghost icon-only" onClick={onClose} aria-label={t('cal.close')} title={t('cal.close')}><Icon name="x" size={17} /></button></div>
      {loading && !data && <Loading />}
      {data && <>
        {data.entry ? <div className="pv-entry">
          <div className="row" style={{ gap: 8 }}>{data.entry.mood > 0 && <span className="pv-mood" style={{ background: `color-mix(in srgb, ${MOOD_COLOR[data.entry.mood]} 25%, transparent)` }}>{MOOD_ICON[data.entry.mood]} {MOODS[data.entry.mood]}</span>}{data.entry.pinned && <span className="badge"><Icon name="star" size={11} />{t('cal.kept').toLowerCase()}</span>}</div>
          {data.entry.highlight && <h3>{data.entry.highlight}</h3>}
          {data.entry.text && <p className="pv-text">{data.entry.text}</p>}
        </div> : <p className="muted">{t('cal.nothing_written')}</p>}
        {data.moments.length > 0 && <div className="pv-moments">{data.moments.slice(0, 4).map(mo => <div key={mo.id}><span className="muted mono small">{mo.time}</span> {mo.text}</div>)}{data.moments.length > 4 && <div className="muted small">{t('cal.more', { n: data.moments.length - 4 })}</div>}</div>}
        <Link className="btn primary block" to={`/day/${date}`}>{data.entry ? t('cal.open_day') : t('cal.write_about')}<Icon name="chevron-right" size={15} /></Link>
      </>}
    </aside>
  )
}
