// The day's remembrance as a 4:5 poster, composed deterministically from the
// art direction: the fixed language (washi, grain, tategaki date, hanko) plus
// what the mood changes (palette, grade, composition). Canvas, no libraries.

export interface PosterData {
  date: string
  title: string
  caption: string
  palette: string[]            // five hex colors, the last is the vermillion accent
  technique: string            // monet | kandinsky | minimal | mist
  grain: number                // 0..1
  photos: { src: string; name?: string }[]
  background?: string | null   // T2I background url
  track?: { title?: string; artist?: string } | null
  sealChar?: string            // the hanko character, default 残
}

const W = 1080
const H = 1350

function hexRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '')
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)]
}
function rgba(hex: string, a: number) {
  const [r, g, b] = hexRgb(hex)
  return `rgba(${r},${g},${b},${a})`
}

function loadImg(src: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => resolve(null)
    img.src = src
  })
}

/** Deterministic film grain: one noise tile, re-used with offsets. */
function grainTile(seed: number, density: number): HTMLCanvasElement {
  const S = 256
  const c = document.createElement('canvas')
  c.width = S; c.height = S
  const ctx = c.getContext('2d')!
  const img = ctx.createImageData(S, S)
  let s = seed
  const rnd = () => { s = (s * 1103515245 + 12345) & 0x7fffffff; return s / 0x7fffffff }
  for (let i = 0; i < img.data.length; i += 4) {
    const v = Math.floor(rnd() * 255)
    img.data[i] = img.data[i + 1] = img.data[i + 2] = v
    img.data[i + 3] = rnd() < density ? 22 : 0
  }
  ctx.putImageData(img, 0, 0)
  return c
}

/** Color-grade a photo toward the palette: draw it, then a low-alpha palette wash (multiply + screen). */
function drawGraded(ctx: CanvasRenderingContext2D, img: HTMLImageElement, x: number, y: number, w: number, h: number, palette: string[]) {
  ctx.save()
  ctx.beginPath(); ctx.rect(x, y, w, h); ctx.clip()
  const scale = Math.max(w / img.width, h / img.height)
  const dw = img.width * scale, dh = img.height * scale
  ctx.drawImage(img, x + (w - dw) / 2, y + (h - dh) / 2, dw, dh)
  // warm/cool wash from the day's palette, kept subtle
  ctx.globalCompositeOperation = 'multiply'
  ctx.fillStyle = rgba(palette[2] || palette[1], 0.18)
  ctx.fillRect(x, y, w, h)
  ctx.globalCompositeOperation = 'soft-light'
  ctx.fillStyle = rgba(palette[0], 0.22)
  ctx.fillRect(x, y, w, h)
  // vignette for the film feel
  const g = ctx.createRadialGradient(x + w / 2, y + h / 2, Math.min(w, h) * 0.5, x + w / 2, y + h / 2, Math.max(w, h) * 0.75)
  g.addColorStop(0, 'rgba(0,0,0,0)')
  g.addColorStop(1, 'rgba(0,0,0,0.28)')
  ctx.globalCompositeOperation = 'source-over'
  ctx.fillStyle = g
  ctx.fillRect(x, y, w, h)
  ctx.restore()
}

/** The procedural backdrop when no T2I background is present. */
function drawBackdrop(ctx: CanvasRenderingContext2D, d: PosterData) {
  const p = d.palette
  switch (d.technique) {
    case 'monet': {
      const g = ctx.createLinearGradient(0, 0, W, H)
      g.addColorStop(0, p[1]); g.addColorStop(0.55, p[0]); g.addColorStop(1, p[2])
      ctx.fillStyle = g; ctx.fillRect(0, 0, W, H)
      // soft brush swashes
      for (let i = 0; i < 26; i++) {
        ctx.save()
        ctx.globalAlpha = 0.05 + (i % 5) * 0.012
        ctx.fillStyle = p[i % 3 === 0 ? 2 : i % 3 === 1 ? 0 : 1]
        const y = (i / 26) * H
        ctx.beginPath()
        ctx.ellipse((i * 173) % W, y, 260 + (i * 37) % 180, 26, ((i % 6) - 3) * 0.06, 0, Math.PI * 2)
        ctx.fill()
        ctx.restore()
      }
      break
    }
    case 'kandinsky': {
      ctx.fillStyle = p[1]; ctx.fillRect(0, 0, W, H)
      // hard grid of lines and patches
      ctx.strokeStyle = rgba(p[3], 0.5); ctx.lineWidth = 3
      for (let i = 1; i < 8; i++) {
        ctx.beginPath(); ctx.moveTo((W / 8) * i, 0); ctx.lineTo((W / 8) * i, H); ctx.stroke()
        ctx.beginPath(); ctx.moveTo(0, (H / 10) * i); ctx.lineTo(W, (H / 10) * i); ctx.stroke()
      }
      const rects: [number, number, number, number, number][] = [[2, 1, 2, 2, 0], [6, 2, 1, 3, 2], [1, 7, 3, 1, 2], [5, 6, 2, 2, 0], [7, 8, 1, 1, 4], [3, 3, 1, 1, 4]]
      for (const [cx, cy, cw, ch, col] of rects) {
        ctx.fillStyle = rgba(p[col], col === 4 ? 0.85 : 0.22)
        ctx.fillRect((W / 8) * cx + 8, (H / 10) * cy + 8, (W / 8) * cw - 16, (H / 10) * ch - 16)
      }
      ctx.strokeStyle = rgba(p[4], 0.9); ctx.lineWidth = 6
      ctx.beginPath(); ctx.moveTo(0, H * 0.82); ctx.lineTo(W * 0.62, H * 0.3); ctx.stroke()
      break
    }
    case 'minimal': {
      ctx.fillStyle = p[0]; ctx.fillRect(0, 0, W, H)
      // one accent, one horizon, lots of air
      ctx.fillStyle = rgba(p[1], 0.9)
      ctx.beginPath(); ctx.arc(W * 0.72, H * 0.3, 92, 0, Math.PI * 2); ctx.fill()
      ctx.fillStyle = rgba(p[3], 0.14)
      ctx.fillRect(0, H * 0.66, W, 3)
      ctx.fillStyle = rgba(p[4], 0.8)
      ctx.fillRect(W * 0.08, H * 0.72, 64, 64)
      break
    }
    case 'mist': {
      const g = ctx.createLinearGradient(0, 0, 0, H)
      g.addColorStop(0, p[1]); g.addColorStop(0.6, p[0]); g.addColorStop(1, p[2])
      ctx.fillStyle = g; ctx.fillRect(0, 0, W, H)
      // haze bands
      for (let i = 0; i < 12; i++) {
        const y = (i / 12) * H
        const g2 = ctx.createLinearGradient(0, y, 0, y + 160)
        g2.addColorStop(0, 'rgba(255,255,255,0)')
        g2.addColorStop(0.5, rgba(p[1], 0.16))
        g2.addColorStop(1, 'rgba(255,255,255,0)')
        ctx.fillStyle = g2
        ctx.fillRect(0, y, W, 160)
      }
      break
    }
  }
}

function wrapText(ctx: CanvasRenderingContext2D, text: string, max: number): string[] {
  const words = text.split(/\s+/)
  const lines: string[] = []
  let cur = ''
  for (const w of words) {
    const t = cur ? cur + ' ' + w : w
    if (ctx.measureText(t).width > max && cur) { lines.push(cur); cur = w } else cur = t
  }
  if (cur) lines.push(cur)
  return lines
}

/** Compose the poster into the given canvas (its size is set here). */
export async function renderPoster(canvas: HTMLCanvasElement, d: PosterData): Promise<void> {
  canvas.width = W; canvas.height = H
  const ctx = canvas.getContext('2d')!
  // wait for the Noto Serif JP / Yomogi / Caveat webfonts before drawing text
  try { await document.fonts.ready } catch { /* keep the fallbacks */ }

  // Backdrop: the T2I background when present, else procedural.
  const bg = d.background ? await loadImg(d.background) : null
  if (bg) {
    ctx.drawImage(bg, 0, 0, W, H)
    // keep the text readable: a quiet veil of the day's paper tone
    ctx.fillStyle = rgba(d.palette[1], 0.25)
    ctx.fillRect(0, 0, W, H)
  } else {
    drawBackdrop(ctx, d)
  }

  const p = d.palette
  const ink = p[3]

  // Photos: one hero, or 2x2.
  const imgs = (await Promise.all(d.photos.slice(0, 4).map((ph) => loadImg(ph.src)))).filter(Boolean) as HTMLImageElement[]
  const margin = 84
  if (imgs.length === 1) {
    const s = W - margin * 2
    drawGraded(ctx, imgs[0], margin, 150, s, s, p)
  } else if (imgs.length >= 4) {
    const s = (W - margin * 2 - 24) / 2
    for (let i = 0; i < 4; i++) drawGraded(ctx, imgs[i], margin + (i % 2) * (s + 24), 150 + Math.floor(i / 2) * (s + 24), s, s, p)
  } else {
    const w = (W - margin * 2 - 24) / 2
    for (let i = 0; i < imgs.length; i++) drawGraded(ctx, imgs[i], margin + (i % 2) * (w + 24), 150 + Math.floor(i / 2) * (w + 24), w, w, p)
  }

  // Title — the day's name, serif.
  ctx.fillStyle = ink
  ctx.font = '600 58px "Noto Serif JP", serif'
  ctx.textAlign = 'left'
  let ty = 1180
  for (const line of wrapText(ctx, d.title, W - margin * 2).slice(0, 2)) {
    ctx.fillText(line, margin, ty)
    ty += 74
  }

  // Handwritten caption, the sensory line.
  ctx.font = '400 40px Yomogi, Caveat, cursive'
  ctx.fillStyle = rgba(ink, 0.82)
  for (const line of wrapText(ctx, d.caption, W - margin * 2).slice(0, 2)) {
    ctx.fillText(line, margin, ty + 14)
    ty += 52
  }

  // The track, small and quiet.
  if (d.track?.title) {
    ctx.font = '300 26px "Noto Serif JP", serif'
    ctx.fillStyle = rgba(ink, 0.55)
    const t = d.track.artist ? `${d.track.title} — ${d.track.artist}` : d.track.title
    ctx.fillText(t.length > 48 ? t.slice(0, 46) + '…' : t, margin, H - 132)
  }

  // Tategaki date: one character per column, right edge.
  const dateStr = d.date.replace(/-/g, '')
  ctx.font = '300 34px "Noto Serif JP", serif'
  ctx.fillStyle = rgba(ink, 0.6)
  ctx.textAlign = 'center'
  let ty2 = 150
  for (const ch of dateStr) {
    ctx.fillText(ch, W - margin - 26, ty2)
    ty2 += 44
  }

  // Hanko seal: vermillion square with the character, bottom right.
  const seal = d.sealChar || '残'
  const sx = W - margin - 84, sy = H - margin - 84
  ctx.fillStyle = rgba(p[4], 0.92)
  ctx.beginPath()
  const r = 10
  ctx.roundRect(sx, sy, 84, 84, r)
  ctx.fill()
  ctx.fillStyle = p[1]
  ctx.font = '600 52px "Noto Serif JP", serif'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(seal, sx + 42, sy + 46)
  ctx.textBaseline = 'alphabetic'

  // Washi + film grain over everything, the grain follows the motion of the day.
  const tile = grainTile(42, 0.05 + d.grain * 0.3)
  ctx.globalAlpha = 0.5 + d.grain * 0.5
  for (let y = 0; y < H; y += 256) {
    for (let x = 0; x < W; x += 256) {
      ctx.drawImage(tile, x + ((y * 37) % 64), y)
    }
  }
  ctx.globalAlpha = 1
}

/** Render and hand back a PNG blob for saving / uploading. */
export async function posterBlob(d: PosterData): Promise<Blob> {
  const c = document.createElement('canvas')
  await renderPoster(c, d)
  return new Promise((resolve, reject) =>
    c.toBlob((b) => (b ? resolve(b) : reject(new Error('poster render failed'))), 'image/png'))
}
