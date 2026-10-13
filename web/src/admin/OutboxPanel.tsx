import { useEffect, useState } from 'react';
import type { AdminApi } from '../api/client';
import type { NotificationStatus } from '../api/types';
import { Badge, ErrorState, SkeletonList } from '../components/ui';
import { useI18n } from '../i18n/useI18n';
import { formatDateTime } from '../lib/format';
import { useResource } from '../lib/useResource';
import styles from './Admin.module.css';

const POLL_MS = 5_000;

const STATUSES: NotificationStatus[] = ['pending', 'sent', 'failed', 'voided', 'skipped'];

const TONE: Record<NotificationStatus, 'accent' | 'info' | 'danger' | undefined> = {
  pending: 'info',
  sent: 'accent',
  failed: 'danger',
  voided: undefined,
  skipped: undefined,
};

/** The outbox rows of a salon: what was sent, what waits for its time. */
export function OutboxPanel({ admin, locationId, zone, version }: { admin: AdminApi; locationId: number; zone: string; version: number }) {
  const { t, locale } = useI18n();
  const [status, setStatus] = useState<NotificationStatus | ''>('');
  const rows = useResource(`outbox|${locationId}|${status}`, () =>
    admin.listNotifications({ locationId, status: status || undefined, limit: 60 }),
  );
  const { reload } = rows;

  useEffect(() => {
    const id = window.setInterval(reload, POLL_MS);
    return () => window.clearInterval(id);
  }, [reload]);

  useEffect(() => {
    if (version > 0) reload();
  }, [version, reload]);

  return (
    <section className={styles.panel} aria-labelledby="outbox-title">
      <div className={styles.panelHead}>
        <h2 id="outbox-title" className={styles.panelTitle}>
          {t('admin.outbox.title')}
        </h2>
        <label className={styles.inlineField}>
          <span className="visually-hidden">{t('admin.outbox.filter')}</span>
          <select
            className={`${styles.select} ${styles.compactSelect}`}
            value={status}
            onChange={(e) => setStatus(e.target.value as NotificationStatus | '')}
          >
            <option value="">{t('admin.outbox.all')}</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {t(`admin.outbox.status.${s}`)}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p className={styles.muted}>{t('admin.outbox.lead')}</p>
      {rows.data === undefined ? (
        rows.error !== undefined ? (
          <ErrorState error={rows.error} onRetry={reload} />
        ) : (
          <SkeletonList rows={3} label={t('common.loading')} />
        )
      ) : rows.data.length === 0 ? (
        <p className={styles.muted}>{t('admin.outbox.empty')}</p>
      ) : (
        <ul className={styles.outbox}>
          {rows.data.map((n) => (
            <li key={n.id} className={styles.outboxRow}>
              <div className={styles.outboxTop}>
                <span className={styles.outboxKind}>{t(`admin.outbox.kind.${n.kind}`)}</span>
                <Badge tone={TONE[n.status]}>{t(`admin.outbox.status.${n.status}`)}</Badge>
              </div>
              <span className={styles.outboxMeta}>
                {n.sent_at
                  ? t('admin.outbox.sent', { time: formatDateTime(n.sent_at, zone, locale) })
                  : t('admin.outbox.due', { time: formatDateTime(n.send_at, zone, locale) })}
                {' · '}
                {t('admin.outbox.booking', { ref: n.booking_id.slice(0, 8) })}
                {n.attempts > 1 && ` · ${t('admin.outbox.attempts', { n: n.attempts })}`}
              </span>
              {n.last_error && n.status !== 'sent' && (
                <span className={n.status === 'failed' ? styles.outboxError : styles.outboxMeta}>{n.last_error}</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
