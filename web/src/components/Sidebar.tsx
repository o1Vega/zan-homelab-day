// Left column: write, sections, the month, recent days, settings — the Zan Calendar sidebar.
import { type ReactNode, useEffect, useState } from 'react'
import { Link, NavLink, useMatch, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { addDays, todayISO } from '../lib/format'
import { useI18n } from '../lib/i18n'
import { MOODS, MOOD_ICON, type CalendarDay, type HomeData } from '../lib/types'
import { Icon } from './icons'
import { MiniMonth } from './MiniMonth'
import { useOnChange, type WriteKind } from './Layout'
import { type Anchor, type MenuItem, Menu, rectOf } from './ui'

const short = (iso: string) => { const [y, m, d] = iso.split('-').map(Number); return new Date(y, m - 1, d).toLocaleDateString(undefined, { day: 'numeric', month: 'short' }) }

export function Sidebar({ onWrite, onNavigate, onPalette }: { onWrite: (k: WriteKind) => void; onNavigate: () => void; onPalette: () => void }) {
  const { status, settings } = useApp()
  const { t } = useI18n()
  const navigate = useNavigate()
  const today = todayISO()
  const dm = useMatch('/day/:date')
  const home = useMatch('/')
  const current = dm?.params.date && /^\d{4}-\d{2}-\d{2}$/.test(dm.params.date) ? dm.params.date : dm?.params.date === 'yesterday' ? addDays(today, -1) : home || dm?.params.date === 'today' ? today : ''
  const [menu, setMenu] = useState<Anchor | null>(null)
  const [month, setMonth] = useState((current || today).slice(0, 7))
  useEffect(() => { if (current) setMonth(current.slice(0, 7)) }, [current])
  const [dots, setDots] = useState<Set<string>>(new Set())
  const [homeData, setHome] = useState<HomeData | null>(null)
  const [tick, setTick] = useState(0)
  useOnChange(() => setTick(t => t + 1))
  useEffect(() => {
    let live = true
    api.get<{ days: CalendarDay[] }>(`${V1}/calendar?month=${month}`).then(r => { if (live) setDots(new Set((r.days || []).filter(d => d.entry).map(d => d.date))) }).catch(() => {})
    return () => { live = false }
  }, [month, tick])
  useEffect(() => {
    let live = true
    api.get<HomeData>(`${V1}/home`).then(r => { if (live) setHome(r) }).catch(() => {})
    return () => { live = false }
  }, [tick])
  const [recentOpen, setRecentOpen] = useState(true)
  const title = settings?.title || status?.title || t('app.name')
  const ai = !!settings?.ai_configured
  const streak = homeData?.streak || 0
  const writeItems: MenuItem[] = [
    { label: t('nav.new_memory'), icon: 'sparkles', onSelect: () => { onNavigate(); navigate('/memory/new') } },
    { label: t('sidebar.todays_entry'), icon: 'edit', hint: 'N', onSelect: () => onWrite('entry') },
    { label: t('sidebar.moment_now'), icon: 'clock', hint: 'M', onSelect: () => onWrite('moment') },
    'sep',
    { label: t('sidebar.yesterday'), icon: 'calendar', onSelect: () => { onNavigate(); navigate(`/day/${addDays(today, -1)}`) } },
    { label: t('sidebar.another_day'), icon: 'search', hint: '/', onSelect: () => { onNavigate(); onPalette() } },
  ]
  const nav: { to: string; label: string; icon: string; k: string; end?: boolean; extra?: ReactNode }[] = [
    { to: '/', label: t('nav.today'), icon: 'today', k: '今', end: true, extra: streak > 0 ? <span className="sb-streak" title={t('sidebar.streak', { n: streak })}><Icon name="flame" size={13} strokeWidth={2} />{streak}</span> : undefined },
    { to: '/memories', label: t('nav.memories'), icon: 'archive', k: '残' },
    { to: '/posts', label: t('nav.posts'), icon: 'globe', k: '帖' },
    { to: '/people', label: t('nav.people'), icon: 'users', k: '人' },
    { to: '/calendar', label: t('nav.calendar'), icon: 'calendar', k: '暦' },
    { to: '/journal', label: t('nav.journal'), icon: 'book-open', k: '記', extra: homeData?.stats.total ? <span className="badge" title={t('sidebar.days_written')}>{homeData.stats.total}</span> : undefined },
    { to: '/year', label: t('nav.year'), icon: 'chart', k: '年' },
    ...(ai ? [{ to: '/ask', label: t('nav.ask'), icon: 'sparkles', k: '問' }, { to: '/stories', label: t('nav.stories'), icon: 'book', k: '話' }] : []),
  ]
  const recent = (homeData?.recent || []).slice(0, 6)
  return (
    <aside className="sidebar" aria-label="Day">
      <div className="sb-brand">
        <img src="/favicon.svg" alt={title} className="sb-logo" />
        <span className="sb-brand-name"><span className="kanji-seal" aria-hidden>残</span>ZANZŌ</span>
        {status?.version && <span className="sb-version" title={t('sidebar.version', { v: status.version })}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
        <Icon name="pen" size={16} strokeWidth={2.1} />{t('nav.write')}<Icon name="chevron-down" size={15} className="create-caret" />
      </button>
      {menu && <Menu anchor={menu} items={writeItems} onClose={() => setMenu(null)} width={236} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} end={n.end} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive || (n.to === '/' && current === today) ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}<span className="sb-kanji" aria-hidden>{n.k}</span>{n.extra}
          </NavLink>
        ))}
      </nav>

      <MiniMonth month={month} onMonth={setMonth} today={today} selected={current} weekStart={settings?.week_start === 'sun' ? 0 : 1} dots={dots}
        onTitle={() => { onNavigate(); navigate(`/calendar/${month}`) }} onPick={d => { onNavigate(); navigate(d === today ? '/' : `/day/${d}`) }} />

      <section className="sb-sec">
        <h4 className="sb-h">
          <button type="button" className="sb-h-btn" aria-expanded={recentOpen} onClick={() => setRecentOpen(o => !o)}><Icon name={recentOpen ? 'chevron-down' : 'chevron-right'} size={13} />{t('sidebar.recently')}</button>
          <button type="button" className="btn ghost icon-only xs" aria-label={t('sidebar.write_today')} title={`${t('sidebar.write_today')} (n)`} onClick={() => onWrite('entry')}><Icon name="plus" size={14} /></button>
        </h4>
        {recentOpen && (recent.length === 0
          ? <p className="sb-empty">{t('sidebar.nothing_yet')} <button type="button" className="link-btn" onClick={() => onWrite('entry')}>{t('sidebar.write_today')}</button></p>
          : <ul className="sb-days">{recent.map(e => (
            <li key={e.id}>
              <Link to={e.date === today ? '/' : `/day/${e.date}`} onClick={onNavigate} className={`sb-day ${e.date === current ? 'active' : ''}`} title={e.highlight || e.text.split('\n')[0]}>
                <span className="sb-day-date">{e.date === today ? t('sidebar.today') : short(e.date)}</span>
                <span className="sb-day-hl">{e.highlight || e.text.split('\n')[0] || <span className="muted">{t('sidebar.no_words')}</span>}</span>
                {e.mood > 0 && <span className="sb-day-mood" title={MOODS[e.mood]}>{MOOD_ICON[e.mood]}</span>}
              </Link>
            </li>
          ))}</ul>)}
      </section>

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />{t('nav.settings')}</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('zan:shortcuts')) }}><Icon name="keyboard" size={15} />{t('nav.shortcuts')}</button>
      </div>
    </aside>
  )
}
