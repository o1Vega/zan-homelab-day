// The diary: the day's folders as a grid of posters. Open one and the day
// looks and sounds like it did.
import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { assetUrl, listMemories } from '../lib/mem-api'
import { memoryArt, type Memory } from '../lib/types'
import { useI18n } from '../lib/i18n'
import { monthName, weekday } from '../lib/format'
import { Card, Empty, Loading, PageHeader, useAsync, useDebounce } from '../components/ui'
import { Icon } from '../components/icons'

function PaletteDots({ m }: { m: Memory }) {
  const art = memoryArt(m)
  const pal = art?.palette
  if (!pal || pal.length < 2) return null
  return (
    <span className="row" style={{ gap: 3 }} aria-hidden>
      {pal.slice(0, 4).map((c, i) => <span key={i} className="pdot" style={{ background: c }} />)}
    </span>
  )
}

export default function Memories() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const { data, loading, error } = useAsync(() => listMemories(), [])
  const list = useMemo(() => {
    if (!data) return []
    if (!dq.trim()) return data
    const s = dq.toLowerCase()
    return data.filter((m) => (m.title + ' ' + m.text + ' ' + m.date).toLowerCase().includes(s))
  }, [data, dq])

  return (
    <>
      <PageHeader
        title={t('nav.memories')}
        sub={data ? t('j.days_written', { n: data.length }) : ''}
        actions={<Link className="btn primary" to="/memory/new"><Icon name="pen" size={15} />{t('nav.new_memory')}</Link>}
        kanji="残"
      />
      {dq && (
        <div className="toolbar">
          <div className="search grow"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder={t('j.search')} aria-label={t('j.search')} /></div>
        </div>
      )}
      {loading && !data && <Loading />}
      {error && <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>}
      {data && data.length === 0 && (
        <Card>
          <Empty icon="archive" title={t('memories.empty')}>
            <p>{t('memories.intro')}</p>
            <Link className="btn primary" to="/memory/new">{t('memories.new')}</Link>
          </Empty>
        </Card>
      )}
      {data && data.length > 0 && list.length === 0 && (
        <Card><Empty icon="search" title={t('j.no_match_title')} /></Card>
      )}
      <div className="memgrid">
        {list.map((m) => {
          const art = memoryArt(m)
          const pal = art?.palette
          const bg = pal ? `linear-gradient(160deg, ${pal[1] ?? '#eee'} 0%, ${pal[0] ?? '#ddd'} 55%, ${pal[2] ?? '#ccc'} 100%)` : 'linear-gradient(160deg,#e8e4dc,#cfcbc2)'
          return (
            <button key={m.id} type="button" className="mcard" onClick={() => navigate(`/memory/${m.id}`)} title={t('memories.open')}>
              <div className="mcard-cover" style={{ background: bg }}>
                {m.poster_path ? (
                  <img src={assetUrl(m.id, m.poster_path)} alt="" loading="lazy" />
                ) : (
                  <span className="mcard-seal" aria-hidden>残</span>
                )}
                {m.photos ? <span className="mcard-count"><Icon name="image" size={12} />{m.photos}</span> : null}
              </div>
              <div className="mcard-body">
                <b>{m.title || m.date}</b>
                <span className="muted small">{m.date.slice(8, 10)} {monthName(Number(m.date.slice(5, 7)), false)} · {weekday(m.date)}</span>
                <div className="row" style={{ gap: 6, marginTop: 6 }}>
                  <PaletteDots m={m} />
                  {m.track_path && <Icon name="music" size={12} className="muted" />}
                  {m.status === 'ready' && <span className="chip sm">{t('memories.ready')}</span>}
                </div>
              </div>
            </button>
          )
        })}
      </div>
    </>
  )
}
