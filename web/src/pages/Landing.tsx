// The guest landing: the designer's pixel style, re-branded for 残像 Zanzō.
// Always dark, self-contained (landing.css), shown at `/` to unauthenticated visitors.
import { useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { LANGUAGES, useI18n } from '../lib/i18n'
import { Scramble, useReducedMotion } from '../lib/fx'
import '../styles/landing.css'

/** Pixel crescent moon: an N×N grid, outer disc minus an offset inner disc. */
function moonRects(n = 20, r = 9, dx = 3.4): string[] {
  const c = (n - 1) / 2, out: string[] = []
  for (let y = 0; y < n; y++)
    for (let x = 0; x < n; x++)
      if (Math.hypot(x - c, y - c) <= r && Math.hypot(x - c - dx, y - c) > r)
        out.push(`${x} ${y}`)
  return out
}

/** Pixel globe: a dot-hashed disc, deterministic. */
function globeRects(n = 20, r = 9): string[] {
  const c = (n - 1) / 2, out: string[] = []
  for (let y = 0; y < n; y++)
    for (let x = 0; x < n; x++)
      if (Math.hypot(x - c, y - c) <= r && ((x * 7 + y * 13) % 5) < 2)
        out.push(`${x} ${y}`)
  return out
}

const MOON = moonRects()
const GLOBE = globeRects()

/** Deterministic pseudo-random (stable across renders, no hydration surprises). */
function seeded(seed: number) { let s = seed >>> 0; return () => { s = (s * 1664525 + 1013904223) >>> 0; return s / 4294967296 } }

/** Cyberpunk rain streaks + floating dust, precomputed once. */
const RAIN = (() => { const r = seeded(20261121); return Array.from({ length: 34 }, () => ({ left: r() * 100, dur: 0.9 + r() * 1.6, delay: r() * 4, op: 0.05 + r() * 0.12, h: 14 + r() * 26 })) })()
const DUST = (() => { const r = seeded(777); return Array.from({ length: 20 }, () => ({ left: r() * 100, top: r() * 100, dur: 6 + r() * 9, delay: r() * 6, op: 0.12 + r() * 0.25, s: 1 + r() * 2 })) })()

function Langs() {
  const { choice, setLang } = useI18n()
  return (
    <div className="lz-langs" role="group" aria-label="Language">
      {LANGUAGES.map(l => (
        <button key={l.code} type="button" className={choice === l.code ? 'on' : ''} onClick={() => setLang(l.code)}>{l.code.toUpperCase()}</button>
      ))}
    </div>
  )
}

export default function Landing() {
  const { t } = useI18n()
  const { status } = useApp()
  const startTo = status?.needs_setup ? '/setup' : '/register'
  const loginTo = '/login'
  const tickerItems = t('lz.ticker').split('▪').map(s => s.trim()).filter(Boolean)
  const rootRef = useRef<HTMLDivElement>(null)
  const reduced = useReducedMotion()
  const fxOn = !reduced

  // Cinematic layers: cursor glow + hero parallax.
  useEffect(() => {
    const root = rootRef.current
    if (!root || !fxOn) return
    const onMove = (e: MouseEvent) => { root.style.setProperty('--mx', `${e.clientX}px`); root.style.setProperty('--my', `${e.clientY}px`) }
    const onScroll = () => { root.style.setProperty('--sy', `${window.scrollY}px`) }
    window.addEventListener('mousemove', onMove, { passive: true })
    window.addEventListener('scroll', onScroll, { passive: true })
    onScroll()
    return () => { window.removeEventListener('mousemove', onMove); window.removeEventListener('scroll', onScroll) }
  }, [fxOn])

  // Scroll reveal (IntersectionObserver); stagger delay is cleared once the reveal finishes.
  useEffect(() => {
    const root = rootRef.current
    if (!root) return
    const els = Array.from(root.querySelectorAll<HTMLElement>('.lz-rv'))
    if (!fxOn) { els.forEach(el => el.classList.add('in')); return }
    const io = new IntersectionObserver(entries => {
      for (const en of entries) {
        if (!en.isIntersecting) continue
        const el = en.target as HTMLElement
        io.unobserve(el)
        el.classList.add('in')
        const d = Number.parseFloat(el.style.transitionDelay) || 0
        if (d > 0) window.setTimeout(() => { el.style.transitionDelay = '' }, d + 900)
      }
    }, { threshold: 0.12, rootMargin: '0px 0px -8% 0px' })
    els.forEach(el => io.observe(el))
    return () => io.disconnect()
  }, [fxOn])

  return (
    <div className="lz" ref={rootRef}>
      {fxOn && (
        <div className="lz-fx" aria-hidden="true">
          <div className="lz-boot" />
          <div className="lz-glow" />
          <div className="lz-rain">{RAIN.map((d, i) => <i key={i} style={{ left: `${d.left}%`, height: `${d.h}px`, animationDuration: `${d.dur}s`, animationDelay: `${d.delay}s`, opacity: d.op }} />)}</div>
          <div className="lz-dust">{DUST.map((d, i) => <i key={i} style={{ left: `${d.left}%`, top: `${d.top}%`, width: `${d.s}px`, height: `${d.s}px`, animationDuration: `${d.dur}s`, animationDelay: `${d.delay}s`, opacity: d.op }} />)}</div>
          <div className="lz-film" />
        </div>
      )}
      <header className="lz-head">
        <div className="lz-wrap lz-head-in">
          <Link to="/" className="lz-brand"><img src="/favicon.svg" alt="" /><span>ZANZŌ</span></Link>
          <nav className="lz-nav" aria-label="Landing">
            <a href="#how">{t('lz.nav.how')}</a>
            <a href="#app">{t('lz.nav.app')}</a>
            <a href="#faq">{t('lz.nav.faq')}</a>
          </nav>
          <div className="lz-head-right">
            <Langs />
            {!status?.needs_setup && <Link to={loginTo} className="lz-btn">{t('lz.nav.signin')}</Link>}
            <Link to={startTo} className="lz-btn red">{t('lz.nav.start')}</Link>
          </div>
        </div>
      </header>

      <main>
        {/* hero */}
        <section className="lz-hero">
          <div className="lz-gridfloor" aria-hidden="true" />
          <div className="lz-streak" aria-hidden="true" />
          <div className="lz-wrap lz-hero-grid">
            <div>
              <p className="lz-kicker"><Scramble text={t('lz.hero.kicker')} /></p>
              <h1 className="lz-h1">
                <span className="lz-glitch" data-text={t('lz.hero.t1')}>{t('lz.hero.t1')}</span><br />
                <span className="lz-glitch" data-text={t('lz.hero.t2')}>{t('lz.hero.t2')}</span>
              </h1>
              <p className="lz-sub">{t('lz.hero.sub')}</p>
              <div className="lz-hero-form">
                <input className="lz-hero-input" type="text" readOnly aria-hidden="true" placeholder={t('lz.hero.input')} />
                <Link to={startTo} className="lz-btn red big">{t('lz.hero.cta')}</Link>
              </div>
              <p className="lz-hero-alt"><Link to={startTo}>{t('lz.hero.alt')} →</Link></p>
              <div className="lz-mascot">
                <img src="/favicon.svg" alt="" />
                <div>
                  <div className="lz-mascot-hi">{t('lz.mascot.hi')}</div>
                  <div className="lz-mascot-line">{t('lz.mascot.line')}</div>
                </div>
              </div>
            </div>
            <div className="lz-hero-art lz-para">
              <svg className="lz-moon" viewBox="0 0 20 20" shapeRendering="crispEdges" aria-hidden="true">
                {MOON.map(xy => <rect key={xy} x={xy} height="1" width="1" />)}
              </svg>
              <div className="lz-seal lz-neon" aria-hidden="true">残</div>
            </div>
          </div>
        </section>

        {/* ticker */}
        <div className="lz-ticker" aria-hidden="true">
          <div className="lz-ticker-track">
            {[0, 1].map(half => (
              <span key={half} className="lz-ticker-half">{tickerItems.map(i => `${i} ▪ `).join('')}</span>
            ))}
          </div>
        </div>

        {/* the app window */}
        <section id="app" className="lz-sec">
          <div className="lz-wrap lz-rv">
            <p className="lz-eyebrow">{t('lz.app.eyebrow')}</p>
            <h2 className="lz-h2">{t('lz.app.t1')} <span className="lz-accent">{t('lz.app.accent')}</span></h2>
            <p className="lz-sub">{t('lz.app.sub')}</p>
            <div className="lz-stage">
              <div className="lz-window">
                <div className="lz-win-bar">
                  <span className="lz-dot r" /><span className="lz-dot y" /><span className="lz-dot g" />
                  <span className="lz-win-title">ZANZŌ · {t('lz.app.day').toUpperCase()}</span>
                </div>
                <div className="lz-win-body">
                  <div>
                    <p className="lz-mock-day">{t('lz.app.day')}</p>
                    <div className="lz-mock-entry"><p>{t('lz.app.entry')}</p></div>
                    <div className="lz-mood"><b>{t('lz.app.mood_l')}</b><span>{t('lz.app.mood_v')}</span></div>
                    <div className="lz-mock-checks">
                      <div><span className="ok">✓</span><span>{t('lz.app.check1')}</span></div>
                      <div><span className="ok">✓</span><span>{t('lz.app.check2')}</span></div>
                      <div><span className="ok">✓</span><span>{t('lz.app.check3')}</span></div>
                    </div>
                  </div>
                  <div className="lz-mock-poster">
                    <span className="lz-mock-kanji" aria-hidden="true">残</span>
                    <div className="t">{t('lz.app.poster')}</div>
                    <div className="s">{t('lz.app.poster_sub')}</div>
                  </div>
                </div>
              </div>
              <span className="lz-chip c1">{t('lz.app.chip1')}</span>
              <span className="lz-chip c2">{t('lz.app.chip2')}</span>
              <span className="lz-chip c3">{t('lz.app.chip3')}</span>
            </div>
          </div>
        </section>

        {/* how it works (graph paper) */}
        <div className="lz-check" aria-hidden="true" />
        <section id="how" className="lz-sec lz-light">
          <div className="lz-wrap lz-rv">
            <p className="lz-eyebrow">{t('lz.how.eyebrow')}</p>
            <h2 className="lz-h2">{t('lz.how.t1')} <span className="lz-accent">{t('lz.how.accent')}</span></h2>
            <p className="lz-sub">{t('lz.how.sub')}</p>
            <div className="lz-steps">
              {[1, 2, 3].map(n => (
                <div className="lz-step lz-rv" key={n} style={{ transitionDelay: `${(n - 1) * 90}ms` }}>
                  <div className="lz-step-tag">{t(`lz.how.s${n}_tag`)}</div>
                  <h3 className="lz-step-title">{t(`lz.how.s${n}_title`)}</h3>
                  <p>{t(`lz.how.s${n}_text`)}</p>
                </div>
              ))}
            </div>
          </div>
        </section>
        <div className="lz-check" aria-hidden="true" />

        {/* rooms */}
        <section id="rooms" className="lz-sec">
          <div className="lz-wrap lz-rv">
            <p className="lz-eyebrow">{t('lz.rooms.eyebrow')}</p>
            <h2 className="lz-h2">{t('lz.rooms.t1')} <span className="lz-accent">{t('lz.rooms.accent')}</span></h2>
            <p className="lz-sub">{t('lz.rooms.sub')}</p>
            <div className="lz-rooms">
              {[1, 2, 3, 4].map(n => (
                <div className="lz-room lz-rv" key={n} style={{ transitionDelay: `${(n - 1) * 80}ms` }}>
                  <div className="lz-room-num">0{n}</div>
                  <h3>{t(`lz.rooms.r${n}`)}</h3>
                  <p>{t(`lz.rooms.r${n}_text`)}</p>
                  <Link className="lz-room-open" to={startTo}>{t('lz.rooms.open')}</Link>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* bento */}
        <section id="more" className="lz-sec">
          <div className="lz-wrap lz-rv">
            <p className="lz-eyebrow">{t('lz.every.eyebrow')}</p>
            <h2 className="lz-h2">{t('lz.every.t1')} <span className="lz-accent">{t('lz.every.accent')}</span></h2>
            <div className="lz-bento">
              <div className="lz-b lz-b1 lz-rv">
                <div className="lz-b-tag">{t('lz.every.b1_tag')}</div>
                <h3>{t('lz.every.b1_title')}</h3>
                <p>{t('lz.every.b1_text')}</p>
                <div className="lz-b-dates">{t('lz.every.b1_dates')}</div>
              </div>
              <div className="lz-b lz-b2 lz-rv" style={{ transitionDelay: '70ms' }}>
                <div className="lz-b-tag">{t('lz.every.b2_tag')}</div>
                <h3>{t('lz.every.b2_title')}</h3>
                <ul>
                  {['a', 'b', 'c', 'd'].map(k => (
                    <li key={k}><span className="ok">✓</span><span>{t(`lz.every.b2_${k}`)}</span></li>
                  ))}
                </ul>
              </div>
              <div className="lz-b lz-b3 lz-rv" style={{ transitionDelay: '140ms' }}>
                <div className="lz-b-tag">{t('lz.every.b3_tag')}</div>
                <h3>{t('lz.every.b3_title')}</h3>
                <pre>{t('lz.every.b3_text')}</pre>
              </div>
              <div className="lz-b lz-b4 lz-rv" style={{ transitionDelay: '210ms' }}>
                <div className="lz-b-tag">{t('lz.every.b4_tag')}</div>
                <h3>{t('lz.every.b4_title')}</h3>
                <pre>{t('lz.every.b4_text')}</pre>
              </div>
              <div className="lz-b lz-b5 lz-rv" style={{ transitionDelay: '280ms' }}>
                <svg viewBox="0 0 20 20" shapeRendering="crispEdges" aria-hidden="true">
                  {GLOBE.map(xy => <rect key={xy} x={xy} height="1" width="1" />)}
                </svg>
                <div className="lz-b-tag">{t('lz.every.b5_tag')}</div>
                <h3>{t('lz.every.b5_title')}</h3>
                <p>{t('lz.every.b5_text')}</p>
                <Link to={startTo} className="lz-btn">{t('lz.every.b5_cta')}</Link>
              </div>
            </div>
          </div>
        </section>

        {/* FAQ */}
        <section id="faq" className="lz-sec">
          <div className="lz-wrap lz-rv">
            <p className="lz-eyebrow">{t('lz.faq.eyebrow')}</p>
            <h2 className="lz-h2">{t('lz.faq.t1')} <span className="lz-accent">{t('lz.faq.accent')}</span></h2>
            <div className="lz-faq">
              {[1, 2, 3, 4, 5, 6].map(n => (
                <details className="lz-faq-item lz-rv" key={n} style={{ transitionDelay: `${(n - 1) * 60}ms` }}>
                  <summary>{t(`lz.faq.q${n}`)}<span className="lz-faq-plus" aria-hidden="true">+</span></summary>
                  <p>{t(`lz.faq.a${n}`)}</p>
                </details>
              ))}
            </div>
          </div>
        </section>

        {/* final CTA */}
        <section className="lz-final">
          <div className="lz-wrap lz-narrow lz-rv">
            <h2 className="lz-h2">{t('lz.final.t1')} <span className="lz-accent">{t('lz.final.accent')}</span></h2>
            <p className="lz-sub">{t('lz.final.sub')}</p>
            <Link to={startTo} className="lz-btn red big">{t('lz.final.cta')}</Link>
          </div>
        </section>
      </main>

      <footer className="lz-foot">
        <div className="lz-wrap lz-foot-in">
          <div className="lz-foot-line"><img src="/favicon.svg" alt="" /><span>{t('lz.footer.line')}</span></div>
          <div className="lz-foot-note">残 · ZANZŌ{status?.version ? ` · v${status.version}` : ''}</div>
        </div>
      </footer>
    </div>
  )
}
