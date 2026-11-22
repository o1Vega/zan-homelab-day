import { useEffect, useRef, useState, type ReactNode } from 'react'

/* ============================================================
   ZANZŌ · FX — японские мотивы движения
   Text Scramble (глифы-кандзи) · stamp · ticker · page-in
   ============================================================ */

const GLYPHS = '残像記憶夢夜月雨風光時空山水火川雲空星花雪桜木石田日米!<>-_\\/[]{}—=+*^?#'

function randGlyph(): string {
  return GLYPHS[(Math.random() * GLYPHS.length) | 0]
}

function scrambleOf(text: string): string {
  let s = ''
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    s += c === ' ' ? ' ' : randGlyph()
  }
  return s
}

export function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() =>
    typeof window !== 'undefined' ? window.matchMedia('(prefers-reduced-motion: reduce)').matches : false
  )
  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const on = (e: MediaQueryListEvent) => setReduced(e.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])
  return reduced
}

/** Text Scramble: посимвольная «раскодировка» из канджи-глифов. */
export function useScramble(text: string, delay = 0): string {
  const reduced = useReducedMotion()
  const [out, setOut] = useState(() => {
    if (typeof window === 'undefined') return text
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return text
    return scrambleOf(text)
  })

  useEffect(() => {
    if (reduced || !text) {
      setOut(text)
      return
    }
    let interval: ReturnType<typeof setInterval> | null = null
    let frame = 0
    const run = () => {
      setOut(scrambleOf(text))
      interval = setInterval(() => {
        frame++
        if (frame >= text.length) {
          setOut(text)
          if (interval) clearInterval(interval)
          return
        }
        let s = ''
        for (let i = 0; i < text.length; i++) {
          const c = text[i]
          s += i < frame || c === ' ' ? c : randGlyph()
        }
        setOut(s)
      }, 28)
    }
    const timer = delay > 0 ? setTimeout(run, delay) : (run(), 0)
    return () => {
      if (timer) clearTimeout(timer)
      if (interval) clearInterval(interval)
    }
  }, [text, delay, reduced])

  return out
}

export type ScrambleTag = 'span' | 'h1' | 'h2' | 'h3' | 'div' | 'p' | 'strong'

export function Scramble({
  text,
  as = 'span',
  className,
  delay = 0,
}: {
  text: string
  as?: ScrambleTag
  className?: string
  delay?: number
}) {
  const out = useScramble(text, delay)
  const Tag = as
  return <Tag className={className}>{out}</Tag>
}

export type TickerItem = { t: string; hot?: boolean }

/** Бесшовная пиксельная бегущая строка (контент продублирован дважды). */
export function Ticker({ items }: { items: TickerItem[] }) {
  const row = [...items, ...items]
  return (
    <div className="fx-ticker" aria-hidden>
      <div className="fx-ticker-track">
        {row.map((it, i) => (
          <span key={i} className={it.hot ? 'tk-red' : undefined}>
            {it.t}
          </span>
        ))}
      </div>
    </div>
  )
}

/** Иероглифы-печати для разделов (по первому сегменту пути). */
export const KANJI: Record<string, string> = {
  '': '今',
  day: '今',
  calendar: '暦',
  journal: '記',
  year: '年',
  ask: '問',
  stories: '話',
  memories: '残',
  memory: '残',
  posts: '帖',
  post: '帖',
  people: '人',
  settings: '設',
}

export function kanjiFor(pathname: string): string {
  const head = (pathname.split('/').filter(Boolean)[0] ?? '') as string
  return KANJI[head] ?? '残'
}

/** Одноразовый «подъём» блока при попадании в вьюпорт. */
export function Reveal({ children, delay = 0, className }: { children: ReactNode; delay?: number; className?: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const reduced = useReducedMotion()
  const [vis, setVis] = useState(false)
  useEffect(() => {
    if (reduced) {
      setVis(true)
      return
    }
    const el = ref.current
    if (!el) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          setVis(true)
          io.disconnect()
        }
      },
      { threshold: 0.08, rootMargin: '0px 0px -6% 0px' }
    )
    io.observe(el)
    return () => io.disconnect()
  }, [reduced])
  return (
    <div
      ref={ref}
      className={`${className ? className + ' ' : ''}fx-reveal${vis ? ' on' : ''}`}
      style={{ transitionDelay: `${delay}ms` }}
    >
      {children}
    </div>
  )
}
