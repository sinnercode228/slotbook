import { useEffect } from 'react';
import { Clock } from '../components/icons';
import { useI18n } from '../i18n/useI18n';
import { formatDateTime, formatTime } from '../lib/format';
import { useNow } from '../lib/useNow';
import styles from './Widget.module.css';

const URGENT_MS = 60_000;

function mmss(ms: number): string {
  const seconds = Math.ceil(ms / 1000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
}

interface HoldTimerProps {
  start: string;
  zone: string;
  /** The visitor's zone when it differs from the salon's. */
  visitorZone: string | null;
  /** Local clock time when the hold runs out. */
  deadline: number;
  /** The full length of the hold, for the progress bar. */
  totalMs: number;
  onExpire: () => void;
}

/** The reserved time and how long the reservation still runs. */
export function HoldTimer({ start, zone, visitorZone, deadline, totalMs, onExpire }: HoldTimerProps) {
  const { t, locale } = useI18n();
  const now = useNow();
  const remaining = Math.max(0, deadline - now);
  const expired = remaining === 0;

  useEffect(() => {
    if (expired) onExpire();
  }, [expired, onExpire]);

  return (
    <div className={`${styles.hold} ${remaining < URGENT_MS ? styles.urgent : ''}`}>
      <div className={styles.holdTop}>
        <div className={styles.holdWhen}>
          <span className={styles.holdLabel}>{t('hold.title')}</span>
          <span className={styles.holdTime}>{formatDateTime(start, zone, locale)}</span>
          {visitorZone && (
            <span className={styles.holdLocal}>{t('summary.yourTime', { time: formatTime(start, visitorZone, locale) })}</span>
          )}
        </div>
        <span className={styles.countdown} role="timer" aria-label={t('hold.remaining', { time: mmss(remaining) })}>
          <Clock />
          {mmss(remaining)}
        </span>
      </div>
      <div className={styles.holdBar} aria-hidden="true">
        <div className={styles.holdBarFill} style={{ width: `${Math.min(100, (remaining / totalMs) * 100)}%` }} />
      </div>
    </div>
  );
}
