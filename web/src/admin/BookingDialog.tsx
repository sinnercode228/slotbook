import { useEffect, useRef, useState } from 'react';
import type { AdminApi } from '../api/client';
import type { Booking, Service, Staff } from '../api/types';
import { CloseIcon } from '../components/icons';
import { Badge, Button, Notice } from '../components/ui';
import { useErrorMessage } from '../components/useErrorMessage';
import { useI18n } from '../i18n/useI18n';
import { formatDateTime, formatDay, formatMoney, formatTime } from '../lib/format';
import styles from './Admin.module.css';

interface BookingDialogProps {
  booking: Booking;
  zone: string;
  staff: Staff[];
  services: Service[];
  admin: AdminApi;
  onClose: () => void;
  onChanged: () => void;
}

const TONE = { confirmed: 'accent', held: 'info', cancelled: 'danger', expired: undefined } as const;

export function BookingDialog({ booking: b, zone, staff, services, admin, onClose, onChanged }: BookingDialogProps) {
  const { t, locale } = useI18n();
  const errorMessage = useErrorMessage();
  const dialog = useRef<HTMLDialogElement>(null);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // No close() on cleanup: removing the element ends the modal state, and a
  // close event would report a dismissal the user never made.
  useEffect(() => {
    const node = dialog.current;
    if (node && !node.open) node.showModal();
  }, []);

  const isBlock = b.kind === 'block';
  const service = services.find((s) => s.id === b.service_id);
  const member = staff.find((s) => s.id === b.staff_id);

  async function act() {
    setBusy(true);
    setError(null);
    try {
      if (isBlock) await admin.deleteBlock(b.id);
      else await admin.cancelBooking(b.id);
      onChanged();
      onClose();
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      className={styles.dialog}
      aria-labelledby="booking-dialog-title"
      onClose={onClose}
      onClick={(e) => e.target === dialog.current && onClose()}
    >
      <div className={styles.dialogInner}>
        <div className={styles.dialogHead}>
          <h2 id="booking-dialog-title" className={styles.dialogTitle}>
            {isBlock ? t('admin.blocked') : (b.customer?.name ?? t('admin.legend.held'))}
            <span>
              <Badge tone={isBlock ? undefined : TONE[b.status]}>
                {isBlock ? t('admin.legend.block') : t(`manage.status.${b.status}`)}
              </Badge>
            </span>
          </h2>
          <Button variant="ghost" size="small" onClick={onClose} aria-label={t('common.close')}>
            <CloseIcon />
          </Button>
        </div>

        <dl className={styles.facts}>
          <dt>{t('admin.details.time')}</dt>
          <dd>
            {formatDay(b.start, zone, locale)},{' '}
            <span className="nowrap">
              {formatTime(b.start, zone, locale)}–{formatTime(b.end, zone, locale)}
            </span>
            {b.busy_until !== b.end && <small>{t('admin.details.buffer', { time: formatTime(b.busy_until, zone, locale) })}</small>}
          </dd>
          <dt>{t('summary.specialist')}</dt>
          <dd>{member?.name ?? '—'}</dd>
          {service && (
            <>
              <dt>{t('admin.details.service')}</dt>
              <dd>{service.name}</dd>
            </>
          )}
          {b.price && (
            <>
              <dt>{t('admin.details.price')}</dt>
              <dd>{formatMoney(b.price, locale)}</dd>
            </>
          )}
          {b.customer && (
            <>
              <dt>{t('admin.details.phone')}</dt>
              <dd>
                <a href={`tel:${b.customer.phone.replace(/[^\d+]/g, '')}`}>{b.customer.phone}</a>
              </dd>
              {b.customer.email && (
                <>
                  <dt>{t('admin.details.email')}</dt>
                  <dd>
                    <a href={`mailto:${b.customer.email}`}>{b.customer.email}</a>
                  </dd>
                </>
              )}
            </>
          )}
          {b.status === 'held' && b.hold_expires_at && (
            <>
              <dt>{t('admin.details.status')}</dt>
              <dd>{t('admin.details.holdUntil', { time: formatTime(b.hold_expires_at, zone, locale) })}</dd>
            </>
          )}
          {b.note && (
            <>
              <dt>{t('admin.details.note')}</dt>
              <dd>{b.note}</dd>
            </>
          )}
          <dt>{t('admin.details.reference')}</dt>
          <dd>
            <code>{b.id.slice(0, 8)}</code>
            <small>{formatDateTime(b.created_at, zone, locale)}</small>
          </dd>
        </dl>

        {error && <Notice tone="danger">{error}</Notice>}

        {isBlock && (
          <div className={styles.actions}>
            <Button variant="danger" busy={busy} onClick={act}>
              {t('admin.details.remove')}
            </Button>
          </div>
        )}
        {!isBlock && b.status === 'confirmed' && !confirming && (
          <div className={styles.actions}>
            <Button variant="danger" onClick={() => setConfirming(true)}>
              {t('admin.details.cancel')}
            </Button>
          </div>
        )}
        {!isBlock && confirming && (
          <Notice
            tone="warning"
            title={t('admin.details.cancelConfirm')}
            action={
              <div className={styles.actions}>
                <Button variant="danger" size="small" busy={busy} onClick={act}>
                  {t('manage.cancelYes')}
                </Button>
                <Button variant="secondary" size="small" disabled={busy} onClick={() => setConfirming(false)}>
                  {t('manage.cancelNo')}
                </Button>
              </div>
            }
          />
        )}
      </div>
    </dialog>
  );
}
