// People: the wanderers of the floating world. Who released a day, who
// follows whom, and the latest day each one hung in the light.
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { followPerson, listPeople, unfollowPerson } from '../lib/mem-api'
import { type Person } from '../lib/types'
import { useI18n } from '../lib/i18n'
import { Card, Empty, Loading, PageHeader, errMsg, useAsync, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Ticker } from '../lib/fx'

export default function People() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data, loading, error } = useAsync(() => listPeople(), [])
  const [follows, setFollows] = useState<Record<number, boolean>>({})
  const [busy, setBusy] = useState(0)

  const isF = (p: Person) => follows[p.id] ?? p.i_follow

  const flip = (p: Person) => {
    if (p.id === data?.me || busy) return
    const target = !isF(p)
    setFollows((f) => ({ ...f, [p.id]: target }))
    setBusy(p.id)
    const req = target ? followPerson(p.id) : unfollowPerson(p.id)
    req.catch((err) => {
      setFollows((f) => ({ ...f, [p.id]: !target }))
      toast(errMsg(err))
    }).finally(() => setBusy(0))
  }

  return (
    <>
      <PageHeader title={t('nav.people')} sub={t('social.intro')} kanji="人" />
      <Ticker items={[{ t: '人', hot: true }, { t: t('nav.people') }, { t: '浮世', hot: true }, { t: '追う' }, { t: '残' }, { t: 'WANDERERS' }]} />
      {loading && !data && <Loading />}
      {error && <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>}
      {data && data.people.length === 0 && (
        <Card><Empty icon="users" title={t('social.empty')} /></Card>
      )}
      {data && data.people.length > 0 && (
        <div className="people-grid">
          {data.people.map((p) => (
            <button key={p.id} type="button" className="person-card" onClick={() => navigate(`/people/${p.id}`)}>
              <span className="person-top">
                <span className="person-seal">{p.name.slice(0, 1).toUpperCase()}</span>
                <span className="person-name">
                  <span>{p.name}</span>
                  {p.id === data.me && <span className="person-you">{t('social.you')}</span>}
                </span>
              </span>
              <span className="person-stats">
                <span><b>{p.public_posts}</b> {t('social.days')}</span>
                <span><b>{p.followers}</b> {t('social.followers')}</span>
                <span><b>{p.following}</b> {t('social.following')}</span>
              </span>
              <span className="person-latest">
                {p.latest ? (
                  <>
                    <span className="pl-dot" />
                    <span>{p.latest.title || p.latest.date}</span>
                  </>
                ) : (
                  <span>{t('social.no_latest')}</span>
                )}
              </span>
              {p.id !== data.me && (
                <span
                  className={`person-follow${isF(p) ? ' on' : ''}`}
                  role="button"
                  tabIndex={0}
                  onClick={e => { e.stopPropagation(); flip(p) }}
                  onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); flip(p) } }}
                >
                  {isF(p) ? t('social.unfollow') : t('social.follow')}
                </span>
              )}
            </button>
          ))}
        </div>
      )}
    </>
  )
}
