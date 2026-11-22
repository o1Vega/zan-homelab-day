// Post: one public day, opened large. Ukiyo-e print mode, four reactive
// gestures (sakura rain, kintsugi seam, ma breath, hanabi burst), and a
// haiku box where visitors leave three lines.
import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { addComment, deleteComment, followPerson, getPerson, getPost, getPostTheme, postAssetUrl, setFeatured, toggleReaction, unfollowPerson, type PostTheme } from '../lib/mem-api'
import { memoryTrack, trackLicenseLabel, type PublicPost, type SocialComment } from '../lib/types'
import { useI18n } from '../lib/i18n'
import { daysBetween, todayISO } from '../lib/format'
import { useReducedMotion } from '../lib/fx'
import { Empty, Loading, errMsg, useAsync, useToast } from '../components/ui'
import { RX_GLYPH, RX_KINDS, ageClass } from '../components/PostCard'

const SAKURA = Array.from({ length: 14 }, (_, i) => ({
  pl: ((i * 137) % 90) / 100,
  pd: 2.4 + ((i * 89) % 12) / 10,
  ph: 300 + ((i * 53) % 160),
  px: (i % 2 ? 1 : -1) * (20 + ((i * 71) % 60)),
}))

const HANABI_COLORS = ['#ffd76a', '#ff9d5c', '#f7a8c4', '#aab4e6', '#9fe8c1']
const HANABI = Array.from({ length: 18 }, (_, i) => {
  const a = (i / 18) * Math.PI * 2
  const r = 46 + ((i * 31) % 40)
  return {
    hx: Math.round(Math.cos(a) * r) + 'px',
    hy: Math.round(Math.sin(a) * r) - 30 + 'px',
    hl: ((i * 17) % 40) / 100,
    sc: HANABI_COLORS[i % HANABI_COLORS.length],
  }
})

// Ambient weather the day's theme may bring: rain, snow, embers, stars.
const RAIN = Array.from({ length: 22 }, (_, i) => ({
  rx: (i * 47) % 100,
  rd: 0.8 + ((i * 37) % 60) / 100,
  rl: ((i * 29) % 40) / 100,
  rh: 40 + ((i * 53) % 60),
}))
const SNOW = Array.from({ length: 24 }, (_, i) => ({
  sx: (i * 41) % 100,
  sd: 5 + ((i * 43) % 50) / 10,
  sl: ((i * 61) % 60) / 100,
  ss: 3 + (i % 4) * 1.5,
  spx: (i % 2 ? 1 : -1) * (10 + ((i * 23) % 30)),
}))
const EMBERS = Array.from({ length: 18 }, (_, i) => ({
  ex: (i * 59) % 100,
  ed: 3 + ((i * 47) % 40) / 10,
  el: ((i * 31) % 50) / 100,
  es: 2 + (i % 3),
  epx: (i % 2 ? 1 : -1) * (8 + ((i * 17) % 24)),
}))
const STARS = Array.from({ length: 26 }, (_, i) => ({
  tx: (i * 67) % 100,
  ty: (i * 37) % 70,
  ts: 1 + (i % 3) * 1.2,
  td: (((i * 43) % 60) / 100) * 2,
  tdl: 1.6 + ((i * 29) % 30) / 100,
}))

export default function Post() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { toast } = useToast()
  const reduced = useReducedMotion()
  const { id } = useParams()
  const num = Number(id)
  const { data, error, reload } = useAsync(() => getPost(num), [num])
  const [theme, setTheme] = useState<PostTheme | null>(null)

  const [mine, setMine] = useState<string[]>([])
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [comments, setComments] = useState<SocialComment[]>([])
  const [print, setPrint] = useState(false)
  const [burst, setBurst] = useState<'' | 'sakura' | 'hanabi'>('')
  const [flash, setFlash] = useState(false)
  const [body, setBody] = useState('')
  const [busy, setBusy] = useState('')
  const [unmuted, setUnmuted] = useState(false)
  const videoRef = useRef<HTMLVideoElement>(null)

  useEffect(() => {
    let on = true
    setUnmuted(false)
    getPostTheme(num).then((r) => { if (on) setTheme(r.theme) }).catch(() => {})
    return () => { on = false }
  }, [num])

  // The LLM's page background: it lives on <body> so it covers the whole
  // scroll area, and is restored when we leave the post.
  useEffect(() => {
    const bg = theme?.background
    if (!bg) return
    const prev = document.body.style.background
    document.body.style.background = bg
    return () => { document.body.style.background = prev }
  }, [theme?.background])
  const burstT = useRef<ReturnType<typeof setTimeout> | null>(null)
  const flashT = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (data) {
      setMine(data.reactions.mine)
      setCounts(data.reactions.counts)
      setComments(data.comments)
    }
  }, [data])

  useEffect(() => () => {
    if (burstT.current) clearTimeout(burstT.current)
    if (flashT.current) clearTimeout(flashT.current)
  }, [])

  const doBurst = (kind: 'sakura' | 'hanabi') => {
    if (reduced) return
    if (burstT.current) clearTimeout(burstT.current)
    setBurst(kind)
    burstT.current = setTimeout(() => setBurst(''), 1800)
  }

  const doFlash = () => {
    if (reduced) return
    if (flashT.current) clearTimeout(flashT.current)
    setFlash(true)
    flashT.current = setTimeout(() => setFlash(false), 1600)
  }

  if (!data) {
    return error ? <Empty icon="globe" title={t('post.not_found')} /> : <Loading />
  }

  const post = data.post
  const days = daysBetween(post.date, todayISO())

  const press = (kind: string) => {
    if (busy) return
    setBusy(kind)
    const had = mine.includes(kind)
    setMine((m) => (had ? m.filter((x) => x !== kind) : [...m, kind]))
    toggleReaction(num, kind)
      .then((res) => {
        setCounts((c) => ({ ...c, [kind]: res.count }))
        setMine((m) => (res.active ? (m.includes(kind) ? m : [...m, kind]) : m.filter((x) => x !== kind)))
        if (res.active) {
          if (kind === 'sakura') doBurst('sakura')
          else if (kind === 'hanabi') doBurst('hanabi')
          else if (kind === 'kintsugi') doFlash()
        }
      })
      .catch((err) => {
        setMine((m) => (had ? [...m, kind] : m.filter((x) => x !== kind)))
        toast(errMsg(err))
        reload()
      })
      .finally(() => setBusy(''))
  }

  const submit = () => {
    const text = body.trim()
    if (!text || busy) return
    setBusy('haiku')
    addComment(num, text)
      .then((c) => {
        setComments((cs) => [c, ...cs])
        setBody('')
      })
      .catch((err) => toast(errMsg(err)))
      .finally(() => setBusy(''))
  }

  const del = (c: SocialComment) => {
    deleteComment(num, c.id)
      .then(() => setComments((cs) => cs.filter((x) => x.id !== c.id)))
      .catch((err) => toast(errMsg(err)))
  }

  const lines = body.split('\n').slice(0, 3)
  const syl = lines.map((l) => l.toLowerCase().match(/[aeiouyаеёиоуыэюя]+/gi)?.length ?? 0)
  const targets = [5, 7, 5]

  const amb = theme && !reduced && theme.effect !== 'none' ? theme.effect : ''
  const ambStyle = { '--ain': String(theme?.intensity ?? 0) } as CSSProperties
  const wrapStyle = theme?.palette?.length
    ? ({ '--pa': theme.palette[0] } as CSSProperties)
    : undefined

  return (
    <div className={`post-wrap ${ageClass(post.date)}`} style={wrapStyle}>
      <div className={`post-cover${print ? ' print-mode' : ''}${mine.includes('ma') ? ' ma-active' : ''}`}>
        {post.poster_path ? (
          <img className="poster" src={postAssetUrl(num, post.poster_path)} alt="" />
        ) : (
          <span className="seal" aria-hidden>残</span>
        )}
        <span className="print-grain" />
        <span className="print-title">{post.title || post.date}</span>
        <span className="print-seal">{theme?.seal || '浮'}</span>
        {amb === 'rain' && (
          <span className="rain-layer" style={ambStyle}>
            {RAIN.map((p, i) => (
              <span key={i} className="drop" style={{ '--rx': `${p.rx}%`, '--rd': `${p.rd}s`, '--rl': `${p.rl}s`, '--rh': `${p.rh}px` } as CSSProperties} />
            ))}
          </span>
        )}
        {amb === 'snow' && (
          <span className="snow-layer" style={ambStyle}>
            {SNOW.map((p, i) => (
              <span key={i} className="flake" style={{ '--sx': `${p.sx}%`, '--sd': `${p.sd}s`, '--sl': `${p.sl}s`, '--ss': `${p.ss}px`, '--spx': `${p.spx}px` } as CSSProperties} />
            ))}
          </span>
        )}
        {amb === 'embers' && (
          <span className="embers-layer" style={ambStyle}>
            {EMBERS.map((p, i) => (
              <span key={i} className="ember" style={{ '--ex': `${p.ex}%`, '--ed': `${p.ed}s`, '--el': `${p.el}s`, '--es': `${p.es}px`, '--epx': `${p.epx}px` } as CSSProperties} />
            ))}
          </span>
        )}
        {amb === 'stars' && (
          <span className="stars-layer" style={ambStyle}>
            {STARS.map((p, i) => (
              <span key={i} className="star" style={{ '--tx': `${p.tx}%`, '--ty': `${p.ty}%`, '--ts': `${p.ts}px`, '--td': `${p.td}s`, '--tdl': `${p.tdl}s` } as CSSProperties} />
            ))}
          </span>
        )}
        {burst === 'sakura' && !reduced && (
          <span className="sakura-layer">
            {SAKURA.map((p, i) => (
              <span key={i} className="petal" style={{ '--pl': `${p.pl}s`, '--pd': `${p.pd}s`, '--ph': `${p.ph}px`, '--px': `${p.px}px` } as CSSProperties} />
            ))}
          </span>
        )}
        {burst === 'hanabi' && !reduced && (
          <span className="hanabi-layer">
            {HANABI.map((p, i) => (
              <span key={i} className="spark" style={{ '--hx': p.hx, '--hy': p.hy, '--hl': `${p.hl}s`, '--sc': p.sc } as CSSProperties} />
            ))}
          </span>
        )}
        {(flash || mine.includes('kintsugi')) && !reduced && (
          <span className={`kintsugi-layer${flash ? '' : ' kintsugi-still'}`}>
            <svg className="kintsugi-crack" viewBox="0 0 400 225" preserveAspectRatio="none" aria-hidden="true">
              <path d="M-5 60 L70 55 L140 95 L215 88 L290 130 L360 122 L405 150" />
              <path d="M140 95 L150 160 L120 230" style={{ '--kd': '0.5s' } as CSSProperties} />
            </svg>
          </span>
        )}
        <span className="ma-layer" />
      </div>

      <div className="post-head">
        <h2 className="post-title">{post.title || post.date}</h2>
        <PostActions post={post} />
      </div>
      <div className="post-byline">
        <button type="button" className="mcard-author" onClick={() => navigate(`/people/${post.author_id}`)}>{post.author}</button>
        <span>{t('post.by')} · {post.date}</span>
        {days > 0 && <span className="age-tag">{t('post.age', { days })}</span>}
        {post.is_mine && <span className="post-badge">{t('post.yours')}</span>}
      </div>

      {theme?.tagline && <div className="post-tagline">{theme.tagline}</div>}

      {post.video_path && (
        <div className="post-video">
          <video ref={videoRef} src={postAssetUrl(num, post.video_path)} controls autoPlay muted loop playsInline />
          {!unmuted && (
            <button
              type="button"
              className="btn sm post-video-unmute"
              onClick={() => {
                if (videoRef.current) videoRef.current.muted = false
                setUnmuted(true)
              }}
            >
              {t('post.unmute')}
            </button>
          )}
        </div>
      )}
      {!post.video_path && post.track_path && (
        <div className="post-video">
          <audio controls autoPlay loop src={postAssetUrl(num, post.track_path)} className="post-track" />
          {(() => {
            const tm = memoryTrack(post)
            if (!tm?.attribution && !tm?.license) return null
            return (
              <div className="post-track-attr">
                <span>{tm.attribution || [tm.title, tm.artist].filter(Boolean).join(' — ')}</span>
                {tm.license_url && <a href={tm.license_url} target="_blank" rel="noreferrer">{trackLicenseLabel(tm)}</a>}
              </div>
            )
          })()}
        </div>
      )}

      <div className="post-actions">
        <button type="button" className={`btn${print ? ' primary' : ''}`} onClick={() => setPrint((p) => !p)}>{t('print.label')}</button>
      </div>

      <div className="rx-bar">
        {RX_KINDS.map((k) => (
          <button key={k} type="button" className={`rx-chip${mine.includes(k) ? ' on-' + k : ''}`} onClick={() => press(k)} disabled={!!busy}>
            <span className="rx-glyph">{RX_GLYPH[k]}</span>
            {t(`rx.${k}`)}
            <span className="rx-n">{counts[k] ?? 0}</span>
          </button>
        ))}
        <p className="rx-hint">{t('rx.hint')}</p>
      </div>

      {post.text && <div className="post-text">{post.text}</div>}

      <section className="haiku-sec">
        <div className="haiku-head">
          <h3>{t('haiku.title')}</h3>
          <span className="haiku-intro">{t('haiku.intro')}</span>
        </div>
        <div className="haiku-list">
          {comments.length === 0 && (
            <div className="haiku-item">
              <span className="haiku-seal">静</span>
              <div className="haiku-body">{t('haiku.empty')}</div>
            </div>
          )}
          {comments.map((c) => (
            <div key={c.id} className="haiku-item">
              <span className="haiku-seal">{c.author.slice(0, 1).toUpperCase()}</span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="haiku-body">{c.body}</div>
                <div className="haiku-who">
                  {c.author}
                  {c.mine && (
                    <button type="button" className="haiku-del" title={t('haiku.delete')} onClick={() => del(c)}>{t('haiku.delete')}</button>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
        <div className="haiku-compose">
          <textarea rows={3} maxLength={200} value={body} placeholder={t('haiku.intro')} onChange={e => setBody(e.target.value)} />
          <div className="haiku-foot">
            <span className="haiku-lines">
              {syl.map((n, i) => (
                <span key={i} className={`haiku-line${n === targets[i] ? ' ok' : n > targets[i] ? ' over' : ''}`}>{n}/{targets[i]}</span>
              ))}
            </span>
            <span className="haiku-free">{t('haiku.free')}</span>
            <span className="haiku-count">{body.length}/200</span>
            <button type="button" className="btn" disabled={!body.trim() || !!busy} onClick={submit}>{t('haiku.submit')}</button>
          </div>
        </div>
      </section>
    </div>
  )
}

function PostActions({ post }: { post: PublicPost }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data } = useAsync(() => getPerson(post.author_id, 1), [post.author_id])
  const [follow, setFollow] = useState(false)
  const [fbusy, setFbusy] = useState(false)
  const [featuredId, setFeaturedId] = useState<number | null>(null)

  useEffect(() => {
    if (data) {
      setFollow(data.person.i_follow)
      setFeaturedId(data.featured?.id ?? null)
    }
  }, [data])

  const flip = (target: boolean) => {
    if (fbusy || post.is_mine) return
    setFbusy(true)
    setFollow(target)
    const p = target ? followPerson(post.author_id) : unfollowPerson(post.author_id)
    p.catch((err) => {
      setFollow(!target)
      toast(errMsg(err))
    }).finally(() => setFbusy(false))
  }

  const flower = () => {
    const on = featuredId !== post.id
    setFeaturedId(on ? post.id : null)
    setFeatured(post.id, on)
      .then(() => toast(on ? t('tokonoma.set') : t('tokonoma.unset')))
      .catch((err) => {
        setFeaturedId(on ? null : post.id)
        toast(errMsg(err))
      })
  }

  return (
    <span className="row">
      {!post.is_mine && (
        <button type="button" className={`btn${follow ? ' primary' : ''}`} disabled={fbusy} onClick={() => flip(!follow)}>
          {follow ? t('social.unfollow') : t('social.follow')}
        </button>
      )}
      {post.is_mine && (
        <>
          <button type="button" className={`btn${featuredId === post.id ? ' primary' : ''}`} onClick={flower}>
            {featuredId === post.id ? t('tokonoma.unset') : t('tokonoma.set')}
          </button>
          <button type="button" className="btn" onClick={() => navigate(`/memory/${post.id}`)}>{t('post.open_memory')}</button>
        </>
      )}
    </span>
  )
}
