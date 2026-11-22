// The app shell: sidebar, top bar, search palette, shortcuts — the Zan Calendar layout.
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { addDays, fmtDate, monthName, todayISO, weekday } from '../lib/format'
import { AUTO, LANGUAGES, useI18n } from '../lib/i18n'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { Avatar, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { kanjiFor, Scramble } from '../lib/fx'

const SIDEBAR_KEY = 'zan-sidebar'
/** Where Settings goes back to. */
export const LAST_PAGE_KEY = 'zan-last-page'

/** An entry or a moment was saved: the sidebar (and anything listening) reloads. */
export const announceChange = () => window.dispatchEvent(new CustomEvent('zan:changed'))
export function useOnChange(cb: () => void) {
  const ref = useRef(cb)
  ref.current = cb
  useEffect(() => {
    const h = () => ref.current()
    window.addEventListener('zan:changed', h)
    return () => window.removeEventListener('zan:changed', h)
  }, [])
}

export type WriteKind = 'entry' | 'moment'

export default function Layout({ children }: { children?: ReactNode }) {
  const { settings, setThemeMode, theme } = useApp()
  const { t } = useI18n()
  const navigate = useNavigate()
  const loc = useLocation()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  // Page-in on every navigation (skipped on first mount).
  const [entering, setEntering] = useState(false)
  const firstNav = useRef(true)
  useEffect(() => {
    if (firstNav.current) { firstNav.current = false; return }
    setEntering(true)
    const e = setTimeout(() => setEntering(false), 900)
    return () => clearTimeout(e)
  }, [loc.pathname])
  useEffect(() => { sessionStorage.setItem(LAST_PAGE_KEY, loc.pathname + loc.search) }, [loc.pathname, loc.search])

  const [palette, setPalette] = useState(false)
  const ai = !!settings?.ai_configured

  /** Today's page with the entry (or the moment field) focused. */
  const write = (kind: WriteKind) => { closeDrawer(); navigate('/', { state: { focus: kind } }) }
  const writeRef = useRef(write)
  writeRef.current = write

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t?.closest?.('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); window.dispatchEvent(new CustomEvent('zan:shortcuts')); break
        case 'n': e.preventDefault(); writeRef.current('entry'); break
        case 'm': e.preventDefault(); writeRef.current('moment'); break
        case 't': go('/'); break
        case 'c': go('/calendar'); break
        case 'j': go('/journal'); break
        case 'y': go('/year'); break
        case 'a': if (ai) go('/ask'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const today = todayISO()
  const commands: PaletteCommand[] = [
    { id: 'write', label: t('palette.write'), icon: 'edit', hint: 'N', keywords: 'new entry journal', run: () => write('entry') },
    { id: 'moment', label: t('palette.moment'), icon: 'clock', hint: 'M', keywords: 'note now timeline', run: () => write('moment') },
    { id: 'today', label: t('palette.today'), icon: 'today', hint: 'T', run: () => navigate('/') },
    { id: 'yesterday', label: t('palette.yesterday'), icon: 'calendar', keywords: 'day before', run: () => navigate(`/day/${addDays(today, -1)}`) },
    { id: 'calendar', label: t('palette.calendar'), icon: 'calendar', hint: 'C', keywords: 'month', run: () => navigate('/calendar') },
    { id: 'journal', label: t('palette.journal'), icon: 'book-open', hint: 'J', keywords: 'entries list search', run: () => navigate('/journal') },
    { id: 'kept', label: t('palette.kept'), icon: 'star', keywords: 'starred pinned', run: () => navigate('/journal?pinned=1') },
    { id: 'year', label: t('palette.year'), icon: 'chart', hint: 'Y', run: () => navigate('/year') },
    ...(ai ? [
      { id: 'ask', label: t('palette.ask'), icon: 'sparkles', hint: 'A', keywords: 'ai question', run: () => navigate('/ask') },
      { id: 'stories', label: t('palette.stories'), icon: 'book', keywords: 'ai week month year story', run: () => navigate('/stories') },
    ] : []),
    { id: 'settings', label: t('palette.settings'), icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'ai', label: t('palette.ai'), icon: 'sparkles', keywords: 'ai openai key model provider', run: () => navigate('/settings/ai') },
    { id: 'notify', label: t('palette.notify'), icon: 'bell', keywords: 'push evening morning', run: () => navigate('/settings/notifications') },
    { id: 'theme', label: theme === 'dark' ? t('palette.light') : t('palette.dark'), icon: theme === 'dark' ? 'sun' : 'moon', keywords: 'theme appearance', run: () => setThemeMode(theme === 'dark' ? 'light' : 'dark') },
    { id: 'shortcuts', label: t('palette.shortcuts'), icon: 'keyboard', hint: '?', run: () => window.dispatchEvent(new CustomEvent('zan:shortcuts')) },
  ]

  return (
    <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
      {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
      <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
        <Sidebar onWrite={write} onNavigate={closeDrawer} onPalette={() => setPalette(true)} />
      </div>
      <div className="main">
        <div className="fx-watermark" aria-hidden>{kanjiFor(loc.pathname)}</div>
        <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} />
        <main className={`content ${entering ? 'fx-entering' : ''}`}><div className="content-inner">{children ?? <Outlet />}</div></main>
      </div>
      {mobile && <button type="button" className="fab" aria-label={t('sidebar.write_today')} title={t('sidebar.write_today')} onClick={() => write('entry')}><Icon name="pen" size={22} strokeWidth={2} /></button>}
      {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
    </div>
  )
}

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const { t, choice, lang, setLang } = useI18n()
  const navigate = useNavigate()
  const loc = useLocation()
  const parts = loc.pathname.split('/')
  const today = todayISO()
  const SECTION_T: Record<string, string> = { '': t('nav.today'), memories: t('nav.memories'), posts: t('nav.posts'), calendar: t('nav.calendar'), journal: t('nav.journal'), year: t('nav.year'), ask: t('nav.ask'), stories: t('nav.stories') }
  let title: ReactNode = SECTION_T[parts[1] || ''] || ''
  if (parts[1] === 'day') {
    const raw = parts[2] || ''
    const date = raw === 'yesterday' ? addDays(today, -1) : raw === 'today' ? today : raw
    if (date === today) title = t('nav.today')
    else if (/^\d{4}-\d{2}-\d{2}$/.test(date)) {
      const [y, m] = date.split('-').map(Number)
      // phones: the month only (the page says which day), still a way back to the calendar
      title = mobile
        ? <button type="button" className="crumb" style={{ color: 'inherit' }} onClick={() => navigate(`/calendar/${date.slice(0, 7)}`)}>{monthName(m)} {y}</button>
        : <><button type="button" className="crumb" onClick={() => navigate(`/calendar/${date.slice(0, 7)}`)}>{monthName(m)} {y}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{weekday(date, true)}, {fmtDate(date)}</span></>
    }
  } else if (parts[1] === 'calendar' && /^\d{4}-\d{2}$/.test(parts[2] || '')) {
    title = <><button type="button" className="crumb" onClick={() => navigate('/calendar')}>{t('nav.calendar')}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{monthName(Number(parts[2].slice(5)))} {parts[2].slice(0, 4)}</span></>
  } else if (parts[1] === 'year' && /^\d{4}$/.test(parts[2] || '')) {
    title = <><button type="button" className="crumb" onClick={() => navigate('/year')}>{t('nav.year')}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span>{parts[2]}</span></>
  }
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: t('topbar.profile'), icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: t('topbar.settings'), icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: t('topbar.shortcuts'), icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('zan:shortcuts')), hint: '?' },
    'sep',
    { heading: t('topbar.appearance') },
    { label: t('topbar.system'), checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: t('topbar.light'), checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: t('topbar.dark'), checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    'sep',
    { heading: t('topbar.language') },
    ...[AUTO, ...LANGUAGES.map(l => l.code)].map(c => ({
      label: c === AUTO ? t('st.language_auto') : LANGUAGES.find(l => l.code === c)!.native,
      checked: choice === c || (c === AUTO && choice === AUTO),
      onSelect: () => setLang(c),
    })),
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: t('topbar.sign_out'), icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? t('topbar.hide_sidebar') : t('topbar.show_sidebar')} title={t('topbar.menu')}><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">{typeof title === 'string' ? <Scramble text={title} /> : title}<span className="tb-kanji" aria-hidden>{kanjiFor(loc.pathname)}</span></h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label={t('topbar.search')} title={`${t('topbar.search')} (⌘K or /)`}><Icon name="search" size={18} /></button>
      <MenuButton label={t('topbar.account')} className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
