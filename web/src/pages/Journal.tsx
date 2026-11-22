import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { monthName, weekday } from '../lib/format'
import { useI18n } from '../lib/i18n'
import { MOODS, MOOD_ICON, tagList, type Entry } from '../lib/types'
import { Card, Empty, Loading, PageHeader, Segmented, useAsync, useDebounce } from '../components/ui'
import { Icon } from '../components/icons'

interface Resp { entries: Entry[]; total: number; limit: number; offset: number }

export default function Journal() {
  const { t } = useI18n()
  const [sp, setSp] = useSearchParams()
  const [q, setQ] = useState(sp.get('q') || '')
  const dq = useDebounce(q, 300)
  const tag = sp.get('tag') || ''
  const mood = Number(sp.get('mood') || 0)
  const pinned = sp.get('pinned') === '1'
  const [page, setPage] = useState(0)
  const limit = 20
  useEffect(() => { setPage(0) }, [dq, tag, mood, pinned])
  const { data, loading, error } = useAsync(() => api.get<Resp>(`${V1}/entries?q=${encodeURIComponent(dq)}&tag=${encodeURIComponent(tag)}&mood=${mood}&pinned=${pinned ? 1 : 0}&limit=${limit}&offset=${page * limit}`), [dq, tag, mood, pinned, page])
  const { data: tags } = useAsync(() => api.get<{ tag: string; count: number }[]>(`${V1}/tags`), [])
  const set = (k: string, v: string) => { const n = new URLSearchParams(sp); if (v) n.set(k, v); else n.delete(k); setSp(n) }
  const pages = data ? Math.ceil(data.total / limit) : 0
  return (
    <>
      <PageHeader kanji="記" title={t('j.title')} sub={data ? (data.total === 1 ? t('j.day_written', { n: 1 }) : t('j.days_written', { n: data.total })) : ''} actions={<Link className="btn primary" to="/" state={{ focus: 'entry' }}><Icon name="pen" size={15} />{t('j.write_today')}</Link>} />
      <div className="toolbar">
        <div className="search"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder={t('j.search')} aria-label={t('j.search')} /></div>
        <Segmented size="sm" value={mood} onChange={v => set('mood', v ? String(v) : '')} options={[{ value: 0, label: t('j.any_mood') }, ...[1, 2, 3, 4, 5].map(m => ({ value: m, label: MOOD_ICON[m], title: MOODS[m] }))]} />
        <button type="button" className={`btn sm ${pinned ? 'primary' : ''}`} onClick={() => set('pinned', pinned ? '' : '1')} aria-pressed={pinned}><Icon name="star" size={14} />{t('j.kept')}</button>
        {tag && <button type="button" className="chip on" onClick={() => set('tag', '')} title={t('j.clear_tag')}>#{tag}<Icon name="x" size={13} /></button>}
      </div>
      {tags && tags.length > 0 && !tag && <div className="chips mb">{tags.slice(0, 20).map(x => <button type="button" key={x.tag} className="chip" onClick={() => set('tag', x.tag)}>#{x.tag} <span className="muted">{x.count}</span></button>)}</div>}
      {loading && !data && <Loading />}
      {error && <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>}
      {data && data.entries.length === 0 && <Card><Empty icon={dq || tag || mood || pinned ? 'search' : 'book-open'} title={dq || tag || mood || pinned ? t('j.no_match_title') : t('j.empty_title')}>{dq || tag || mood || pinned ? t('j.no_match_body') : t('j.empty_body')}</Empty></Card>}
      <div className="journal">
        {data?.entries.map(e => <EntryRow key={e.id} e={e} />)}
      </div>
      {pages > 1 && <div className="row mt" style={{ justifyContent: 'center' }}><button type="button" className="btn sm" disabled={page === 0} onClick={() => setPage(p => p - 1)}><Icon name="chevron-left" size={14} />{t('j.newer')}</button><span className="muted small">{page + 1} / {pages}</span><button type="button" className="btn sm" disabled={page + 1 >= pages} onClick={() => setPage(p => p + 1)}>{t('j.older')}<Icon name="chevron-right" size={14} /></button></div>}
    </>
  )
}

function EntryRow({ e }: { e: Entry }) {
  const { t } = useI18n()
  return (
    <Link to={`/day/${e.date}`} className="jrow" style={{ color: 'inherit' }}>
      <div className="jdate"><div className="d">{e.date.slice(8)}</div><div className="m">{monthName(Number(e.date.slice(5, 7)), false)} {e.date.slice(0, 4) !== String(new Date().getFullYear()) ? e.date.slice(0, 4) : ''}</div><div className="muted small">{weekday(e.date)}</div></div>
      <div className="jbody">
        <div className="row" style={{ gap: 8 }}>{e.mood > 0 && <span title={MOODS[e.mood]}>{MOOD_ICON[e.mood]}</span>}<b className="grow">{e.highlight || (e.text ? firstLine(e.text) : <span className="muted">{t('sidebar.no_words')}</span>)}</b>{e.pinned && <span title={t('j.kept')} style={{ color: 'var(--star)', display: 'inline-grid' }}><Icon name="star" size={14} strokeWidth={2.2} /></span>}</div>
        {e.highlight && e.text && <div className="muted clamp2">{e.text}</div>}
        <div className="row wrap small muted" style={{ gap: 6, marginTop: 6 }}>
          {tagList(e.tags).map(tag => <span key={tag} className="chip sm">#{tag}</span>)}
          {(e.moments || 0) > 0 && <span className="row" style={{ gap: 3 }}><Icon name="clock" size={12} />{t(e.moments! === 1 ? 'j.moment' : 'j.moments', { n: e.moments! })}</span>}
          {e.words ? <span>{t('j.words', { n: e.words! })}</span> : null}
        </div>
      </div>
    </Link>
  )
}

function firstLine(t: string) { return t.split('\n')[0] }
