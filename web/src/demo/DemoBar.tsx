import { useState } from 'react';
import { useDemo } from '../app/ApiContext';
import { useI18n } from '../i18n/useI18n';
import { formatDateTime } from '../lib/format';
import styles from './DemoBar.module.css';
import { useVisibleSlots } from './visibleSlots';

/** A slim strip that says the demo runs in the browser and offers its two tricks. */
export function DemoBar() {
  const demo = useDemo();
  const visible = useVisibleSlots();
  const { t, locale } = useI18n();
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (!demo) return null;

  async function someoneElseBooks() {
    if (!demo || !visible || visible.starts.length === 0) {
      setMessage(t('demo.someoneElseNone'));
      return;
    }
    const start = visible.starts[Math.floor(Math.random() * visible.starts.length)]!;
    setBusy(true);
    try {
      await demo.bookForSomeoneElse({ serviceId: visible.serviceId, staffId: visible.staffId, start });
      setMessage(t('demo.someoneElseDone', { time: formatDateTime(start, visible.zone, locale) }));
    } catch {
      setMessage(t('common.error.generic'));
    } finally {
      setBusy(false);
    }
  }

  async function reset() {
    setBusy(true);
    await demo?.reset();
    window.location.hash = '#/';
    window.location.reload();
  }

  return (
    <aside className={styles.bar} aria-label={t('demo.label')}>
      <div className={styles.inner}>
        <span className={styles.label}>
          <span className={styles.tag}>{t('demo.label')}</span>
          <span>{t('demo.note')}</span>
        </span>
        <span className={styles.actions}>
          <button type="button" className={styles.action} onClick={someoneElseBooks} disabled={busy}>
            {t('demo.someoneElse')}
          </button>
          <button type="button" className={styles.action} onClick={reset} disabled={busy}>
            {t('demo.reset')}
          </button>
        </span>
        {message && (
          <p className={styles.message} role="status">
            {message}
          </p>
        )}
      </div>
    </aside>
  );
}
