import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { addDays, fmtDateTime, todayISO } from '../lib/format'
import { useI18n } from '../lib/i18n'
import type { Story } from '../lib/types'
import { Card, Empty, PageHeader, useAsync } from '../components/ui'
import { StoryCard, useAI } from '../components/ai'
import { Icon } from '../components/icons'

function isoWeek(iso: string): string {
  const [y, m, d] = iso.split('-').map(Number)
  const t = new Date(Date.UTC(y, m - 1, d))
  const day = t.getUTCDay() || 7
  t.setUTCDate(t.getUTCDate() + 4 - day)
  const y0 = new Date(Date.UTC(t.getUTCFullYear(), 0, 1))
  const w = Math.ceil(((t.getTime() - y0.getTime()) / 86400000 + 1) / 7)
  return `${t.getUTCFullYear()}-W${String(w).padStart(2, '0')}`
}
const KIND_LABEL: Record<string, string> = { week: 'Week', month: 'Month', year: 'Year' }

export default function Stories() {
  const aiOn = useAI()
  const { t } = useI18n()
  const [sp, setSp] = useSearchParams()
  const today = todayISO()
  const choices = useMemo(() => {
    const lastMonth = new Date(Number(today.slice(0, 4)), Number(today.slice(5, 7)) - 2, 1)
    const lm = `${lastMonth.getFullYear()}-${String(lastMonth.getMonth() + 1).padStart(2, '0')}`
    return [
      { kind: 'week' as const, period: isoWeek(today), label: t('s.this_week') },
      { kind: 'week' as const, period: isoWeek(addDays(today, -7)), label: t('s.last_week') },
      { kind: 'month' as const, period: today.slice(0, 7), label: t('s.this_month') },
      { kind: 'month' as const, period: lm, label: t('s.last_month') },
      { kind: 'year' as const, period: today.slice(0, 4), label: t('s.this_year') },
    ]
  }, [today, t])
  const kind = (sp.get('kind') as 'week' | 'month' | 'year') || 'week'
  const period = sp.get('period') || choices[0].period
  const label = choices.find(c => c.kind === kind && c.period === period)?.label || `${t(`s.${kind}` as const).toLowerCase()} ${period}`
  const { data: list } = useAsync(() => api.get<Story[]>(`${V1}/ai/stories`), [kind, period])
  const [, force] = useState(0)
  if (!aiOn) return <><PageHeader kanji="話" title={t('s.title')} /><Card><Empty icon="sparkles" title={t('a.connect_title')}>{t('s.connect_body')}</Empty></Card></>
  return (
    <>
      <PageHeader kanji="話" title={t('s.title')} sub={t('s.sub')} />
      <div className="chips mb">{choices.map(c => <button type="button" key={c.kind + c.period} className={`chip ${c.kind === kind && c.period === period ? 'active' : ''}`} onClick={() => setSp({ kind: c.kind, period: c.period })}>{c.label[0].toUpperCase() + c.label.slice(1)} <span className="muted">{c.period}</span></button>)}</div>
      <StoryCard key={kind + period} kind={kind} period={period} label={label} />
      {list && list.length > 0 && <Card title={<><Icon name="book" size={16} />{t('s.written')}</>}>
        <div className="inline-list">{list.map(s => <a key={s.kind + s.period} className="li" href="#" style={{ color: 'inherit' }} onClick={e => { e.preventDefault(); setSp({ kind: s.kind, period: s.period }); force(x => x + 1); document.querySelector('.content')?.scrollTo({ top: 0 }) }}>
          <span className="badge">{t(`s.${s.kind}` as const)}</span><span className="grow"><b>{s.title}</b> <span className="muted small">{s.period}</span></span><span className="muted small">{fmtDateTime(s.created_at, false)}</span>
        </a>)}</div>
      </Card>}
    </>
  )
}
