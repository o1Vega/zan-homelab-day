// The day's-folder API: a memory is one day's text + photos + track, developed
// into an art direction and a rendered remembrance.
import { api, V1, ApiError } from './api'
import type { ArtDirection, Memory, MemoryPhoto, PeoplePage, Person, PersonDetail, PostDetail, PublicPost, SocialComment, TrackMeta } from './types'
import type { TrackFeatures } from './audio'

export type { ArtDirection }

/** Create (or reopen) the memory for a date from its text. */
export function createMemory(date: string, text: string): Promise<Memory> {
  return api.post<Memory>(`${V1}/memories`, { date, text })
}

export function listMemories(): Promise<Memory[]> {
  return api.get<Memory[]>(`${V1}/memories`)
}

export interface MemoryDetail { memory: Memory; photos: MemoryPhoto[] }
export function getMemory(id: number): Promise<MemoryDetail> {
  return api.get<MemoryDetail>(`${V1}/memories/${id}`)
}

export function updateMemory(id: number, body: { text?: string; title?: string }): Promise<Memory> {
  return api.put<Memory>(`${V1}/memories/${id}`, body)
}

export function deleteMemory(id: number): Promise<void> {
  return api.del<void>(`${V1}/memories/${id}`)
}

/** Upload one or more photos (multipart "photos") with measured colors. */
export function uploadPhotos(id: number, files: File[], colors: number[][][]): Promise<{ photos: { name: string; type: string; size: number }[] }> {
  return new Promise((resolve, reject) => {
    const fd = new FormData()
    for (const f of files) fd.append('photos', f)
    if (colors.length) fd.append('colors', JSON.stringify(colors))
    fetch(`${V1}/memories/${id}/photos`, { method: 'POST', body: fd, credentials: 'same-origin' })
      .then(async (res) => {
        const j = await res.json().catch(() => ({}))
        if (!res.ok) reject(new ApiError(res.status, j.error || res.statusText, j))
        else resolve(j)
      })
      .catch(reject)
  })
}

export function deletePhoto(id: number, pid: number): Promise<void> {
  return api.del<void>(`${V1}/memories/${id}/photos/${pid}`)
}

/** Upload the track with the client-measured features. */
export function uploadTrack(id: number, file: File, f: TrackFeatures, title: string, artist: string): Promise<{ track: { file: string; type: string } }> {
  return new Promise((resolve, reject) => {
    const fd = new FormData()
    fd.append('file', file)
    fd.append('title', title)
    fd.append('artist', artist)
    if (f.bpm) fd.append('bpm', String(f.bpm))
    if (f.key) fd.append('key', f.key)
    if (f.major !== null) fd.append('major', String(f.major))
    if (f.energy) fd.append('energy', String(f.energy))
    if (f.valence) fd.append('valence', String(f.valence))
    fetch(`${V1}/memories/${id}/track`, { method: 'POST', body: fd, credentials: 'same-origin' })
      .then(async (res) => {
        const j = await res.json().catch(() => ({}))
        if (!res.ok) reject(new ApiError(res.status, j.error || res.statusText, j))
        else resolve(j)
      })
      .catch(reject)
  })
}

/** Set (or clear) the day's song as a link to a music service. */
export function setTrackLink(id: number, body: { url: string; title?: string; artist?: string }): Promise<{ track_url: string }> {
  return api.put<{ track_url: string }>(`${V1}/memories/${id}/track`, body)
}

/** Ask the server to find an open-licensed track (CC0/CC BY/CC BY-SA) that
 *  matches the day's mood and download it into the day's folder.
 *  force=true re-rolls a different track. */
export function findTrack(id: number, force = false): Promise<{ track: { file: string; meta: TrackMeta } }> {
  return api.post<{ track: { file: string; meta: TrackMeta } }>(`${V1}/memories/${id}/track/find${force ? '?force=1' : ''}`)
}

/** Render the day's short video (poster + photos over the track). */
export function renderVideo(id: number, force = false, engine?: 'ai'): Promise<{ file: string; cached: boolean; seconds?: number }> {
  const q = [force ? 'force=1' : '', engine ? `engine=${engine}` : ''].filter(Boolean).join('&')
  return api.post<{ file: string; cached: boolean; seconds?: number }>(`${V1}/memories/${id}/video${q ? `?${q}` : ''}`)
}

/** Speak the day's text with the mood's voice; the audio is stored in the folder. */
export function renderNarration(id: number, force = false): Promise<{ file: string; cached: boolean }> {
  return api.post<{ file: string; cached: boolean }>(`${V1}/memories/${id}/narration${force ? '?force=1' : ''}`)
}

/** Develop the atmosphere: text + photo colors + track → art direction. */
export function analyze(id: number): Promise<ArtDirection> {
  return api.post<ArtDirection>(`${V1}/memories/${id}/analyze`)
}

/** Develop the T2I background (cached; force=1 re-rolls). */
export function background(id: number, force = false): Promise<{ file: string; cached: boolean }> {
  return api.post<{ file: string; cached: boolean }>(`${V1}/memories/${id}/background${force ? '?force=1' : ''}`)
}

/** Save the browser-rendered poster into the day's folder. */
export function savePoster(id: number, blob: Blob): Promise<{ poster: string }> {
  return new Promise((resolve, reject) => {
    const fd = new FormData()
    fd.append('file', blob, 'poster.png')
    fetch(`${V1}/memories/${id}/poster`, { method: 'POST', body: fd, credentials: 'same-origin' })
      .then(async (res) => {
        const j = await res.json().catch(() => ({}))
        if (!res.ok) reject(new ApiError(res.status, j.error || res.statusText, j))
        else resolve(j)
      })
      .catch(reject)
  })
}

/** One asset of the day's folder (photo, track, background, poster, memory.json). */
export function assetUrl(id: number, file: string): string {
  return `${V1}/memories/${id}/asset?file=${encodeURIComponent(file)}`
}

export interface PostsPage { posts: PublicPost[]; total: number }

/** The public feed: days other people chose to share, newest first. */
export function listPosts(limit = 24, offset = 0): Promise<PostsPage> {
  return api.get<PostsPage>(`${V1}/posts?limit=${limit}&offset=${offset}`)
}

/** Toggle the public flag of one of your memories. */
export function setMemoryPublic(id: number, isPublic: boolean): Promise<{ is_public: boolean }> {
  return api.put<{ is_public: boolean }>(`${V1}/memories/${id}/public`, { public: isPublic })
}

/** One asset of a public day's folder, served without an owner token. */
export function postAssetUrl(id: number, file: string): string {
  return `${V1}/posts/${id}/asset?file=${encodeURIComponent(file)}`
}

// ---- the floating world: the social layer over public days ----

/** The floating-world feed. followsOnly: only the days you follow. authorId: one wanderer's days. */
export function listFeed(followsOnly = false, authorId = 0, limit = 24, offset = 0): Promise<PostsPage> {
  const q = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  if (followsOnly) q.set('follows', '1')
  if (authorId > 0) q.set('author', String(authorId))
  return api.get<PostsPage>(`${V1}/posts?${q}`)
}

/** One public day with its four gestures and its haiku lines. */
export function getPost(id: number): Promise<PostDetail> {
  return api.get<PostDetail>(`${V1}/posts/${id}`)
}

/** The LLM-themed look of a day's page: palette, effect, background, tagline,
 *  a mini-story in the day's language and the page's opening animation. */
export interface PostTheme {
  mood: string
  palette: string[]
  effect: 'sakura' | 'hanabi' | 'kintsugi' | 'ma' | 'rain' | 'snow' | 'embers' | 'stars' | 'none'
  intensity: number
  background: string
  tagline: string
  story: string
  opening: 'ink' | 'petals' | 'rise' | 'fade' | 'brush' | 'wave' | 'ma' | string
  seal: string
}
/** Ask the model for the day's page theme (cached in the day's folder). */
export function getPostTheme(id: number, force = false): Promise<{ theme: PostTheme; cached: boolean }> {
  return api.get<{ theme: PostTheme; cached: boolean }>(`${V1}/posts/${id}/theme${force ? '?force=1' : ''}`)
}

/** The theme of your own day; rolled synchronously (and cached) on first open. */
export function getMemoryTheme(id: number, force = false): Promise<{ theme: PostTheme; cached: boolean }> {
  return api.get<{ theme: PostTheme; cached: boolean }>(`${V1}/memories/${id}/theme${force ? '?force=1' : ''}`)
}

/** Press (or lift) one of the four gestures: sakura, kintsugi, ma, hanabi. */
export function toggleReaction(id: number, kind: string): Promise<{ kind: string; active: boolean; count: number }> {
  return api.put<{ kind: string; active: boolean; count: number }>(`${V1}/posts/${id}/reaction/${kind}`)
}

/** Leave a haiku line under a public day (up to 200 characters). */
export function addComment(id: number, body: string): Promise<SocialComment> {
  return api.post<SocialComment>(`${V1}/posts/${id}/comments`, { body })
}

/** Erase one of your own lines. */
export function deleteComment(id: number, cid: number): Promise<void> {
  return api.del<void>(`${V1}/posts/${id}/comments/${cid}`)
}

/** The wanderers of the floating world — everyone who shares at least one day. */
export function listPeople(): Promise<PeoplePage> {
  return api.get<PeoplePage>(`${V1}/people`)
}

/** One wanderer's tokonoma: the hung scroll, the flower, the shelf of days. */
export function getPerson(id: number, limit = 24): Promise<PersonDetail> {
  return api.get<PersonDetail>(`${V1}/people/${id}?limit=${limit}`)
}

/** Follow a wanderer. */
export function followPerson(id: number): Promise<{ following: boolean }> {
  return api.post<{ following: boolean }>(`${V1}/people/${id}/follow`)
}

/** Let go of a wanderer. */
export function unfollowPerson(id: number): Promise<{ following: boolean }> {
  return api.del<{ following: boolean }>(`${V1}/people/${id}/follow`)
}

/** Hang one of your days in your tokonoma (or take it down). */
export function setFeatured(id: number, on: boolean): Promise<{ featured: boolean }> {
  return api.put<{ featured: boolean }>(`${V1}/memories/${id}/featured`, { featured: on })
}
