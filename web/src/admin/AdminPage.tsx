import { useEffect, useMemo, useState } from 'react';
import { useApi } from '../app/ApiContext';
import { errorCode } from '../api/errors';
import type { Booking, Location } from '../api/types';
import { ChevronLeft, ChevronRight } from '../components/icons';
import { Button, ErrorState, SkeletonList } from '../components/ui';
import { useI18n } from '../i18n/useI18n';
import { formatDateLabel } from '../lib/format';
import { useNow } from '../lib/useNow';
import { useResource } from '../lib/useResource';
import { addDays, parseLocalDate, type LocalDate } from '../schedule/date';
import { dateOf } from '../schedule/zone';
import styles from './Admin.module.css';
import { AdminLogin } from './AdminLogin';
import { BlockForm } from './BlockForm';
import { BookingDialog } from './BookingDialog';
import { DayCalendar } from './DayCalendar';
import { OutboxPanel } from './OutboxPanel';

const TOKEN_KEY = 'slotbook.adminToken';
const POLL_MS = 15_000;

function readToken(): string | null {
  try {
    return window.sessionStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

function storeToken(token: string | null): void {
  try {
    if (token) window.sessionStorage.setItem(TOKEN_KEY, token);
    else window.sessionStorage.removeItem(TOKEN_KEY);
  } catch {
    // The token then lasts until the page is closed.
  }
}

export function AdminPage() {
  const [token, setToken] = useState(readToken);
  const signIn = (value: string | null) => {
    storeToken(value);
    setToken(value);
  };
  if (!token) return <AdminLogin onSignedIn={signIn} />;
  return <Schedule token={token} onSignOut={() => signIn(null)} />;
}

function Schedule({ token, onSignOut }: { token: string; onSignOut: () => void }) {
  const api = useApi();
  const { t } = useI18n();
  const admin = useMemo(() => api.admin(token), [api, token]);
  const locations = useResource('locations', () => api.listLocations());
  const [locationId, setLocationId] = useState<number | null>(null);
  const location = locations.data?.find((l) => l.id === locationId) ?? locations.data?.[0];

  if (!location) {
    return locations.error !== undefined ? (
      <ErrorState error={locations.error} onRetry={locations.reload} />
    ) : (
      <SkeletonList rows={4} label={t('common.loading')} />
    );
  }
  return (
    <LocationSchedule
      key={location.id}
      admin={admin}
      location={location}
      locations={locations.data ?? []}
      onLocation={setLocationId}
      onSignOut={onSignOut}
    />
  );
}

function LocationSchedule({
  admin,
  location,
  locations,
  onLocation,
  onSignOut,
}: {
  admin: ReturnType<ReturnType<typeof useApi>['admin']>;
  location: Location;
  locations: Location[];
  onLocation: (id: number) => void;
  onSignOut: () => void;
}) {
  const api = useApi();
  const { t, locale } = useI18n();
  const zone = location.time_zone;
  const now = useNow(30_000);
  const [today] = useState(() => dateOf(Date.now(), zone));
  const [date, setDate] = useState<LocalDate>(today);
  const [showCancelled, setShowCancelled] = useState(false);
  const [selected, setSelected] = useState<Booking | null>(null);
  const [version, setVersion] = useState(0);

  const staff = useResource(`staff|${location.id}`, () => api.listStaff(location.id));
  const services = useResource(`services|${location.id}`, () => api.listServices(location.id));
  const day = useResource(`day|${location.id}|${date}`, () => admin.listBookings(location.id, date));
  const { reload } = day;

  const refresh = () => {
    reload();
    setVersion((v) => v + 1);
  };

  useEffect(() => {
    const id = window.setInterval(reload, POLL_MS);
    const stop = api.subscribe(() => {
      reload();
      setVersion((v) => v + 1);
    });
    return () => {
      window.clearInterval(id);
      stop();
    };
  }, [api, reload]);

  useEffect(() => {
    if (errorCode(day.error) === 'unauthorized') onSignOut();
  }, [day.error, onSignOut]);

  return (
    <div className={styles.admin}>
      <div className={styles.toolbar}>
        <div className={`${styles.toolbarField} ${styles.locationField}`}>
          <label className={styles.label} htmlFor="admin-location">
            {t('admin.salon')}
          </label>
          <select
            id="admin-location"
            className={styles.select}
            value={location.id}
            onChange={(e) => onLocation(Number(e.target.value))}
          >
            {locations.map((l) => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
          </select>
        </div>
        <div className={styles.toolbarField}>
          <label className={styles.label} htmlFor="admin-date">
            {t('admin.date')}
          </label>
          <div className={styles.dateNav}>
            <button type="button" className={styles.iconButton} onClick={() => setDate(addDays(date, -1))} aria-label={t('admin.prevDay')}>
              <ChevronLeft />
            </button>
            <input
              id="admin-date"
              className={styles.input}
              type="date"
              value={date}
              onChange={(e) => {
                try {
                  setDate(parseLocalDate(e.target.value));
                } catch {
                  // A half-typed date; keep the current one.
                }
              }}
            />
            <button type="button" className={styles.iconButton} onClick={() => setDate(addDays(date, 1))} aria-label={t('admin.nextDay')}>
              <ChevronRight />
            </button>
            <Button variant="secondary" onClick={() => setDate(today)} disabled={date === today}>
              {t('admin.today')}
            </Button>
          </div>
        </div>
        <Button variant="ghost" className={styles.signOut} onClick={onSignOut}>
          {t('admin.logout')}
        </Button>
      </div>

      <div className={styles.columns}>
        <section className={styles.panel} aria-labelledby="day-title">
          <div className={styles.panelHead}>
            <h1 id="day-title" className={styles.dayTitle}>
              {formatDateLabel(date, locale, 'long')}
            </h1>
            <label className={styles.toggle}>
              <input type="checkbox" checked={showCancelled} onChange={(e) => setShowCancelled(e.target.checked)} />
              {t('admin.showCancelled')}
            </label>
          </div>
          <ul className={styles.legend}>
            <li>
              <span className={`${styles.swatch} ${styles.confirmed}`} />
              {t('admin.legend.confirmed')}
            </li>
            <li>
              <span className={`${styles.swatch} ${styles.held}`} />
              {t('admin.legend.held')}
            </li>
            <li>
              <span className={`${styles.swatch} ${styles.block}`} />
              {t('admin.legend.block')}
            </li>
          </ul>
          {day.error !== undefined && !day.data ? (
            <ErrorState error={day.error} onRetry={reload} />
          ) : !day.data || !staff.data || !services.data ? (
            <SkeletonList rows={4} label={t('common.loading')} />
          ) : (
            <DayCalendar
              date={date}
              zone={zone}
              staff={staff.data}
              bookings={day.data.bookings}
              services={services.data}
              showCancelled={showCancelled}
              now={now}
              onSelect={setSelected}
            />
          )}
        </section>

        <div className={styles.side}>
          {staff.data && <BlockForm admin={admin} date={date} zone={zone} staff={staff.data} onCreated={refresh} />}
          <OutboxPanel admin={admin} locationId={location.id} zone={zone} version={version} />
        </div>
      </div>

      {selected && (
        <BookingDialog
          booking={selected}
          zone={zone}
          staff={staff.data ?? []}
          services={services.data ?? []}
          admin={admin}
          onClose={() => setSelected(null)}
          onChanged={refresh}
        />
      )}
    </div>
  );
}
