// Posts: 浮世 — the floating world. Days people release into the light.
// All / Following tabs, search, and the public feed of public days.
import { useMemo, useState } from 'react'
import { listFeed } from '../lib/mem-api'
import { useI18n } from '../lib/i18n'
import { Card, Empty, Loading, PageHeader, Segmented, useAsync, useDebounce } from '../components/ui'
import { Icon } from '../components/icons'
import { Ticker } from '../lib/fx'
import PostCard from '../components/PostCard'

export default function Posts() {
  const { t } = useI18n()
  const [tab, setTab] = useState<'all' | 'follows'>('all')
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const { data, loading, error } = useAsync(() => listFeed(tab === 'follows'), [tab])
  const tabs: { value: 'all' | 'follows'; label: string }[] = [
    { value: 'all', label: t('social.filter_all') },
    { value: 'follows', label: t('social.filter_follows') },
  ]
  const list = useMemo(() => {
    if (!data) return []
    if (!dq.trim()) return data.posts
    const s = dq.toLowerCase()
    return data.posts.filter((p) => (p.title + ' ' + p.text + ' ' + p.date + ' ' + p.author).toLowerCase().includes(s))
  }, [data, dq])

  return (
    <>
      <PageHeader
        title={t('nav.posts')}
        sub={t('social.intro')}
        kanji="浮"
        actions={<Segmented options={tabs} value={tab} onChange={setTab} />}
      />
      <Ticker items={[{ t: '浮世', hot: true }, { t: t('nav.posts') }, { t: '帖', hot: true }, { t: '浮世絵' }, { t: '間', hot: true }, { t: '残' }, { t: 'PUBLIC DAYS' }]} />
      {dq && (
        <div className="toolbar">
          <div className="search grow"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder={t('j.search')} aria-label={t('j.search')} /></div>
        </div>
      )}
      {loading && !data && <Loading />}
      {error && <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>}
      {data && data.posts.length === 0 && tab === 'all' && (
        <Card>
          <Empty icon="globe" title={t('social.empty')}>
            <p>{t('social.intro')}</p>
          </Empty>
        </Card>
      )}
      {data && data.posts.length === 0 && tab === 'follows' && (
        <Card><Empty icon="users" title={t('social.empty_follows')} /></Card>
      )}
      {data && data.posts.length > 0 && list.length === 0 && (
        <Card><Empty icon="search" title={t('j.no_match_title')} /></Card>
      )}
      <div className="feed-grid">
        {list.map((p) => <PostCard key={p.id} post={p} />)}
      </div>
    </>
  )
}
