// Write a day: the text (or dictation), the photos, the song. Then "Develop
// the day" turns them into a remembrance and opens the day's page.
import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { analyze, createMemory, setTrackLink, uploadPhotos, uploadTrack } from '../lib/mem-api'
import { analyzeTrack, photoColors as samplePhotoColors, type TrackFeatures } from '../lib/audio'
import { todayISO } from '../lib/format'
import { useI18n } from '../lib/i18n'
import { useApp } from '../lib/app-context'
import { V1 } from '../lib/api'
import { Card, Loading, PageHeader, useToast } from '../components/ui'
import { Icon } from '../components/icons'

export default function NewMemory() {
  const { t } = useI18n()
  const { settings } = useApp()
  const { toast } = useToast()
  const navigate = useNavigate()
  const [sp] = useSearchParams()
  const [date, setDate] = useState(() => {
    const d = sp.get('date')
    return d && /^\d{4}-\d{2}-\d{2}$/.test(d) && d <= todayISO() ? d : todayISO()
  })
  const [text, setText] = useState('')
  const [photos, setPhotos] = useState<{ file: File; url: string; colors: number[][] }[]>([])
  const [photoColors, setPhotoColors] = useState<number[][][]>([])
  const [track, setTrack] = useState<{ file: File; url: string; feat: TrackFeatures } | null>(null)
  const [trackTitle, setTrackTitle] = useState('')
  const [trackArtist, setTrackArtist] = useState('')
  const [linkMode, setLinkMode] = useState(false)
  const [linkUrl, setLinkUrl] = useState('')
  const [busy, setBusy] = useState<'' | 'upload' | 'sound' | 'develop'>('')
  const [savedId, setSavedId] = useState<number | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const trackRef = useRef<HTMLInputElement>(null)
  const [recording, setRecording] = useState(false)
  const recRef = useRef<MediaRecorder | null>(null)

  useEffect(() => () => { photos.forEach(p => URL.revokeObjectURL(p.url)); if (track) URL.revokeObjectURL(track.url) }, []) // eslint-disable-line

  // ---- dictation: record the voice, transcribe through the day's dictation route ----
  async function startRecording() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      const rec = new MediaRecorder(stream)
      const chunks: Blob[] = []
      rec.ondataavailable = (e) => { if (e.data.size) chunks.push(e.data) }
      rec.onstop = async () => {
        stream.getTracks().forEach((tr) => tr.stop())
        setRecording(false)
        const blob = new Blob(chunks, { type: rec.mimeType || 'audio/webm' })
        try {
          // the dictation route reads a file field
          const fd = new FormData()
          fd.append('file', blob, 'dictation.webm')
          const res = await fetch(`${V1}/ai/transcribe`, { method: 'POST', body: fd, credentials: 'same-origin' })
          const j = await res.json().catch(() => ({}))
          if (res.ok && j.text) setText((cur) => (cur ? cur + '\n' : '') + j.text)
          else toast(j.error || res.statusText, 'err')
        } catch (e) {
          toast(e instanceof Error ? e.message : 'dictation failed', 'err')
        }
      }
      recRef.current = rec
      rec.start()
      setRecording(true)
    } catch {
      toast('the microphone is not available', 'err')
    }
  }
  function stopRecording() { recRef.current?.stop() }

  // ---- photos ----
  async function addPhotos(list: FileList | null) {
    if (!list || !list.length) return
    setBusy('upload')
    try {
      const files = Array.from(list)
      const colors = await Promise.all(files.map((f) => samplePhotoColors(f)))
      setPhotos((cur) => [...cur, ...files.map((file, i) => ({ file, url: URL.createObjectURL(file), colors: colors[i] }))])
      setPhotoColors((cur) => [...cur, ...colors])
    } finally {
      setBusy('')
    }
  }

  // ---- track ----
  async function addTrack(f: File | null) {
    if (!f) return
    setBusy('sound')
    try {
      const feat = await analyzeTrack(f)
      setTrack({ file: f, url: URL.createObjectURL(f), feat })
      setTrackTitle(feat.key || '')
    } finally {
      setBusy('')
    }
  }

  // ---- develop: save the day, attach the files, analyze the atmosphere ----
  async function develop() {
    if (!text.trim() || busy) return
    setBusy('develop')
    try {
      let m = await createMemory(date, text)
      if (!savedId || savedId !== m.id) setSavedId(m.id)
      if (photos.length) {
        await uploadPhotos(m.id, photos.map((p) => p.file), photoColors)
      }
      if (track) {
        await uploadTrack(m.id, track.file, track.feat, trackTitle, trackArtist)
      } else if (linkMode && linkUrl.trim()) {
        await setTrackLink(m.id, { url: linkUrl.trim(), title: trackTitle, artist: trackArtist })
      }
      const dir = await analyze(m.id)
      toast(`${t('memories.developed')}: ${dir.title}`, 'ok')
      navigate(`/memory/${m.id}`)
    } catch (e) {
      toast(e instanceof Error ? e.message : 'develop failed', 'err')
      setBusy('')
    }
  }

  const ai = !!settings?.ai_configured
  return (
    <>
      <PageHeader
        title={t('nav.new_memory')}
        sub={t('memories.intro')}
        actions={<input type="date" value={date} max={todayISO()} onChange={e => setDate(e.target.value)} aria-label={t('memories.date')} />}
        kanji="新"
      />
      <div className="newmem">
        <Card>
          <div className="row" style={{ gap: 10, marginBottom: 8 }}>
            <b>{t('nav.new_memory')}</b>
            <span className="muted small">{date}</span>
          </div>
          <textarea
            className="grow-entry" rows={7} value={text}
            onChange={e => setText(e.target.value)}
            placeholder={t('memories.write_ph')} aria-label={t('memories.write_ph')}
          />
          <div className="row mt" style={{ gap: 8 }}>
            <button type="button" className="btn sm" onClick={recording ? stopRecording : startRecording} disabled={recording}>
              <Icon name="mic" size={14} />{recording ? t('memories.dictating') : t('memories.dictate')}
            </button>
            <span className="grow" />
            <button type="button" className="btn primary" onClick={develop} disabled={!text.trim() || !!busy} title={!text.trim() ? t('memories.no_text') : undefined}>
              <Icon name="sparkles" size={15} />{busy === 'develop' ? t('memories.analyzing_atmo') : t('memories.analyze')}
            </button>
          </div>
          {!ai && <div className="callout mt"><Icon name="info" size={14} /><span>AI is not connected yet — the day is developed by the fixed rules. Under Settings → AI, connect a model for a richer direction.</span></div>}
        </Card>

        <Card>
          <div className="row" style={{ gap: 8, marginBottom: 10 }}><Icon name="image" size={15} /><b>{t('memories.photos')}</b><span className="muted small">{t('memories.photos_hint')}</span></div>
          <div className="nm-thumbs">
            {photos.map((p, i) => (
              <div key={i} className="nm-thumb">
                <img src={p.url} alt="" />
                <button type="button" className="nm-x" onClick={() => { URL.revokeObjectURL(p.url); setPhotos(photos.filter((_, j) => j !== i)); setPhotoColors(photoColors.filter((_, j) => j !== i)) }} aria-label="×"><Icon name="x" size={13} /></button>
              </div>
            ))}
            <button type="button" className="nm-add" onClick={() => fileRef.current?.click()} disabled={busy === 'upload'}>
              <Icon name="plus" size={18} />{t('memories.add_photo')}
            </button>
          </div>
          <input ref={fileRef} type="file" accept="image/jpeg,image/png,image/webp,image/heic" multiple hidden onChange={e => { void addPhotos(e.target.files); e.target.value = '' }} />
        </Card>

        <Card>
          <div className="row wrap" style={{ gap: 8, marginBottom: 10 }}><Icon name="music" size={15} /><b>{t('memories.track')}</b><span className="muted small">{t('memories.track_hint')}</span>
            <span className="grow" />
            <button type="button" className="btn sm" aria-pressed={linkMode} onClick={() => { setLinkMode(v => !v); if (!linkMode) setTrack(null) }}><Icon name="link" size={13} />{linkMode ? t('memories.track_file_mode') : t('memories.track_link_mode')}</button>
          </div>
          {linkMode ? (
            <div className="stack">
              <input value={linkUrl} onChange={e => setLinkUrl(e.target.value)} placeholder={t('memories.track_link_ph')} aria-label={t('memories.track_link_ph')} />
              <div className="row" style={{ gap: 8 }}>
                <input className="grow" value={trackTitle} onChange={e => setTrackTitle(e.target.value)} placeholder={t('memories.track_title')} aria-label={t('memories.track_title')} />
                <input className="grow" value={trackArtist} onChange={e => setTrackArtist(e.target.value)} placeholder={t('memories.track_artist')} aria-label={t('memories.track_artist')} />
              </div>
              <p className="small muted">{t('memories.track_link_note')}</p>
            </div>
          ) : !track ? (
            <button type="button" className="btn" onClick={() => trackRef.current?.click()} disabled={busy === 'sound'}>
              <Icon name="music" size={15} />{busy === 'sound' ? t('memories.analyzing') : t('memories.add_track')}
            </button>
          ) : (
            <div className="nm-track">
              <audio controls src={track.url} style={{ width: '100%' }} />
              {busy !== 'sound' && track.feat.measured.length > 0 && (
                <div className="row wrap small muted" style={{ gap: 6, marginTop: 8 }}>
                  {track.feat.bpm > 0 && <span className="chip sm">{track.feat.bpm} {t('memories.bpm')}</span>}
                  {track.feat.key && <span className="chip sm">{t('memories.key')}: {track.feat.key}{track.feat.major === false ? ' min' : ''}</span>}
                  <span className="chip sm">⚡ {Math.round((track.feat.energy || 0) * 100)}%</span>
                </div>
              )}
              <div className="row mt" style={{ gap: 8 }}>
                <input className="grow" value={trackTitle} onChange={e => setTrackTitle(e.target.value)} placeholder={t('memories.track_title')} aria-label={t('memories.track_title')} />
                <input className="grow" value={trackArtist} onChange={e => setTrackArtist(e.target.value)} placeholder={t('memories.track_artist')} aria-label={t('memories.track_artist')} />
                <button type="button" className="btn sm" onClick={() => { URL.revokeObjectURL(track.url); setTrack(null) }}><Icon name="trash" size={13} /></button>
              </div>
            </div>
          )}
          <input ref={trackRef} type="file" accept="audio/mpeg,audio/mp4,audio/wav,audio/ogg,audio/flac,audio/opus" hidden onChange={e => { void addTrack(e.target.files?.[0] || null); e.target.value = '' }} />
        </Card>

        {busy === 'upload' && <Loading />}
      </div>
    </>
  )
}
