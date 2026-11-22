// Tokonoma: a person's alcove. The hung scroll (their chosen day), the
// ikebana vase (their palette), and the shelf of public days.
import { useEffect, useState, type CSSProperties } from 'react'
import { useParams } from 'react-router-dom'
import { followPerson, getPerson, postAssetUrl, setFeatured, unfollowPerson } from '../lib/mem-api'
import { memoryArt } from '../lib/types'
import { useI18n } from '../lib/i18n'
import { Empty, Loading, PageHeader, errMsg, useAsync, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Ticker } from '../lib/fx'
import PostCard from '../components/PostCard'

export default function Tokonoma() {
  const { t } = useI18n()
  const { toast } = useToast()
  const { id } = useParams()
  const num = Number(id)
  const { data, error, reload } = useAsync(() => getPerson(num), [num])

  const [follow, setFollow] = useState(false)
  const [fbusy, setFbusy] = useState(false)

  useEffect(() => {
    if (data) setFollow(data.person.i_follow)
  }, [data])

  if (!data) {
    return error ? <Empty icon="globe" title={t('post.not_found')} /> : <Loading />
  }

  const { person, featured, posts, me } = data
  const isMe = person.id === me
  const pal = featured ? memoryArt(featured)?.palette ?? [] : []

  const flip = () => {
    if (fbusy || isMe) return
    const target = !follow
    setFbusy(true)
    setFollow(target)
    const req = target ? followPerson(person.id) : unfollowPerson(person.id)
    req.catch((err) => {
      setFollow(!target)
      toast(errMsg(err))
    }).finally(() => setFbusy(false))
  }

  const flower = (postId: number) => {
    const on = featured?.id !== postId
    setFeatured(postId, on)
      .then(() => {
        toast(on ? t('tokonoma.set') : t('tokonoma.unset'))
        reload()
      })
      .catch((err) => toast(errMsg(err)))
  }

  return (
    <>
      <PageHeader title={t('tokonoma.title')} sub={person.name} kanji="床" />
      <Ticker items={[{ t: '床', hot: true }, { t: t('tokonoma.title') }, { t: '掛軸', hot: true }, { t: '生花' }, { t: '一' }, { t: person.name }]} />
      {error && <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>}
      <div className="tokonoma">
        <div className="toko-left">
          <div className="kakejiku">
            <span className="kakejiku-rod" />
            <div className="kakejiku-scroll">
              {featured?.poster_path ? (
                <img src={postAssetUrl(featured.id, featured.poster_path)} alt="" />
              ) : (
                <span className="seal" aria-hidden>{featured ? '残' : '空'}</span>
              )}
              <span className="vt">{featured?.title || person.name}</span>
              <span className="st">浮</span>
            </div>
            <span className="kakejiku-tail" />
          </div>
          <div className="ikebana" title={t('tokonoma.flower')}>
            <span className="ikebana-flower">
              {[0, 1, 2, 3, 4].map((i) => (
                <span key={i} className="ikebana-petal" style={{ '--fa': `${i * 72}deg`, '--fp': pal[i % Math.max(pal.length, 1)] ?? '#f7a8c4' } as CSSProperties} />
              ))}
              <span className="ikebana-heart" />
            </span>
            <span className="ikebana-stem"><span className="ikebana-leaf" /></span>
            <span className="ikebana-vase" />
          </div>
        </div>
        <div className="toko-right">
          <div className="toko-info">
            <div className="toko-name">
              {person.name}
              {isMe && <span className="person-you">{t('social.you')}</span>}
            </div>
            <div className="person-stats">
              <span><b>{person.public_posts}</b> {t('social.days')}</span>
              <span><b>{person.followers}</b> {t('social.followers')}</span>
              <span><b>{person.following}</b> {t('social.following')}</span>
            </div>
            {!isMe && (
              <button type="button" className={`btn${follow ? ' primary' : ''}`} disabled={fbusy} onClick={flip}>
                {follow ? t('social.unfollow') : t('social.follow')}
              </button>
            )}
            {featured ? (
              <div className="toko-cap">{featured.title || featured.date} · {featured.date}</div>
            ) : posts.length > 0 ? (
              <div className="toko-note">{t('tokonoma.latest_fallback')}</div>
            ) : null}
          </div>
          {posts.length > 0 ? (
            <div className="toko-shelf">
              <h4>{person.name} · {t('social.days')}</h4>
              <div className="feed-grid">
                {posts.map((p) => (
                  <PostCard
                    key={p.id}
                    post={p}
                    onFlower={isMe ? { featured: featured?.id === p.id, toggle: () => flower(p.id) } : undefined}
                  />
                ))}
              </div>
            </div>
          ) : (
            <Empty icon="globe" title={t('tokonoma.empty')} />
          )}
        </div>
      </div>
    </>
  )
}
