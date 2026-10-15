import { useEffect, useState } from 'react';
import { useApi } from '../app/ApiContext';
import { manageRoute, navigate } from '../app/router';
import { errorCode } from '../api/errors';
import type { Booking, Location, Service, Staff } from '../api/types';
import { buttonClass } from '../components/buttonClass';
import { Badge, Button, ErrorState, Notice, SkeletonList } from '../components/ui';
import { useErrorMessage } from '../components/useErrorMessage';
import { useI18n } from '../i18n/useI18n';
import { formatDateTime, formatDay, formatMoney, formatTime, visitorZone as detectVisitorZone, zonesDiffer } from '../lib/format';
import { useNow } from '../lib/useNow';
import { useResource } from '../lib/useResource';
import { SlotPicker } from '../widget/SlotPicker';
import styles from './ManagePage.module.css';

interface Context {
  location: Location;
  service: Service | undefined;
  staff: Staff | undefined;
}

type Mode = 'view' | 'confirmCancel' | 'reschedule';
type Flash = { tone: 'success' | 'warning' | 'danger'; text: string } | null;

const STATUS_TONE = { confirmed: 'accent', held: 'warning', cancelled: 'danger', expired: undefined, released: undefined } as const;

export function ManagePage({ bookingId, manageToken, notice }: { bookingId: string; manageToken: string; notice?: string | null }) {
  const api = useApi();
  const { t } = useI18n();
  const booking = useResource(`booking|${bookingId}`, () => api.getBooking(bookingId, manageToken));
  // Kept from the first render: the URL loses the notice right away.
  const [arrivalNotice] = useState(notice);

  // The notice is for this visit only; a reload of the page should not repeat it.
  useEffect(() => {
    if (arrivalNotice) window.history.replaceState(null, '', `#${manageRoute(bookingId, manageToken)}`);
  }, [arrivalNotice, bookingId, manageToken]);

  // A link kept from before a reschedule leads to the cancelled original;
  // the booking that replaced it carries the same token.
  const movedTo = booking.data?.rescheduled_to;
  useEffect(() => {
    if (movedTo) navigate(manageRoute(movedTo, manageToken), { replace: true });
  }, [movedTo, manageToken]);
  const locationId = booking.data?.location_id;
  const context = useResource<Context>(locationId === undefined ? null : `context|${locationId}`, async () => {
    const [locations, services, staff] = await Promise.all([
      api.listLocations(),
      api.listServices(locationId!),
      api.listStaff(locationId!),
    ]);
    const b = booking.data!;
    return {
      location: locations.find((l) => l.id === b.location_id)!,
      service: services.find((s) => s.id === b.service_id),
      staff: staff.find((s) => s.id === b.staff_id),
    };
  });

  if (booking.error !== undefined && !booking.data) {
    return (
      <div className={styles.page}>
        {errorCode(booking.error) === 'not_found' ? (
          <Notice tone="warning" action={<a className={buttonClass('secondary', 'small')} href="#/">{t('manage.bookAgain')}</a>}>
            {t('manage.notFound')}
          </Notice>
        ) : (
          <ErrorState error={booking.error} onRetry={booking.reload} />
        )}
      </div>
    );
  }
  if (context.error !== undefined && !context.data) {
    return (
      <div className={styles.page}>
        <ErrorState error={context.error} onRetry={context.reload} />
      </div>
    );
  }
  if (!booking.data || !context.data || movedTo) {
    return (
      <div className={styles.page}>
        <SkeletonList rows={3} label={t('common.loading')} />
      </div>
    );
  }
  return (
    <BookingDetails
      booking={booking.data}
      context={context.data}
      manageToken={manageToken}
      initialFlash={arrivalNotice === 'moved' ? 'moved' : null}
      onChanged={booking.reload}
    />
  );
}

function BookingDetails({
  booking,
  context,
  manageToken,
  initialFlash,
  onChanged,
}: {
  booking: Booking;
  context: Context;
  manageToken: string;
  initialFlash: 'moved' | null;
  onChanged: () => void;
}) {
  const api = useApi();
  const { t, locale } = useI18n();
  const errorMessage = useErrorMessage();
  const now = useNow(30_000);
  const [ownZone] = useState(detectVisitorZone);
  const [mode, setMode] = useState<Mode>('view');
  const [busy, setBusy] = useState(false);
  const [pendingStart, setPendingStart] = useState<string | null>(null);
  const [hidden, setHidden] = useState<string[]>([]);
  const [refreshKey, setRefreshKey] = useState(0);
  const [pickNotice, setPickNotice] = useState<string | null>(null);
  const [justCancelled, setJustCancelled] = useState(false);
  const zone = context.location.time_zone;
  const [flash, setFlash] = useState<Flash>(() =>
    initialFlash === 'moved'
      ? { tone: 'success', text: t('manage.moved', { date: formatDay(booking.start, zone, locale), time: formatTime(booking.start, zone, locale) }) }
      : null,
  );

  const start = Date.parse(booking.start);
  const cutoff = start - context.location.settings.cancel_cutoff_minutes * 60_000;
  const confirmed = booking.status === 'confirmed';
  const changeable = confirmed && now <= cutoff;
  const ownTime = zonesDiffer(zone, ownZone, start) ? formatTime(booking.start, ownZone, locale) : null;

  async function cancel() {
    setBusy(true);
    try {
      await api.cancelBooking(booking.id, manageToken);
      setJustCancelled(true);
      setFlash(null);
      setMode('view');
      onChanged();
    } catch (error) {
      setMode('view');
      if (errorCode(error) === 'too_late' || errorCode(error) === 'not_active') onChanged();
      setFlash({ tone: 'danger', text: errorCode(error) === 'too_late' ? t('manage.tooLate', { time: formatDateTime(cutoff, zone, locale) }) : errorMessage(error) });
    } finally {
      setBusy(false);
    }
  }

  async function reschedule(newStart: string) {
    setPendingStart(newStart);
    setPickNotice(null);
    try {
      const moved = await api.rescheduleBooking(booking.id, manageToken, newStart);
      navigate(`${manageRoute(moved.id, manageToken)}&notice=moved`, { replace: true });
    } catch (error) {
      const code = errorCode(error);
      if (code === 'slot_taken' || code === 'invalid_slot') {
        setHidden((h) => [...h, newStart]);
        setRefreshKey((n) => n + 1);
        setPickNotice(code === 'slot_taken' ? t('slots.taken', { time: formatTime(newStart, zone, locale) }) : t('slots.invalid'));
      } else if (code === 'too_late' || code === 'not_active') {
        setMode('view');
        onChanged();
        setFlash({ tone: 'danger', text: t('manage.tooLate', { time: formatDateTime(cutoff, zone, locale) }) });
      } else {
        setPickNotice(errorMessage(error));
      }
    } finally {
      setPendingStart(null);
    }
  }

  return (
    <div className={styles.page}>
      {flash && <Notice tone={flash.tone}>{flash.text}</Notice>}
      <section className={`${styles.card} ${booking.status === 'cancelled' ? styles.cancelled : ''}`} aria-labelledby="manage-title">
        <div className={styles.head}>
          <h1 id="manage-title" className={styles.title}>
            {t('manage.title')}
          </h1>
          <Badge tone={STATUS_TONE[booking.status]}>{t(`manage.status.${booking.status}`)}</Badge>
        </div>

        <div className={styles.when}>
          <span className={styles.whenDate}>{formatDay(booking.start, zone, locale)}</span>
          <span className={styles.whenSub}>
            {formatTime(booking.start, zone, locale)}–{formatTime(booking.end, zone, locale)}
            {ownTime && ` · ${t('summary.yourTime', { time: ownTime })}`}
          </span>
        </div>

        <dl className={styles.facts}>
          <dt>{t('summary.service')}</dt>
          <dd>{context.service?.name ?? '—'}</dd>
          <dt>{t('summary.specialist')}</dt>
          <dd>{context.staff?.name ?? '—'}</dd>
          <dt>{t('summary.salon')}</dt>
          <dd>
            {context.location.name}
            <small>{context.location.address}</small>
          </dd>
          {booking.price && (
            <>
              <dt>{t('summary.price')}</dt>
              <dd>{formatMoney(booking.price, locale)}</dd>
            </>
          )}
          {booking.customer && (
            <>
              <dt>{t('admin.details.customer')}</dt>
              <dd>
                {booking.customer.name}
                <small>{booking.customer.phone}</small>
              </dd>
            </>
          )}
        </dl>

        {booking.status === 'cancelled' && (
          <Notice
            tone={justCancelled ? 'success' : 'info'}
            action={<a className={buttonClass('secondary', 'small')} href="#/">{t('manage.bookAgain')}</a>}
          >
            {justCancelled ? t('manage.cancelledNow') : t('manage.cancelled')}
          </Notice>
        )}

        {confirmed && !changeable && (
          <Notice tone="warning">{t('manage.tooLate', { time: formatDateTime(cutoff, zone, locale) })}</Notice>
        )}

        {changeable && mode === 'view' && (
          <>
            <p className={styles.whenSub}>{t('manage.changeUntil', { time: formatDateTime(cutoff, zone, locale) })}</p>
            <div className={styles.actions}>
              <Button onClick={() => setMode('reschedule')}>{t('manage.reschedule')}</Button>
              <Button variant="danger" onClick={() => setMode('confirmCancel')}>
                {t('manage.cancel')}
              </Button>
            </div>
          </>
        )}

        {changeable && mode === 'confirmCancel' && (
          <Notice
            tone="warning"
            title={t('manage.cancelConfirm')}
            action={
              <div className={styles.actions}>
                <Button variant="danger" size="small" busy={busy} onClick={cancel}>
                  {t('manage.cancelYes')}
                </Button>
                <Button variant="secondary" size="small" disabled={busy} onClick={() => setMode('view')}>
                  {t('manage.cancelNo')}
                </Button>
              </div>
            }
          />
        )}
      </section>

      {changeable && mode === 'reschedule' && context.service && (
        <section className={`${styles.card} ${styles.reschedule}`} aria-labelledby="reschedule-title">
          <div className={styles.rescheduleHead}>
            <h2 id="reschedule-title" className={styles.rescheduleTitle}>
              {t('manage.rescheduleTitle', { name: context.staff?.name ?? '' })}
            </h2>
            <Button variant="ghost" size="small" onClick={() => setMode('view')}>
              {t('manage.keepTime')}
            </Button>
          </div>
          <SlotPicker
            location={context.location}
            serviceId={context.service.id}
            staffId={booking.staff_id}
            onPick={reschedule}
            pendingStart={pendingStart}
            hiddenStarts={hidden}
            refreshKey={refreshKey}
            notice={pickNotice && <Notice tone="warning">{pickNotice}</Notice>}
          />
        </section>
      )}
    </div>
  );
}
