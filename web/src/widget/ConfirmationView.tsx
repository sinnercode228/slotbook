import { useState } from 'react';
import { absoluteUrl, manageRoute } from '../app/router';
import type { Confirmation, Location, Service } from '../api/types';
import { CalendarIcon, Check, CopyIcon } from '../components/icons';
import { buttonClass } from '../components/buttonClass';
import { Button } from '../components/ui';
import { useI18n } from '../i18n/useI18n';
import { formatDay, formatMoney, formatTime } from '../lib/format';
import { downloadIcs } from '../lib/ics';
import styles from './Widget.module.css';

interface ConfirmationViewProps {
  confirmation: Confirmation;
  location: Location;
  service: Service;
  staffName: string;
  visitorZone: string | null;
  onBookAnother: () => void;
}

export function ConfirmationView({ confirmation, location, service, staffName, visitorZone, onBookAnother }: ConfirmationViewProps) {
  const { t, locale } = useI18n();
  const [copied, setCopied] = useState(false);
  const { booking, manage_token: manageToken } = confirmation;
  const zone = location.time_zone;
  const managePath = manageRoute(booking.id, manageToken);
  const manageUrl = absoluteUrl(managePath);

  function addToCalendar() {
    downloadIcs(
      {
        uid: booking.id,
        start: booking.start,
        end: booking.end,
        summary: `${service.name}, ${staffName}`,
        location: `${location.name}, ${location.address}`,
        description: `${t('done.manage')}: ${manageUrl}`,
      },
      `booking-${booking.start.slice(0, 10)}.ics`,
    );
  }

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(manageUrl);
      setCopied(true);
    } catch {
      // Clipboard access denied: the link stays visible to copy by hand.
    }
  }

  return (
    <section className={styles.done} aria-labelledby="booking-done-title">
      <div className={styles.doneHead}>
        <span className={styles.doneIcon} aria-hidden="true">
          <Check />
        </span>
        <h2 id="booking-done-title" className={styles.doneTitle} tabIndex={-1}>
          {t('done.title')}
        </h2>
        <p className={styles.lead}>
          {t('done.lead', { date: formatDay(booking.start, zone, locale), time: formatTime(booking.start, zone, locale) })}
        </p>
      </div>

      <dl className={styles.facts}>
        <dt>{t('summary.service')}</dt>
        <dd>
          {service.name}
          <small>{t('common.minutes', { n: service.duration_minutes })}</small>
        </dd>
        <dt>{t('summary.specialist')}</dt>
        <dd>{staffName}</dd>
        <dt>{t('summary.time')}</dt>
        <dd>
          {formatDay(booking.start, zone, locale)},{' '}
          <span className="nowrap">
            {formatTime(booking.start, zone, locale)}–{formatTime(booking.end, zone, locale)}
          </span>
          {visitorZone && <small>{t('summary.yourTime', { time: formatTime(booking.start, visitorZone, locale) })}</small>}
        </dd>
        <dt>{t('summary.salon')}</dt>
        <dd>
          {location.name}
          <small>{location.address}</small>
        </dd>
        {booking.price && (
          <>
            <dt>{t('summary.price')}</dt>
            <dd>{formatMoney(booking.price, locale)}</dd>
          </>
        )}
      </dl>

      <div className={styles.actions}>
        <Button onClick={addToCalendar}>
          <CalendarIcon />
          {t('done.addToCalendar')}
        </Button>
        <a className={buttonClass('secondary')} href={`#${managePath}`}>
          {t('done.manage')}
        </a>
      </div>

      <div className={styles.linkBox}>
        <p className={styles.hint}>{t('done.manageHint')}</p>
        <div className={styles.linkRow}>
          <span className={styles.linkText}>{manageUrl}</span>
          <Button variant="ghost" size="small" onClick={copyLink} aria-live="polite">
            {copied ? <Check /> : <CopyIcon />}
            {copied ? t('done.copied') : t('done.copyLink')}
          </Button>
        </div>
      </div>

      <div>
        <Button variant="ghost" onClick={onBookAnother}>
          {t('done.bookAnother')}
        </Button>
      </div>
    </section>
  );
}
