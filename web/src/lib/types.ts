// Shared types and small helpers for the web app.

export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }
export interface Currency { code: string; name: string; decimals: number }

export interface Settings {
  ai_provider: string; ai_configured: boolean
  title: string; date_format: string; time_format: string; week_start: 'mon' | 'sun'; home_currency: string; distance_unit: 'km' | 'mi'
  home_lat: number; home_lon: number; home_place: string; prompt: string
  notify_url: string; notify_format: string; notify_configured: boolean; notify_evening: boolean; evening_hour: number; notify_morning: boolean; morning_hour: number
  ai_key: string; ai_model: string; ai_base_url: string; ai_transcribe_model: string; ai_language: string
  ai_image_key: string; ai_image_model: string; ai_transcribe_key: string
  ai_ocr_key: string; ai_ocr_model: string; ai_rerank_key: string; ai_rerank_model: string
  ai_tts_key: string; ai_tts_model: string; ai_tts_voice: string
  ai_question: boolean; ai_weekly_story: boolean; ai_send_text: boolean; ai_default_model: string; ai_default_transcribe: string; ai_default_tts: string
  google_client_id: string; google_client_secret: string; google_account: string; google_connected: boolean; google_last_sync: string
  setup_complete: boolean; base_url: string; timezone: string
}

export interface Story { user_id: number; kind: 'week' | 'month' | 'year'; period: string; title: string; text: string; model: string; created_at: string }
export interface Ask { id: number; question: string; answer: string; dates: string[]; scope: string; model: string; created_at: string }
export interface UsageRow { feature: string; model: string; calls: number; prompt_tokens: number; completion_tokens: number }

export interface Moment { id: number; date: string; time: string; text: string; kind: string; created_at: string }
export interface Pin { id: number; user_id: number; date: string; source: string; ref: string; title: string; subtitle: string; image: string; created_at: string }

export interface HomeData {
  today: string
  stats: { total: number; this_month: number; this_year: number; mood_month: number; words: number; first_date: string; pinned: number; moments: number }
  streak: number
  longest_streak: number
  recent: Entry[]
  memories: Entry[]
}

export interface Entry { id: number; user_id: number; date: string; highlight: string; text: string; mood: number; tags: string; pinned: boolean; created_at: string; updated_at: string; moments?: number; words?: number }

export interface DayPage { date: string; weekday: number; today: boolean; future: boolean; entry: Entry | null; moments: Moment[]; pins: Pin[]; memories: Entry[]; prev_entry: string; next_entry: string; streak: number; prompt: string }

export interface CalendarDay { date: string; entry: boolean; mood: number; highlight: string; pinned: boolean; moments: number }
export interface MonthRecap { month: string; entries: number; moments: number; mood: number }
export interface YearRecap { year: number; months: MonthRecap[]; entries: number; moments: number; words: number; mood: number; moods: number[]; tags: { tag: string; count: number }[]; pinned: Entry[]; best: Entry[]; longest_streak: number; weekday_mood: number[] }

export const MOODS = ['', 'rough', 'meh', 'okay', 'good', 'great']
export const MOOD_ICON = ['○', '😖', '😕', '😐', '🙂', '😄']
export const MOOD_COLOR = ['var(--border)', '#ef4444', '#f59e0b', '#a3a3a3', '#22c55e', '#4f46e5']
export const KIND_ICON: Record<string, string> = { note: '•', win: '🏆', thanks: '🙏', idea: '💡', event: '📌' }
export const cap = (s: string) => s ? s[0].toUpperCase() + s.slice(1).replace(/_/g, ' ') : ''
export function fmtMinutes(m: number): string {
  if (!m) return ''
  const h = Math.floor(m / 60), mm = m % 60
  return h ? (mm ? `${h} h ${mm} min` : `${h} h`) : `${mm} min`
}
export function fmtKm(km: number, unit: 'km' | 'mi' = 'km'): string {
  if (unit === 'mi') return `${(km / 1.609344).toFixed(km >= 100 ? 0 : 1)} mi`
  return `${km.toFixed(km >= 100 ? 0 : 1)} km`
}
export function fmtAmount(v: number, cur: string): string {
  try { return new Intl.NumberFormat(undefined, { style: 'currency', currency: cur, maximumFractionDigits: 2 }).format(v) } catch { return `${v.toFixed(2)} ${cur}` }
}
export const tagList = (t: string) => t ? t.split(',').filter(Boolean) : []

// ---- memories: a day, developed into a remembrance ----
export interface ArtDirection {
  title: string
  mood_cluster: string
  palette: string[]
  technique: string
  caption: string
  voice: { rate: number; pitch: number; warmth: string }
  motion: { frame_sec: number; grain: number }
  reasons: string[]
}
/** Provenance of an open-licensed track found on Openverse (CC0/CC BY/CC BY-SA). */
export interface TrackMeta { title?: string; artist?: string; bpm?: number; key?: string; major?: boolean | null; energy?: number; valence?: number; mime?: string; url?: string; source?: string; license?: string; license_url?: string; attribution?: string }
export interface MemoryPhoto { id: number; memory_id: number; path: string; colors: number[]; sort: number }
export interface Memory {
  id: number; user_id: number; date: string; title: string; text: string
  mood_cluster: string; art_direction: ArtDirection | string | null
  poster_path: string; video_path: string; narration_path: string
  track_path: string; track_url: string; track_meta: TrackMeta | string | null
  status: string; created_at: string; updated_at: string; photos?: number
  is_public?: boolean
}
/** One entry of the public Posts feed. */
export interface PublicPost extends Memory {
  author: string; is_mine: boolean
  author_id: number
  reactions?: Record<string, number>
  my_reactions?: string[]
  comments?: number
}
/** A haiku line under a public day. */
export interface SocialComment {
  id: number; memory_id: number; body: string; created_at: string
  author: string; mine: boolean
}
/** One public day with its reactions and lines. */
export interface PostDetail {
  post: PublicPost
  reactions: { counts: Record<string, number>; mine: string[] }
  comments: SocialComment[]
}
/** The newest public day of a person, as a chip. */
export interface MiniPost { id: number; title: string; date: string; poster_path: string; mood_cluster: string }
/** A wanderer of the floating world. */
export interface Person {
  id: number; name: string; public_posts: number
  followers: number; following: number; i_follow: boolean
  latest?: MiniPost
}
export interface PeoplePage { people: Person[]; me: number }
export interface PersonDetail {
  person: Person; featured: PublicPost | null
  posts: PublicPost[]; total: number; me: number
}
/** Normalize the JSON fields that may arrive as strings. */
export function memoryArt(m: Memory): ArtDirection | null {
  if (!m.art_direction) return null
  if (typeof m.art_direction === 'string') { try { return JSON.parse(m.art_direction) } catch { return null } }
  return m.art_direction
}
/** The license label of an open-licensed track, when known. */
export function trackLicenseLabel(meta: TrackMeta | null): string {
  if (!meta?.license) return ''
  return meta.license === 'cc0' ? 'CC0' : meta.license === 'by' ? 'CC BY' : meta.license === 'by-sa' ? 'CC BY-SA' : meta.license.toUpperCase()
}
export function memoryTrack(m: Memory): TrackMeta | null {
  if (!m.track_meta) return null
  if (typeof m.track_meta === 'string') { try { return JSON.parse(m.track_meta) } catch { return null } }
  return m.track_meta
}
