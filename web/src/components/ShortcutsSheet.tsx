import { Modal } from './ui'
import { useI18n } from '../lib/i18n'

export function ShortcutsSheet({ onClose }: { onClose: () => void }) {
  const { t } = useI18n()
  const GROUPS: { title: string; keys: [string[], string][] }[] = [
    { title: t('sc.goto'), keys: [[['t'], t('sc.today')], [['c'], t('sc.calendar_g')], [['j'], t('sc.journal')], [['y'], t('sc.year')], [['a'], t('sc.ask')], [[','], t('sc.settings')], [['/'], t('sc.search')], [['⌘', 'K'], t('sc.search')]] },
    { title: t('sc.write'), keys: [[['n'], t('sc.todays')], [['m'], t('sc.moment')]] },
    { title: t('sc.onday'), keys: [[['←'], t('sc.prev_day')], [['→'], t('sc.next_day')]] },
    { title: t('sc.calendar'), keys: [[['⇧', '←'], t('sc.prev_month')], [['⇧', '→'], t('sc.next_month')], [['←', '→'], t('sc.move_day')], [['↵'], t('sc.open_day')]] },
    { title: t('sc.window'), keys: [[['['], t('sc.sidebar')], [['esc'], t('sc.close')]] },
    { title: t('sc.help'), keys: [[['?'], t('sc.this_sheet')]] },
  ]
  return (
    <Modal title={t('sc.title')} onClose={onClose} size="lg">
      <div className="shortcuts">
        {GROUPS.map(g => (
          <section key={g.title}>
            <h4 className="sc-h">{g.title}</h4>
            {g.keys.map(([keys, label]) => (
              <div key={label + keys.join('')} className="sc-row"><span>{label}</span><span className="sc-keys">{keys.map(k => <kbd key={k}>{k}</kbd>)}</span></div>
            ))}
          </section>
        ))}
      </div>
      <p className="muted small mt">{t('sc.tip')}</p>
    </Modal>
  )
}
