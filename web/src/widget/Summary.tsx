import type { ReactNode } from 'react';
import type { Location, Service } from '../api/types';
import { useI18n } from '../i18n/useI18n';
import { formatDateTime, formatMoney, formatTime } from '../lib/format';
import styles from './Widget.module.css';

export type EditableStep = 'location' | 'service' | 'staff' | 'time';

export interface SummaryProps {
  location: Location | null;
  service: Service | null;
  /** undefined: not chosen yet; null: any available specialist. */
  staffName: string | null | undefined;
  start: string | null;
  visitorZone: string | null;
}

function useSummary({ location, service, staffName, start }: SummaryProps) {
  const { t, locale } = useI18n();
  return {
    staff: staffName === undefined ? null : (staffName ?? t('staff.any')),
    when: start && location ? formatDateTime(start, location.time_zone, locale) : null,
    price: service ? formatMoney(service.price, locale) : null,
  };
}

/** Chips above the steps on narrow screens; each one jumps back to its step. */
export function SummaryBar(props: SummaryProps & { onEdit: (step: EditableStep) => void }) {
  const { t } = useI18n();
  const { staff, when } = useSummary(props);
  const chips: { step: EditableStep; label: string }[] = [];
  if (props.location) chips.push({ step: 'location', label: props.location.name });
  if (props.service) chips.push({ step: 'service', label: props.service.name });
  if (staff) chips.push({ step: 'staff', label: staff });
  if (when) chips.push({ step: 'time', label: when });
  if (chips.length === 0) return null;
  return (
    <ul className={styles.summaryBar} aria-label={t('summary.title')}>
      {chips.map((chip) => (
        <li key={chip.step} className={styles.chipItem}>
          <button type="button" className={styles.chip} onClick={() => props.onEdit(chip.step)}>
            {chip.label}
          </button>
        </li>
      ))}
    </ul>
  );
}

/** The booking so far, beside the steps on wide screens. */
export function SummaryCard(props: SummaryProps) {
  const { t, locale } = useI18n();
  const { staff, when, price } = useSummary(props);
  const { location, service, start, visitorZone } = props;
  return (
    <aside className={styles.summaryCard} aria-label={t('summary.title')}>
      <h2 className={styles.summaryTitle}>{t('summary.title')}</h2>
      <dl className={styles.summaryList}>
        <Row label={t('summary.salon')}>
          {location && (
            <>
              {location.name}
              <small>{location.address}</small>
            </>
          )}
        </Row>
        <Row label={t('summary.service')}>
          {service && (
            <>
              {service.name}
              <small>{t('common.minutes', { n: service.duration_minutes })}</small>
            </>
          )}
        </Row>
        <Row label={t('summary.specialist')}>{staff}</Row>
        <Row label={t('summary.time')}>
          {when && start && (
            <>
              {when}
              {visitorZone && <small>{t('summary.yourTime', { time: formatTime(start, visitorZone, locale) })}</small>}
            </>
          )}
        </Row>
        <Row label={t('summary.price')}>{price}</Row>
      </dl>
    </aside>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className={styles.summaryRow}>
      <dt>{label}</dt>
      <dd>{children || <span className={styles.summaryPlaceholder}>—</span>}</dd>
    </div>
  );
}
