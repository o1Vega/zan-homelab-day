// One day, developed: the poster canvas, the track, the narration, the
// "why the day looks this way" and the day's folder actions.
import { useCallback, useEffect, useRef, useState, type CSSProperties } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { assetUrl, background, deleteMemory, findTrack, getMemory, getMemoryTheme, renderNarration, renderVideo, savePoster, setMemoryPublic, type MemoryDetail, type PostTheme } from '../lib/mem-api'
import { narrate } from '../lib/audio'
import { posterBlob, renderPoster, type PosterData } from '../components/Poster'
import { memoryArt, memoryTrack, trackLicenseLabel } from '../lib/types'
import { useI18n } from '../lib/i18n'
import { useApp } from '../lib/app-context'
import { Card, Empty, Loading, PageHeader, useToast } from '../components/ui'
import { Icon } from '../components/icons'

export default function MemoryPage() {
  const { id } = useParams()
  const num = Number(id)
  const { t } = useI18n()
  const { settings } = useApp()
  const { toast } = useToast()
  const navigate = useNavigate()
  const [detail, setDetail] = useState<MemoryDetail | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<'' | 'bg' | 'poster' | 'delete' | 'public' | 'track' | 'video' | 'video_ai' | 'narration'>('')
  const [speaking, setSpeaking] = useState(false)
  const stopRef = useRef<{ cancel: () => void } | null>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [posterData, setPosterData] = useState<PosterData | null>(null)
  const [theme, setTheme] = useState<PostTheme | null>(null)
  const [opening, setOpening] = useState('')

  const load = useCallback(async () => {
    try {
      const d = await getMemory(num)
      setDetail(d)
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'load failed')
    }
  }, [num])
  useEffect(() => { void load() }, [load])

  // The LLM theme of the day: its colors, story and opening animation. It is
  // generated in the background when the day is created, so by the first open
  // it is usually just a fast cache read and the page breathes in its own way.
  useEffect(() => {
    let on = true
    const mount = Date.now()
    setTheme(null)
    setOpening('')
    getMemoryTheme(num)
      .then((r) => {
        if (!on) return
        setTheme(r.theme)
        // a theme that arrives with the page plays its opening; a late one
        // (first synchronous roll) only re-colors the day, it does not
        // re-animate a page the reader is already looking at
        if (Date.now() - mount < 1600) setOpening(r.theme.opening || 'fade')
      })
      .catch(() => {})
    return () => { on = false }
  }, [num])

  // The day's background lives on <body> so it covers the whole scroll area;
  // it is restored when the page unmounts.
  useEffect(() => {
    const bg = theme?.background
    if (!bg) return
    const prev = document.body.style.background
    document.body.style.background = bg
    return () => { document.body.style.background = prev }
  }, [theme?.background])

  const m = detail?.memory
  const art = m ? memoryArt(m) : null
  const track = m ? memoryTrack(m) : null
  const ai = !!settings?.ai_configured

  // Compose the poster whenever the direction or assets change.
  useEffect(() => {
    if (!m || !art || !detail) return
    const data: PosterData = {
      date: m.date,
      title: art.title || m.title,
      caption: art.caption,
      palette: art.palette,
      technique: art.technique,
      grain: art.motion?.grain ?? 0.3,
      photos: detail.photos.slice(0, 4).map((p) => ({ src: assetUrl(m.id, p.path) })),
      // a missing background 404s and the renderer falls back to the procedural one
      background: assetUrl(m.id, 'background.png'),
      track: track ? { title: track.title, artist: track.artist } : null,
    }
    setPosterData(data)
    const c = canvasRef.current
    if (c) {
      // The background may not be there yet; render anyway, it re-renders on reload.
      renderPoster(c, data).catch(() => {})
    }
  }, [m, art, detail, track]) // eslint-disable-line

  async function developBg(force: boolean) {
    if (!m || busy) return
    setBusy('bg')
    try {
      await background(m.id, force)
      // re-render the poster with the new background
      setDetail(null)
      await load()
    } catch (e) {
      toast(e instanceof Error ? e.message : 'background failed', 'err')
    } finally {
      setBusy('')
    }
  }

  async function doSavePoster() {
    if (!m || !posterData || busy) return
    setBusy('poster')
    try {
      const blob = await posterBlob(posterData)
      await savePoster(m.id, blob)
      toast(t('memories.save_poster'), 'ok')
    } catch (e) {
      toast(e instanceof Error ? e.message : 'save failed', 'err')
    } finally {
      setBusy('')
    }
  }

  function downloadPoster() {
    if (!posterData) return
    posterBlob(posterData).then((blob) => {
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${m?.date || 'day'}.png`
      a.click()
      URL.revokeObjectURL(url)
    })
  }

  function narrateDay() {
    if (!m || !art) return
    if (speaking) {
      stopRef.current?.cancel()
      setSpeaking(false)
      return
    }
    stopRef.current = narrate(m.text, art.voice)
    setSpeaking(true)
    const onEnd = () => setSpeaking(false)
    window.speechSynthesis?.addEventListener?.('end', onEnd, { once: true })
  }

  async function doDelete() {
    if (!m || busy) return
    setBusy('delete')
    try {
      await deleteMemory(m.id)
      toast(t('memories.deleted'), 'ok')
      navigate('/memories')
    } catch (e) {
      toast(e instanceof Error ? e.message : 'delete failed', 'err')
      setBusy('')
    }
  }

  async function doPublic() {
    if (!m || busy) return
    setBusy('public')
    try {
      const res = await setMemoryPublic(m.id, !m.is_public)
      setDetail((d) => (d ? { ...d, memory: { ...d.memory, is_public: res.is_public } } : d))
      toast(res.is_public ? t('memories.public_on') : t('memories.public_off'), 'ok')
    } catch (e) {
      toast(e instanceof Error ? e.message : 'public failed', 'err')
    } finally {
      setBusy('')
    }
  }

  async function doVideo(force: boolean, engine?: 'ai') {
    if (!m || busy) return
    setBusy(engine ? 'video_ai' : 'video')
    try {
      const res = await renderVideo(m.id, force, engine)
      setDetail((d) => (d ? { ...d, memory: { ...d.memory, video_path: res.file } } : d))
      toast(t('memories.video_done'), 'ok')
    } catch (e) {
      toast(e instanceof Error ? e.message : 'video failed', 'err')
    } finally {
      setBusy('')
    }
  }

  async function doNarrateFile(force: boolean) {
    if (!m || busy) return
    setBusy('narration')
    try {
      const res = await renderNarration(m.id, force)
      setDetail((d) => (d ? { ...d, memory: { ...d.memory, narration_path: res.file } } : d))
      toast(t('memories.narration_done'), 'ok')
    } catch (e) {
      toast(e instanceof Error ? e.message : 'narration failed', 'err')
    } finally {
      setBusy('')
    }
  }

  /** Find (or re-roll) an open-licensed track for the day. */
  async function doFindTrack(force: boolean) {
    if (!m || busy) return
    setBusy('track')
    try {
      const res = await findTrack(m.id, force)
      setDetail((d) => (d ? { ...d, memory: { ...d.memory, track_path: res.track.file, track_meta: res.track.meta } } : d))
      toast(t('memories.track_done'), 'ok')
    } catch (e) {
      toast(e instanceof Error ? e.message : 'track search failed', 'err')
    } finally {
      setBusy('')
    }
  }

  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!m) return <Loading />

  return (
    <div className={`mem-open${opening ? ` open-${opening}` : ''}`} style={theme?.palette?.length ? ({ '--pa': theme.palette[0] } as CSSProperties) : undefined}>
      <PageHeader
        kanji={theme?.seal || '残'}
        title={art?.title || m.title || m.date}
        sub={
          <span className="row" style={{ gap: 8 }}>
            {m.date}
            {m.mood_cluster && <span className="chip sm">{t(`memories.cluster.${m.mood_cluster}`)}</span>}
            <span className="chip sm" title={m.status}>{m.status === 'ready' ? t('memories.ready') : t('memories.draft')}</span>
          </span>
        }
        actions={
          <span className="row" style={{ gap: 8 }}>
            <Link className="btn sm" to={`/memories`}><Icon name="archive" size={14} />{t('nav.memories')}</Link>
            <button type="button" key={String(m.is_public)} className={`seal-btn${m.is_public ? ' on' : ''}`} onClick={doPublic} disabled={busy === 'public'}><span className="seal-ico seal-stamp" aria-hidden>残</span>{m.is_public ? t('memories.make_private') : t('memories.make_public')}</button>
            <button type="button" className="btn sm danger" onClick={doDelete} disabled={busy === 'delete'}><Icon name="trash" size={14} />{t('memories.delete')}</button>
          </span>
        }
      />

      <div className="memday">
        <Card className="memday-poster">
          <div className="memday-canvas-wrap">
            <canvas ref={canvasRef} aria-label={t('memories.poster')} />
          </div>
          <div className="row wrap mt" style={{ gap: 8 }}>
            <button type="button" className="btn" onClick={() => developBg(false)} disabled={busy !== '' || !ai}>
              <Icon name="palette" size={15} />{busy === 'bg' ? t('memories.analyzing') : t('memories.background')}
            </button>
            {(m.poster_path) && <button type="button" className="btn" onClick={() => developBg(true)} disabled={busy !== ''}><Icon name="refresh" size={14} />{t('memories.background_reroll')}</button>}
            <span className="grow" />
            <button type="button" className="btn" onClick={downloadPoster} disabled={!posterData}><Icon name="download" size={15} />{t('memories.download')}</button>
            <button type="button" className="btn primary" onClick={doSavePoster} disabled={busy !== '' || !posterData}><Icon name="check" size={15} />{t('memories.save_poster')}</button>
          </div>
          {m.video_path ? (
            <div className="mt" style={{ borderTop: '1px solid var(--border)', paddingTop: 12 }}>
              <div className="row" style={{ gap: 8, marginBottom: 6 }}><Icon name="film" size={14} /><b>{t('memories.video')}</b>
                <span className="grow" />
                {m.video_path !== 'day-ai.mp4' && (
                  <button type="button" className="btn sm" onClick={() => doVideo(false, 'ai')} disabled={busy !== ''}>
                    <Icon name="sparkles" size={13} />{busy === 'video_ai' ? t('memories.video_ai_busy') : t('memories.video_ai')}
                  </button>
                )}
                <button type="button" className="btn sm" onClick={() => doVideo(true, m.video_path === 'day-ai.mp4' ? 'ai' : undefined)} disabled={busy !== ''}><Icon name="refresh" size={13} />{t('memories.video_reroll')}</button>
              </div>
              <video controls src={assetUrl(m.id, m.video_path)} style={{ width: '100%', borderRadius: 8, background: '#000' }} />
            </div>
          ) : (
            <div className="mt row" style={{ borderTop: '1px solid var(--border)', paddingTop: 12, gap: 8 }}>
              <Icon name="film" size={14} /><span className="muted small grow">{t('memories.video_hint')}</span>
              <button type="button" className="btn sm" onClick={() => doVideo(false, 'ai')} disabled={busy !== ''}>
                <Icon name="sparkles" size={13} />{busy === 'video_ai' ? t('memories.video_ai_busy') : t('memories.video_ai')}
              </button>
              <button type="button" className="btn sm" onClick={() => doVideo(false)} disabled={busy !== '' || !posterData && detail.photos.length === 0}>
                {busy === 'video' ? t('memories.analyzing') : t('memories.video_make')}
              </button>
            </div>
          )}
          {!ai && <div className="callout mt"><Icon name="info" size={14} /><span>AI is not connected — the background is procedural. Connect a model with a Text-to-Image for the painted texture.</span></div>}
        </Card>

        <div className="memday-side">
          {theme?.tagline && <div className="memday-tagline">{theme.tagline}</div>}
          <Card>
            <div className="row" style={{ gap: 8, marginBottom: 8 }}><Icon name="pen" size={15} /><b>{t('memories.write_ph').split('…')[0]}</b></div>
            <div className="memday-text">{m.text}</div>
            {theme?.story && (
              <div className="mt" style={{ borderTop: '1px solid var(--border)', paddingTop: 12 }}>
                <div className="memday-story-head"><span className="memday-story-seal" aria-hidden>{theme.seal || '残'}</span><b>{t('memories.story')}</b></div>
                <div className="memday-story">{theme.story}</div>
              </div>
            )}
            {track && (
              <div className="mt" style={{ borderTop: '1px solid var(--border)', paddingTop: 12 }}>
                <div className="row wrap" style={{ gap: 8, marginBottom: 6 }}>
                  <Icon name="music" size={14} /><b>{track.title || t('memories.track_unknown')}{track.artist ? ` — ${track.artist}` : ''}</b>
                  {trackLicenseLabel(track) && <span className="chip sm" title={track.license_url}>{trackLicenseLabel(track)}</span>}
                  <span className="grow" />
                  {track.source === 'openverse' && m.track_path && (
                    <button type="button" className="btn sm" onClick={() => doFindTrack(true)} disabled={busy !== ''}>
                      <Icon name="refresh" size={13} />{busy === 'track' ? t('memories.track_busy') : t('memories.track_reroll')}
                    </button>
                  )}
                </div>
                {m.track_path && <audio controls src={assetUrl(m.id, m.track_path)} style={{ width: '100%' }} />}
                {track.url && <a className="small" href={track.url} target="_blank" rel="noreferrer" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><Icon name="link" size={13} />{t('memories.listen_on_service')}</a>}
                {track.attribution && (
                  <div className="track-attr">
                    <span>{track.attribution}</span>
                    {track.license_url && <a href={track.license_url} target="_blank" rel="noreferrer">{trackLicenseLabel(track)}</a>}
                  </div>
                )}
              </div>
            )}
            {!track && !m.track_url && (
              <div className="mt row wrap" style={{ borderTop: '1px solid var(--border)', paddingTop: 12, gap: 8 }}>
                <Icon name="music" size={14} /><span className="muted small grow">{t('memories.track_hint_find')}</span>
                <button type="button" className="btn sm" onClick={() => doFindTrack(false)} disabled={busy !== ''}>
                  <Icon name="search" size={13} />{busy === 'track' ? t('memories.track_busy') : t('memories.track_find')}
                </button>
              </div>
            )}
            {!track && m.track_url && (
              <div className="mt" style={{ borderTop: '1px solid var(--border)', paddingTop: 12 }}>
                <a className="row" href={m.track_url} target="_blank" rel="noreferrer" style={{ gap: 8 }}><Icon name="music" size={14} />{t('memories.listen_on_service')}</a>
              </div>
            )}
            {m.narration_path && (
              <div className="mt" style={{ borderTop: '1px solid var(--border)', paddingTop: 12 }}>
                <div className="row" style={{ gap: 8, marginBottom: 6 }}><Icon name="mic" size={14} /><b>{t('memories.narration_file')}</b></div>
                <audio controls src={assetUrl(m.id, m.narration_path)} style={{ width: '100%' }} />
              </div>
            )}
            <div className="row mt wrap" style={{ gap: 8 }}>
              <button type="button" className="btn" onClick={narrateDay} disabled={!m.text || !art}>
                <Icon name={speaking ? 'stop' : 'wave'} size={14} />{speaking ? t('memories.stop') : t('memories.narrate')}
              </button>
              <button type="button" className="btn" onClick={() => doNarrateFile(false)} disabled={busy !== '' || !m.text || !ai} title={t('memories.narration_save_hint')}>
                <Icon name="mic" size={14} />{busy === 'narration' ? t('memories.analyzing') : t('memories.narration_save')}
              </button>
            </div>
          </Card>

          {art && (
            <Card>
              <div className="row" style={{ gap: 8, marginBottom: 10 }}><Icon name="sparkles" size={15} /><b>{t('memories.why')}</b></div>
              <div className="row wrap" style={{ gap: 6, marginBottom: 12 }}>
                {art.palette.map((c, i) => <span key={i} className="swatch" style={{ background: c }} title={c} />)}
              </div>
              {(art.reasons || []).length > 0 && (
                <ul className="memday-reasons">
                  {art.reasons.map((r, i) => <li key={i}>{r}</li>)}
                </ul>
              )}
              <div className="row small muted mt" style={{ gap: 10 }}>
                {art.technique && <span className="chip sm">{art.technique}</span>}
                {art.motion && <span className="chip sm">{art.motion.frame_sec}s / frame</span>}
              </div>
            </Card>
          )}

          {detail.photos.length > 0 && (
            <Card>
              <div className="row" style={{ gap: 8, marginBottom: 10 }}><Icon name="image" size={15} /><b>{t('memories.photos')}</b><span className="muted small">{t('memories.photos_n', { n: detail.photos.length })}</span></div>
              <div className="memday-grid">
                {detail.photos.map((p) => <img key={p.id} src={assetUrl(m.id, p.path)} alt="" loading="lazy" />)}
              </div>
            </Card>
          )}
        </div>
      </div>

      {detail.photos.length === 0 && !art && (
        <Card>
          <Empty icon="sparkles" title={t('memories.no_text')}>
            <p>{t('memories.intro')}</p>
          </Empty>
        </Card>
      )}
    </div>
  )
}
