import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { fmtDate, intlLocale, monthName, todayISO, weekdayLetters } from '../lib/format'
import { useI18n } from '../lib/i18n'
import { MOODS, MOOD_COLOR, MOOD_ICON, cap, type CalendarDay, type YearRecap } from '../lib/types'

import { Card, Empty, Loading, useAsync } from '../components/ui'
import { Icon } from '../components/icons'
import { StoryCard, useAI } from '../components/ai'
import { Ring } from './Calendar'

const MON = (m: number) => monthName(m, false)
type Metric = 'written' | 'mood'
const METRIC_KEY: Record<Metric, string> = { written: 'metric.written', mood: 'metric.mood' }
const METRIC_ICON: Record<Metric, string> = { written: 'pen', mood: 'smile' }
const METRIC_IDS: Metric[] = ['written', 'mood']
const compact = (n: number) => n >= 10000 ? new Intl.NumberFormat(intlLocale(), { notation: 'compact', maximumFractionDigits: 1 }).format(n) : Math.round(n).toLocaleString(intlLocale())

export default function Year() {
  const { year: yp } = useParams()
  const navigate = useNavigate()
  const { t } = useI18n()
  const aiOn = useAI()
  const today = todayISO()
  const thisYear = Number(today.slice(0, 4))
  const year = yp && /^\d{4}$/.test(yp) ? Number(yp) : thisYear
  const { data, loading, error } = useAsync(() => api.get<YearRecap>(`${V1}/years/${year}`), [year])
  const { data: cal } = useAsync(() => api.get<{ days: CalendarDay[] }>(`${V1}/calendar?from=${year}-01-01&to=${year}-12-31`), [year])
  const { data: years } = useAsync(() => api.get<{ year: number; entries: number }[]>(`${V1}/years`), [])
  const [metric, setMetricRaw] = useState<Metric>(() => { const v = localStorage.getItem('zan-year-metric'); return v === 'mood' ? 'mood' : 'written' })
  const setMetric = (m: Metric) => { setMetricRaw(m); localStorage.setItem('zan-year-metric', m) }
  const days = cal?.days || []
  const writtenDays = days.filter(d => d.entry).length
  const metrics = METRIC_IDS.map(id => ({ id, label: t(METRIC_KEY[id]), icon: METRIC_ICON[id] }))
  const available = metrics.filter(m => days.some(d => m.id === 'written' ? d.entry : d.mood > 0))
  const active = available.find(m => m.id === metric)?.id || available[0]?.id || 'written'

  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return null
  const d = data
  const moodsTotal = d.moods.slice(1).reduce((a, b) => a + b, 0)
  const empty = d.entries === 0
  const lived = year === thisYear ? Math.round((Date.parse(today) - Date.parse(`${year}-01-01`)) / 86400000) + 1 : 365
  const yearList = (years || []).map(y => y.year)

  return (
    <div className="yr">
      <div className="cal2-head">
        <div>
          <h1 className="cal2-title">{year} <span>{t('y.in_review')}</span></h1>
          <div className="cal2-sub">{empty ? t('y.nothing') : `${t('y.days_recorded', { a: writtenDays })} · ${t('y.written', { b: d.entries })}${d.longest_streak > 1 ? ` · ${t('y.longest_streak', { n: d.longest_streak })}` : ''}`}</div>
        </div>
        <div className="cal2-nav">
          <button type="button" className="btn icon-only" onClick={() => navigate(`/year/${year - 1}`)} title={String(year - 1)} aria-label={String(year - 1)}><Icon name="chevron-left" size={18} /></button>
          {yearList.length > 1 ? yearList.slice().sort().map(y => <Link key={y} className={`btn ${y === year ? 'primary' : ''}`} to={`/year/${y}`}>{y}</Link>) : <span className="btn primary">{year}</span>}
          <button type="button" className="btn icon-only" onClick={() => navigate(`/year/${year + 1}`)} disabled={year >= thisYear} title={String(year + 1)} aria-label={String(year + 1)}><Icon name="chevron-right" size={18} /></button>
        </div>
      </div>

      {empty ? <Card><Empty icon="chart" title={t('y.empty_title')}>{t('y.empty_body')}</Empty></Card> : <>
        <div className="cal2-stats">
          <div className="tile ring-tile"><Ring value={lived ? writtenDays / lived : 0} /><div><b>{writtenDays}<span>/{lived}</span></b><small>{t('y.days_rec')}</small></div></div>
          <div className="tile"><span className="ti"><Icon name="pen" size={18} /></span><div><b>{d.entries}</b><small>{t('y.written_n', { n: d.entries })}{d.words ? ` · ${t('tl.words_n', { n: compact(d.words) })}` : ''}</small></div></div>
          {d.mood > 0 && <div className="tile"><span className="ti">{MOOD_ICON[Math.round(d.mood)]}</span><div><b>{d.mood.toFixed(1)}</b><small>{t('y.avg_mood')}</small></div></div>}
        </div>

        {aiOn && <StoryCard key={year} kind="year" period={String(year)} label={String(year)} />}

        <Card className="mb" title={<><Icon name="grid" size={16} />{t('y.every_day')}</>} actions={<div className="layer-switch">{available.map(m => <button type="button" key={m.id} className={active === m.id ? 'on' : ''} onClick={() => setMetric(m.id)}><Icon name={m.icon} size={14} />{m.label}</button>)}</div>}>
          {cal ? <>
            <Heatmap year={year} days={days} metric={active} today={today} onOpen={iso => navigate(`/day/${iso}`)} />
            <MonthBars year={year} days={days} metric={active} today={today} onOpen={m => navigate(`/calendar/${m}`)} />
          </> : <Loading />}
        </Card>

        <div className="insights">
          {moodsTotal > 0 && <Card title={<><Icon name="activity" size={16} />{t('y.moods')}</>}>
            <div className="mood-bars">{[5, 4, 3, 2, 1].map(m => <div key={m} className="mb-row"><span>{MOOD_ICON[m]} {cap(MOODS[m])}</span><div className="bar-h"><div style={{ width: `${d.moods[m] / moodsTotal * 100}%`, background: MOOD_COLOR[m] }} /></div><span className="muted small">{d.moods[m]}</span></div>)}</div>
            {moodsTotal >= 7 && <div className="wd-mood">{weekdayLetters(1).map((w, i) => { const v = d.weekday_mood[i + 1]; return <div key={i} title={v ? v.toFixed(1) : t('y.no_mood')}><i style={{ height: `${v ? v / 5 * 100 : 0}%`, background: v ? MOOD_COLOR[Math.round(v)] : undefined }} /><span>{w}</span></div> })}</div>}
          </Card>}
          {d.pinned.length > 0 && <Card title={<><Icon name="star" size={16} />{t('y.kept_days')}</>}><DayList items={d.pinned} /></Card>}
          {d.best.length > 0 && <Card title={<><Icon name="sun" size={16} />{t('y.great_days')}</>}><DayList items={d.best} /></Card>}
          {d.tags.length > 0 && <Card title={<><Icon name="tag" size={16} />{t('y.tags')}</>}><div className="chips">{d.tags.map(x => <Link key={x.tag} className="chip" to={`/journal?tag=${encodeURIComponent(x.tag)}`}>#{x.tag} <span className="muted">{x.count}</span></Link>)}</div></Card>}
        </div>
      </>}
    </div>
  )
}

function metricValue(d: CalendarDay | undefined, m: Metric): number {
  if (!d) return 0
  if (m === 'written') return d.entry ? 1 : 0
  return d.mood
}
function fmtMetric(v: number, m: Metric, t: (k: string, vars?: Record<string, string | number>) => string): string {
  if (m === 'mood') return v ? `${v.toFixed(1).replace('.0', '')} ${MOODS[Math.round(v)]}` : t('y.no_mood')
  return t('y.written_n', { n: Math.round(v) })
}
function cellColor(v: number, m: Metric, max: number): string | undefined {
  if (!v) return undefined
  if (m === 'mood') return MOOD_COLOR[Math.round(v)]
  return 'var(--accent)'
}

/** 53 week columns × 7 weekday rows, GitHub style. */
function Heatmap({ year, days, metric, today, onOpen }: { year: number; days: CalendarDay[]; metric: Metric; today: string; onOpen: (iso: string) => void }) {
  const { t } = useI18n()
  const byDate = useMemo(() => new Map(days.map(d => [d.date, d])), [days])
  const max = useMemo(() => {
    const vals = days.map(d => metricValue(d, metric)).filter(v => v > 0).sort((a, b) => a - b)
    return vals.length ? vals[Math.floor(vals.length * 0.95)] || vals[vals.length - 1] : 1 // ignore the top 5% outliers
  }, [days, metric])
  const first = new Date(year, 0, 1)
  const lead = (first.getDay() + 6) % 7 // Monday first
  const total = (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0)) ? 366 : 365
  const cells: (string | null)[] = [...Array(lead).fill(null)]
  for (let i = 0; i < total; i++) { const t = new Date(year, 0, 1 + i); cells.push(`${year}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`) }
  const weeks = Math.ceil(cells.length / 7)
  const wrap = useRef<HTMLDivElement>(null)
  useEffect(() => { const el = wrap.current; if (el && el.scrollWidth > el.clientWidth) el.scrollLeft = el.scrollWidth }, [year, days.length]) // phones: start at the recent weeks
  const monthCols = Array.from({ length: 12 }, (_, mi) => Math.floor((lead + Math.round((Date.UTC(year, mi, 1) - Date.UTC(year, 0, 1)) / 86400000)) / 7))
  const WD: string[] = ['Mon', '', 'Wed', '', 'Fri', '', 'Sun']
  const WD_L: string[] = WD.map((w, i) => (w ? weekdayLetters(1)[i].toUpperCase() : ''))
  return (
    <div className="hm-wrap" ref={wrap}>
      <div className="hm" style={{ gridTemplateColumns: `28px repeat(${weeks}, minmax(9px, 1fr))` }}>
        <span />{Array.from({ length: weeks }).map((_, w) => <span key={w} className="hm-m">{monthCols.includes(w) ? MON(monthCols.lastIndexOf(w) + 1) : ''}</span>)}
        {[0, 1, 2, 3, 4, 5, 6].map(r => <Fragment key={r}>
          <span key={'l' + r} className="hm-l">{WD_L[r]}</span>
          {Array.from({ length: weeks }).map((_, w) => {
            const iso = cells[w * 7 + r]
            if (!iso) return <span key={`${r}-${w}`} className="hm-c none" />
            const d = byDate.get(iso)
            const v = metricValue(d, metric)
            const future = iso > today
            return <button type="button" key={iso} className={`hm-c ${future ? 'future' : ''} ${iso === today ? 'today' : ''} ${d?.pinned ? 'kept' : ''}`} style={{ background: future ? undefined : cellColor(v, metric, max) }} disabled={future}
              title={`${fmtDate(iso)} · ${fmtMetric(v, metric, t)}${d?.highlight ? ` · ${d.highlight}` : ''}`} onClick={() => onOpen(iso)} />
          })}
        </Fragment>)}
      </div>
      <div className="hm-legend"><span>{metric === 'mood' ? '' : t('y.less')}</span>{metric === 'mood' ? [1, 2, 3, 4, 5].map(m => <span key={m} className="hm-k"><i style={{ background: MOOD_COLOR[m] }} />{MOODS[m]}</span>) : [0, .25, .5, .75, 1].map(f => <i key={f} style={{ background: f ? cellColor(f * max, metric, max) : 'var(--surface-2)' }} />)}<span>{metric === 'mood' ? '' : t('y.more')}</span><span className="muted small hm-tip">{t('y.hover_tip')}</span></div>
    </div>
  )
}

/** The same metric per month: sums, or averages for mood. */
function MonthBars({ year, days, metric, today, onOpen }: { year: number; days: CalendarDay[]; metric: Metric; today: string; onOpen: (month: string) => void }) {
  const { t } = useI18n()
  const vals = Array.from({ length: 12 }, (_, i) => {
    const key = `${year}-${String(i + 1).padStart(2, '0')}`
    const ds = days.filter(d => d.date.startsWith(key))
    if (metric === 'mood') { const ms = ds.filter(d => d.mood > 0); return { key, v: ms.length ? ms.reduce((a, d) => a + d.mood, 0) / ms.length : 0 } }
    return { key, v: ds.reduce((a, d) => a + metricValue(d, metric), 0) }
  })
  const max = metric === 'mood' ? 5 : Math.max(1, ...vals.map(x => x.v))
  const label = (v: number) => {
    if (!v) return ''
    if (metric === 'mood') return v.toFixed(1)
    return compact(v)
  }
  return (
    <div className="mbars">
      {vals.map((x, i) => {
        const future = x.key > today.slice(0, 7)
        return <button type="button" key={x.key} className={`mbar ${future ? 'future' : ''} ${x.key === today.slice(0, 7) ? 'now' : ''}`} onClick={() => !future && onOpen(x.key)} disabled={future} title={`${MON(i + 1)} · ${label(x.v) || t('y.nothing_val')}`}>
          <span className="mb-v">{label(x.v)}</span>
          <span className="mb-track"><i style={{ height: `${x.v / max * 100}%`, background: metric === 'mood' && x.v ? MOOD_COLOR[Math.round(x.v)] : undefined }} /></span>
          <span className="mb-m">{MON(i + 1)}</span>
        </button>
      })}
    </div>
  )
}

function DayList({ items }: { items: YearRecap['pinned'] }) {
  const { t } = useI18n()
  return <div className="inline-list">{items.map(e => <Link key={e.id} className="li" to={`/day/${e.date}`} style={{ color: 'inherit' }}><span className="muted mono small nowrap">{fmtDate(e.date, false)}</span><span className="grow">{e.highlight || e.text.split('\n')[0] || t('sidebar.no_words')}</span>{e.mood > 0 && <span>{MOOD_ICON[e.mood]}</span>}</Link>)}</div>
}
