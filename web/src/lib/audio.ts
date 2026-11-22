// Audio features from the track the person attaches: bpm, key, major/minor,
// energy and valence. The song must influence the look of the day, so we
// measure it, not just attach it.
//
// essentia.js (WASM) runs in the browser, nothing is sent to a server. When
// WASM or a feature fails (Safari decode, unusual codec) the result degrades
// field by field to 0/unknown and the UI lets the person type the key.

export interface TrackFeatures {
  bpm: number       // 0 when unknown
  key: string       // e.g. "Am", "C", "" when unknown
  major: boolean | null // null when unknown
  energy: number    // 0..1, 0 when unknown
  valence: number   // 0..1, 0 when unknown (brightness proxy)
  measured: string[] // which fields came from the analyzer
}

type Essentia = {
  arrayToVector(a: Float32Array): any
  vectorToArray(v: any): Float32Array
  FrameGenerator(signal: any, frameSize?: number, hopSize?: number): any
  FFT(frame: any): any
  BeatTrackerDegara(signal: any, maxTempo?: number, minTempo?: number): any
  BpmRubato(beats: any): any
  KeyExtractor(audio: any, opts?: Record<string, unknown>): any
  Energy(array: any): any
  Centroid(array: any): any
  shutdown?(): void
}

let essentiaPromise: Promise<Essentia | null> | null = null

/** Load the WASM backend once; resolves null when it cannot (we degrade). */
function loadEssentia(): Promise<Essentia | null> {
  if (!essentiaPromise) {
    essentiaPromise = (async () => {
      try {
        const mod: any = await import('essentia.js')
        const wasm: any = mod.EssentiaWASM
        if (wasm && (wasm.ready?.then || wasm.ready === undefined)) {
          const r = wasm.ready
          if (r && typeof r.then === 'function') await r
        }
        const e = new mod.Essentia(wasm)
        return e as unknown as Essentia
      } catch {
        return null
      }
    })()
  }
  return essentiaPromise
}

function decode(file: File): Promise<AudioBuffer | null> {
  return new Promise((resolve) => {
    const ctx = new AudioContext()
    file.arrayBuffer()
      .then((ab) => ctx.decodeAudioData(ab))
      .then((buf) => { ctx.close().catch(() => {}); resolve(buf) })
      .catch(() => { ctx.close().catch(() => {}); resolve(null) })
  })
}

function toMono(buf: AudioBuffer): Float32Array {
  const ch0 = buf.getChannelData(0)
  if (buf.numberOfChannels === 1) return ch0
  const ch1 = buf.getChannelData(1)
  const out = new Float32Array(buf.length)
  for (let i = 0; i < buf.length; i++) out[i] = (ch0[i] + ch1[i]) / 2
  return out
}

const clamp01 = (x: number) => Math.max(0, Math.min(1, x))

/** Measure the track's features. Every field degrades to 0/unknown on error. */
export async function analyzeTrack(file: File): Promise<TrackFeatures> {
  const out: TrackFeatures = { bpm: 0, key: '', major: null, energy: 0, valence: 0, measured: [] }
  const buf = await decode(file)
  if (!buf) return out

  const e = await loadEssentia()
  if (!e) return out

  try {
    const mono = toMono(buf)
    const signal = e.arrayToVector(mono)

    // Tempo: beats → bpm, folded into 60..180.
    try {
      const beats = e.vectorToArray(e.BeatTrackerDegara(signal, 200, 60))
      const bpm = e.BpmRubato(beats)
      if (typeof bpm === 'number' && bpm > 30 && bpm < 400) {
        out.bpm = Math.round(bpm)
        out.measured.push('bpm')
      }
    } catch { /* no beats — leave 0 */ }

    // Energy: RMS of the whole signal, normalized against a typical full-scale mix.
    try {
      let sum = 0
      const step = Math.max(1, Math.floor(mono.length / 200000))
      let n = 0
      for (let i = 0; i < mono.length; i += step) { sum += mono[i] * mono[i]; n++ }
      const rms = n ? Math.sqrt(sum / n) : 0
      out.energy = clamp01(rms / 0.15) // 0.15 rms ≈ a loud modern mix
      out.measured.push('energy')
    } catch { /* leave 0 */ }

    // Valence proxy: mean spectral centroid (brightness) over a few frames.
    try {
      // materialize the frame stream once, then sample frames evenly from it
      const frames = e.vectorToArray(e.FrameGenerator(signal, 4096, 2048))
      const nFrames = Math.floor(frames.length / 4096)
      const sample = Math.min(64, Math.max(1, nFrames))
      const step = Math.max(1, Math.floor(nFrames / sample))
      let cent = 0
      let counted = 0
      for (let i = 0; i < nFrames; i += step) {
        const slice = frames.subarray(i * 4096, i * 4096 + 4096)
        if (slice.length < 4096) break
        const spec = e.FFT(e.arrayToVector(slice))
        const c = e.Centroid(spec)
        if (typeof c === 'number' && c > 0) { cent += Math.min(c / 12000, 1); counted++ }
      }
      if (counted > 0) {
        out.valence = clamp01(cent / counted + 0.15)
        out.measured.push('valence')
      }
    } catch { /* leave 0 */ }

    // Key: the full extractor is heavy in WASM; try once, time-boxed.
    try {
      const keyRes = e.KeyExtractor(signal, {
        frameSize: 4096, hopSize: 2048, hpcpSize: 8,
        sampleRate: buf.sampleRate, profileType: 'krumhansl',
      })
      const key = (keyRes && (keyRes.key as string)) || ''
      if (key) {
        out.key = key.replace('Major', '').replace('Minor', '').trim()
        out.major = !/min$/i.test(String(keyRes.key ?? key))
        out.measured.push('key')
      }
    } catch { /* leave unknown — the person can type it */ }
  } finally {
    try { e.shutdown?.() } catch { /* already gone */ }
  }
  return out
}

/** Dominant colors of a photo, normalized 0..1, for the atmosphere input. */
export function photoColors(file: File): Promise<number[][]> {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => {
      try {
        const c = document.createElement('canvas')
        const S = 48
        c.width = S; c.height = S
        const ctx = c.getContext('2d')
        if (!ctx) { URL.revokeObjectURL(url); resolve([]); return }
        ctx.drawImage(img, 0, 0, S, S)
        const d = ctx.getImageData(0, 0, S, S).data
        // 3x3 grid of 4x4 blocks → up to 9 dominant colors
        const out: number[][] = []
        for (let by = 0; by < S; by += S / 3) {
          for (let bx = 0; bx < S; bx += S / 3) {
            let r = 0, g = 0, b = 0, n = 0
            for (let y = Math.floor(by); y < by + S / 3 && y < S; y++) {
              for (let x = Math.floor(bx); x < bx + S / 3 && x < S; x++) {
                const i = (y * S + x) * 4
                r += d[i]; g += d[i + 1]; b += d[i + 2]; n++
              }
            }
            if (n) out.push([r / n / 255, g / n / 255, b / n / 255])
          }
        }
        resolve(out)
      } catch { resolve([]) } finally { URL.revokeObjectURL(url) }
    }
    img.onerror = () => { URL.revokeObjectURL(url); resolve([]) }
    img.src = url
  })
}

/** Speak the day's text with the voice the direction chose (browser TTS). */
export function narrate(text: string, voice: { rate: number; pitch: number; warmth: string }): { cancel: () => void } {
  const u = new SpeechSynthesisUtterance(text)
  u.rate = voice.rate || 1
  u.pitch = voice.pitch || 1
  const lang = (navigator.language || 'en').slice(0, 2)
  u.lang = lang
  // Pick a voice that fits the warmth when the browser offers one.
  const voices = window.speechSynthesis?.getVoices() ?? []
  const pref = voice.warmth === 'soft' ? ['quiet', 'gentle', 'calm', 'female', 'woman'] :
    voice.warmth === 'bright' ? ['lively', 'young', 'child'] : []
  const match = voices.find((v) => v.lang.startsWith(lang) && pref.some((p) => v.name.toLowerCase().includes(p)))
    || voices.find((v) => v.lang.startsWith(lang))
    || voices[0]
  if (match) u.voice = match
  window.speechSynthesis?.speak(u)
  return { cancel: () => window.speechSynthesis?.cancel() }
}
