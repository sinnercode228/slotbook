import type { Booking, Service, Staff } from '../api/types';
import { useI18n } from '../i18n/useI18n';
import { formatTime } from '../lib/format';
import { initials } from '../lib/initials';
import type { LocalDate } from '../schedule/date';
import { formatTimeOfDay } from '../schedule/timeOfDay';
import { dateOf, wallClock } from '../schedule/zone';
import styles from './Admin.module.css';

const PX_PER_MINUTE = 72 / 60;
const PAD = 10;
const DEFAULT_HOURS: [number, number] = [8, 21];

interface DayCalendarProps {
  date: LocalDate;
  zone: string;
  staff: Staff[];
  bookings: Booking[];
  services: Service[];
  showCancelled: boolean;
  now: number;
  onSelect: (booking: Booking) => void;
}

/** Minutes since local midnight of `date`, clamped to that day. */
function minutesOn(date: LocalDate, zone: string, instant: string | number): number {
  const at = typeof instant === 'number' ? instant : Date.parse(instant);
  const day = dateOf(at, zone);
  if (day < date) return 0;
  if (day > date) return 24 * 60;
  return wallClock(at, zone).minutes;
}

function eventClass(b: Booking): string {
  if (b.kind === 'block') return styles.block!;
  if (b.status === 'held') return styles.held!;
  if (b.status === 'confirmed') return styles.confirmed!;
  return styles.cancelledEvent!;
}

/** One column per specialist, bookings placed by their local wall-clock time. */
export function DayCalendar({ date, zone, staff, bookings, services, showCancelled, now, onSelect }: DayCalendarProps) {
  const { t, locale } = useI18n();
  const shown = bookings.filter((b) => showCancelled || b.status === 'held' || b.status === 'confirmed');

  let [fromHour, toHour] = DEFAULT_HOURS;
  for (const b of shown) {
    fromHour = Math.min(fromHour, Math.floor(minutesOn(date, zone, b.start) / 60));
    toHour = Math.max(toHour, Math.ceil(minutesOn(date, zone, b.busy_until) / 60));
  }
  const from = fromHour * 60;
  const height = PAD * 2 + (toHour - fromHour) * 60 * PX_PER_MINUTE;
  const y = (minutes: number) => PAD + (minutes - from) * PX_PER_MINUTE;
  const nowMinutes = dateOf(now, zone) === date ? wallClock(now, zone).minutes : null;
  const serviceName = (id: number | null) => services.find((s) => s.id === id)?.name ?? '';

  return (
    <div className={styles.calendar} role="region" aria-label={t('admin.title')} tabIndex={0}>
      <div className={styles.grid} style={{ gridTemplateColumns: `var(--axis) repeat(${staff.length}, minmax(156px, 1fr))` }}>
        <div className={styles.corner} />
        {staff.map((member) => (
          <div key={member.id} className={styles.colHead}>
            <span className={styles.avatar} aria-hidden="true">
              {initials(member.name)}
            </span>
            <span className={styles.colHeadName}>{member.name}</span>
          </div>
        ))}

        <div className={styles.axis} style={{ height }} aria-hidden="true">
          {Array.from({ length: toHour - fromHour + 1 }, (_, i) => (
            <span key={i} className={styles.hourLabel} style={{ top: y((fromHour + i) * 60) }}>
              {formatTimeOfDay((fromHour + i) * 60)}
            </span>
          ))}
        </div>

        {staff.map((member) => (
          <div key={member.id} className={styles.column} style={{ height, backgroundPositionY: PAD }}>
            {shown
              .filter((b) => b.staff_id === member.id)
              .map((b) => {
                const start = minutesOn(date, zone, b.start);
                const end = minutesOn(date, zone, b.end);
                const busy = minutesOn(date, zone, b.busy_until);
                const boxHeight = Math.max(20, (busy - start) * PX_PER_MINUTE);
                const bufferHeight = (busy - end) * PX_PER_MINUTE;
                const compact = boxHeight < 44;
                const time = `${formatTime(b.start, zone, locale)}–${formatTime(b.end, zone, locale)}`;
                const title = b.kind === 'block' ? t('admin.blocked') : (b.customer?.name ?? t('admin.legend.held'));
                const sub = b.kind === 'block' ? b.note : serviceName(b.service_id);
                const cancelled = b.status === 'cancelled';
                return (
                  <button
                    key={b.id}
                    type="button"
                    className={`${styles.event} ${eventClass(b)} ${compact ? styles.compact : ''}`}
                    style={{
                      top: y(start),
                      height: boxHeight,
                      ...(cancelled ? { left: '50%', zIndex: 0 } : {}),
                    }}
                    aria-label={`${time}, ${title}${sub ? `, ${sub}` : ''}, ${t(`admin.legend.${b.kind === 'block' ? 'block' : b.status === 'held' || b.status === 'confirmed' ? b.status : 'cancelled'}`)}`}
                    onClick={() => onSelect(b)}
                  >
                    <span className={styles.eventBody}>
                      <span className={styles.eventTime}>{time}</span>
                      <span className={styles.eventTitle}>{title}</span>
                      {!compact && sub && <span className={styles.eventSub}>{sub}</span>}
                    </span>
                    {bufferHeight >= 4 && <span className={styles.buffer} style={{ height: bufferHeight }} aria-hidden="true" />}
                  </button>
                );
              })}
            {nowMinutes !== null && nowMinutes >= from && nowMinutes <= toHour * 60 && (
              <div className={styles.nowLine} style={{ top: y(nowMinutes) }} aria-hidden="true" />
            )}
          </div>
        ))}
      </div>
      {shown.length === 0 && <p className={styles.calendarEmpty}>{t('admin.emptyDay')}</p>}
    </div>
  );
}
