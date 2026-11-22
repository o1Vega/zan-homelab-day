import { useMemo, useState } from 'react'
import { api, V1 } from '../lib/api'
import { KIND_ICON, type Moment } from '../lib/types'
import { useI18n } from '../lib/i18n'
import { Card, Segmented, useToast } from './ui'
import { Icon } from './icons'
import { DictateButton, useAI } from './ai'

/** A moment's kind as a stroke icon (the API keeps note / win / thanks / idea / event). */
export const KIND_GLYPH: Record<string, string> = { note: 'note', win: 'trophy', thanks: 'heart', idea: 'bulb', event: 'pin' }
const g = (name: string) => <Icon name={name} size={14} strokeWidth={2} />

/** The day in time order, with the moments you add in between. */
export function Timeline({ date, moments, isToday, onMoments }: { date: string; moments: Moment[]; isToday: boolean; onMoments: (m: Moment[]) => void }) {
  const { toast } = useToast()
  const { t } = useI18n()
  const aiOn = useAI()
  const [text, setText] = useState('')
  const [kind, setKind] = useState('note')
  const [time, setTime] = useState('')
  const [all, setAll] = useState(false)
  const sorted = useMemo(() => {
    const timed = moments.filter(m => m.time).sort((a, b) => a.time.localeCompare(b.time))
    return [...timed, ...moments.filter(m => !m.time)]
  }, [moments])
  const add = async (e: { preventDefault(): void }) => {
    e.preventDefault()
    if (!text.trim()) return
    try {
      const body: Partial<Moment> = { date, text, kind }
      if (time) body.time = time
      else if (!isToday) body.time = ''
      const m = await api.post<Moment>(`${V1}/moments`, body)
      onMoments([...moments, m])
      setText(''); setTime('')
    } catch (e) { toast((e as Error).message, 'err') }
  }
  const del = async (id: number) => { try { await api.del(`${V1}/moments/${id}`); onMoments(moments.filter(m => m.id !== id)) } catch (e) { toast((e as Error).message, 'err') } }
  const edit = async (m: Moment, t: string) => {
    if (!t.trim() || t === m.text) return
    try { const r = await api.put<Moment>(`${V1}/moments/${m.id}`, { text: t }); onMoments(moments.map(x => x.id === m.id ? r : x)) } catch (e) { toast((e as Error).message, 'err') }
  }
  const limit = 14
  const shown = all ? sorted : sorted.slice(0, limit)
  const row = (mo: Moment) => (
    <div key={mo.id} className="tl-item tl-moment">
      <span className="tl-time">{mo.time || ''}</span>
      <span className="tl-dot">{g(KIND_GLYPH[mo.kind] || 'dot')}</span>
      <div className="tl-body"><span className="tl-title" contentEditable suppressContentEditableWarning onBlur={e => edit(mo, e.currentTarget.textContent || '')} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); (e.target as HTMLElement).blur() } }}>{mo.text}</span></div>
      <button type="button" className="btn ghost icon-only sm tl-del" onClick={() => del(mo.id)} title={t('tl.delete')} aria-label={t('tl.delete')}><Icon name="x" size={14} /></button>
    </div>
  )
  return (
    <Card title={<><Icon name="clock" size={16} />{t('tl.the_day')}</>} className="mb timeline-card" actions={<span className="muted small">{sorted.length ? t('tl.things', { n: sorted.length }) : ''}</span>}>
      {sorted.length === 0 && <p className="muted small" style={{ margin: '0 0 12px' }}>{isToday ? t('tl.empty_today') : t('tl.empty_other')}</p>}
      {sorted.length > 0 && <div className="tl">{shown.map(row)}</div>}
      {sorted.length > limit && <button type="button" className="btn ghost sm tl-more" onClick={() => setAll(a => !a)}>{all ? t('tl.show_less') : t('tl.show_all', { n: sorted.length })}</button>}
      <form onSubmit={add} className="moment-add">
        <Segmented size="sm" value={kind} onChange={setKind} options={Object.keys(KIND_ICON).map(k => ({ value: k, label: <Icon name={KIND_GLYPH[k] || 'dot'} size={15} />, title: t(`kind.${k}`) }))} />
        <input name="moment" value={text} onChange={e => setText(e.target.value)} aria-label={t('palette.moment')} placeholder={isToday ? t('tl.moment_ph_today') : t('tl.moment_ph_other')} />
        <input type="time" value={time} onChange={e => setTime(e.target.value)} title={t('tl.time_ph')} className="time-input" />
        {aiOn && <DictateButton small onText={txt => setText(x => (x ? x + ' ' : '') + txt)} />}
        <button type="submit" className="btn primary sm" disabled={!text.trim()}><Icon name="plus" size={14} />{t('tl.add')}</button>
      </form>
    </Card>
  )
}
