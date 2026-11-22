import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { fmtDate, fmtDateTime } from '../lib/format'
import { useI18n } from '../lib/i18n'
import type { Ask as AskT } from '../lib/types'
import { Card, Confirm, Empty, Loading, PageHeader, useAsync, useToast } from '../components/ui'
import { CitedText, DictateButton, useAI } from '../components/ai'
import { Icon } from '../components/icons'

const EXAMPLE_KEYS = ['a.ex1', 'a.ex2', 'a.ex3', 'a.ex4', 'a.ex5', 'a.ex6']

export default function Ask() {
  const aiOn = useAI()
  const { toast } = useToast()
  const { t } = useI18n()
  const EXAMPLES = EXAMPLE_KEYS.map(k => t(k))
  const { data, setData, loading } = useAsync(() => api.get<AskT[]>(`${V1}/ai/asks`), [])
  const [q, setQ] = useState('')
  const [busy, setBusy] = useState(false)
  const [clear, setClear] = useState(false)
  const ask = async (question: string) => {
    if (!question.trim()) return
    setBusy(true)
    try { const a = await api.post<AskT>(`${V1}/ai/asks`, { question }); setData(d => [a, ...(d || [])]); setQ('') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const del = async (id: number) => { await api.del(`${V1}/ai/asks/${id}`); setData(d => (d || []).filter(a => a.id !== id)) }
  if (!aiOn) return <><PageHeader kanji="問" title={t('a.title')} /><Card><Empty icon="sparkles" title={t('a.connect_title')}>{t('a.connect_body')}</Empty></Card></>
  return (
    <>
      <PageHeader kanji="問" title={t('a.title')} sub={t('a.sub')} actions={data && data.length > 0 ? <button type="button" className="btn ghost sm" onClick={() => setClear(true)}><Icon name="trash" size={14} />{t('a.clear')}</button> : undefined} />
      <Card className="mb">
        <form className="ask-form" onSubmit={e => { e.preventDefault(); ask(q) }}>
          <input value={q} onChange={e => setQ(e.target.value)} placeholder={t('a.ph')} autoFocus disabled={busy} aria-label={t('a.ph')} />
          <DictateButton small onText={x => setQ(x)} />
          <button type="submit" className="btn primary lg" disabled={busy || !q.trim()}><Icon name="sparkles" size={15} />{busy ? t('a.reading') : t('a.ask')}</button>
        </form>
        {(!data || data.length === 0) && <div className="chips mt">{EXAMPLES.map(x => <button type="button" key={x} className="chip" onClick={() => ask(x)} disabled={busy}>{x}</button>)}</div>}
        {busy && <p className="muted small mt-sm row"><span className="spinner" />{t('a.working')}</p>}
      </Card>
      {loading && !data && <Loading />}
      {data && data.length > 0 && <Card>
        {data.map(a => <div key={a.id} className="ask-item">
          <div className="row"><div className="q grow">{a.question}</div><button type="button" className="btn ghost icon-only sm" onClick={() => ask(a.question)} disabled={busy} title={t('a.again')} aria-label={t('a.again')}><Icon name="refresh" size={14} /></button><button type="button" className="btn ghost icon-only sm" onClick={() => del(a.id)} title={t('a.forget')} aria-label={t('a.forget')}><Icon name="x" size={14} /></button></div>
          <div className="a"><CitedText text={a.answer} /></div>
          {a.dates.length > 0 && <div className="chips">{a.dates.map(d => <Link key={d} className="chip sm" to={`/day/${d}`}>{fmtDate(d)}</Link>)}</div>}
          <div className="muted small mt-sm">{t('a.read', { scope: a.scope, time: fmtDateTime(a.created_at) })}</div>
        </div>)}
      </Card>}
      {clear && <Confirm title={t('a.clear_title')} message={t('a.clear_msg')} onCancel={() => setClear(false)} onConfirm={async () => { setClear(false); await api.del(`${V1}/ai/asks/0`); setData([]) }} />}
    </>
  )
}
