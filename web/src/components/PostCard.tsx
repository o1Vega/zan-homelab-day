// PostCard: one public day in the floating world. Instead of likes — four
// gestures (sakura / kintsugi / ma / hanabi), the day's palette dots, and a
// slow mono-no-aware fade as the day grows old.
import { useMemo, useState, type KeyboardEvent, type MouseEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { postAssetUrl, toggleReaction } from '../lib/mem-api'
import { memoryArt, type PublicPost } from '../lib/types'
import { useI18n } from '../lib/i18n'
import { daysBetween, monthName, todayISO } from '../lib/format'
import { errMsg, useToast } from './ui'
import { Icon } from './icons'

export const RX_KINDS = ['sakura', 'kintsugi', 'ma', 'hanabi'] as const

export const RX_GLYPH: Record<string, string> = { sakura: '🌸', kintsugi: '金', ma: '間', hanabi: '✶' }

/** Mono no aware: the day fades as it grows old. */
export function ageClass(dateIso: string): string {
  const d = daysBetween(dateIso, todayISO())
  if (d > 90) return 'aw-3'
  if (d > 30) return 'aw-2'
  if (d > 7) return 'aw-1'
  return 'aw-0'
}

const grad = (pal: string[]) =>
  `linear-gradient(160deg, ${pal[1] ?? '#3a3a4a'} 0%, ${pal[0] ?? '#26262e'} 55%, ${pal[2] ?? '#1c1c22'} 100%)`

export default function PostCard({
  post,
  onAuthor,
  onFlower,
}: {
  post: PublicPost
  onAuthor?: () => void
  onFlower?: { featured: boolean; toggle: () => void }
}) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [rx, setRx] = useState<Record<string, number>>(post.reactions ?? {})
  const [mine, setMine] = useState<Set<string>>(() => new Set(post.my_reactions ?? []))
  const [busy, setBusy] = useState('')
  const pal = useMemo(() => memoryArt(post)?.palette ?? [], [post])
  const days = daysBetween(post.date, todayISO())

  const toggle = (kind: string) => {
    if (busy) return
    setBusy(kind)
    const had = mine.has(kind)
    setMine((s) => {
      const n = new Set(s)
      if (had) n.delete(kind)
      else n.add(kind)
      return n
    })
    toggleReaction(post.id, kind)
      .then((res) => {
        setRx((r) => ({ ...r, [kind]: res.count }))
        setMine((s) => {
          const n = new Set(s)
          if (res.active) n.add(kind)
          else n.delete(kind)
          return n
        })
      })
      .catch((err) => {
        setMine((s) => {
          const n = new Set(s)
          if (had) n.add(kind)
          else n.delete(kind)
          return n
        })
        toast(errMsg(err))
      })
      .finally(() => setBusy(''))
  }

  const click = (fn: () => void) => (e: MouseEvent) => {
    e.stopPropagation()
    fn()
  }
  const key = (fn: () => void) => (e: KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      e.stopPropagation()
      fn()
    }
  }

  const openAuthor = () => {
    if (onAuthor) onAuthor()
    else navigate(`/people/${post.author_id}`)
  }

  return (
    <button
      type="button"
      className={`mcard pcard ${ageClass(post.date)}`}
      onClick={() => navigate(`/post/${post.id}`)}
      title={post.author}
    >
      <span className="pcover">
        {post.poster_path ? (
          <img src={postAssetUrl(post.id, post.poster_path)} alt="" loading="lazy" />
        ) : (
          <span className="pcov-grad" style={{ background: grad(pal) }} />
        )}
        <span className="pcover-seal" aria-hidden>残</span>
        <span className="mcard-chips">
          {post.is_mine && <span className="mcard-chip">{t('post.yours')}</span>}
          {(post.photos ?? 0) > 0 && (
            <span className="mcard-chip"><Icon name="image" size={11} />{post.photos}</span>
          )}
          {(post.comments ?? 0) > 0 && <span className="mcard-chip">行 {post.comments}</span>}
          {onFlower && (
            <span
              className="mcard-chip"
              role="button"
              tabIndex={0}
              title={onFlower.featured ? t('tokonoma.unset') : t('tokonoma.set')}
              style={onFlower.featured ? { background: 'rgba(239, 143, 179, 0.3)' } : undefined}
              onClick={click(onFlower.toggle)}
              onKeyDown={key(onFlower.toggle)}
            >
              ✿
            </span>
          )}
        </span>
      </span>
      <span className="mcard-foot">
        <span className="mcard-title">{post.title || post.date}</span>
        <span className="mcard-meta">
          <span
            className="mcard-author"
            role="button"
            tabIndex={0}
            onClick={click(openAuthor)}
            onKeyDown={key(openAuthor)}
          >
            {post.author}
          </span>
          <span>{post.date.slice(8, 10)} {monthName(Number(post.date.slice(5, 7)), false)}</span>
          {days > 0 && <span className="age-tag">{t('post.age', { days })}</span>}
        </span>
        <span className="mcard-rx">
          {RX_KINDS.map((k) => (
            <span
              key={k}
              className={`mrx${mine.has(k) ? ' on' : ''}`}
              role="button"
              tabIndex={0}
              title={t(`rx.${k}`)}
              onClick={click(() => toggle(k))}
              onKeyDown={key(() => toggle(k))}
            >
              {RX_GLYPH[k]}
              {(rx[k] ?? 0) > 0 && <b>{rx[k]}</b>}
            </span>
          ))}
          {pal.slice(0, 4).map((c, i) => (
            <span key={i} className="pdot" style={{ background: c }} />
          ))}
        </span>
      </span>
    </button>
  )
}
