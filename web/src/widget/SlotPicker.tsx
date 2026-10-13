import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useApi } from '../app/ApiContext';
import type { Location } from '../api/types';
import { ChevronLeft, ChevronRight } from '../components/icons';
import { Button, ErrorState } from '../components/ui';
import { publishVisibleSlots } from '../demo/visibleSlots';
import { useI18n } from '../i18n/useI18n';
import {
  dayPeriod,
  formatDateLabel,
  formatTime,
  minutesOfDay,
  visitorZone as detectVisitorZone,
  zoneLabel,
  zonesDiffer,
  type DayPeriod,
} from '../lib/format';
import { useResource } from '../lib/useResource';
import { addDays, type LocalDate } from '../schedule/date';
import { dateOf, instantAt } from '../schedule/zone';
import styles from './Widget.module.css';

const WEEK = 7;
const PERIODS: DayPeriod[] = ['morning', 'afternoon', 'evening'];

export interface SlotPickerProps {
  location: Location;
  serviceId: number;
  /** null lists times when any specialist who offers the service is free. */
  staffId: number | null;
  onPick: (start: string) => void;
  /** A start being reserved right now; its button shows that it is busy. */
  pendingStart?: string | null;
  /** The visitor's own running hold: listed even though it now counts as busy. */
  heldStart?: string | null;
  /** Starts known to be gone, removed before the reload confirms it. */
  hiddenStarts?: readonly string[];
  notice?: ReactNode;
  /** Changing it reloads the list. */
  refreshKey?: number;
  visitorZone?: string;
}

function weekLabel(from: LocalDate, locale: string): string {
  const first = formatDateLabel(from, locale, 'month');
  const last = formatDateLabel(addDays(from, WEEK - 1), locale, 'month');
  return first === last ? first : `${first.split(' ')[0]} – ${last}`;
}

export function SlotPicker({
  location,
  serviceId,
  staffId,
  onPick,
  pendingStart = null,
  heldStart = null,
  hiddenStarts = [],
  notice,
  refreshKey = 0,
  visitorZone,
}: SlotPickerProps) {
  const api = useApi();
  const { t, locale } = useI18n();
  const zone = location.time_zone;
  const [today] = useState(() => dateOf(Date.now(), zone));
  const [ownZone] = useState(() => visitorZone ?? detectVisitorZone());
  const [weekStart, setWeekStart] = useState(today);
  const [chosenDate, setChosenDate] = useState<LocalDate | null>(null);
  const lastBookable = addDays(today, location.settings.horizon_days);

  const weekEnd = addDays(weekStart, WEEK - 1);
  const slots = useResource(`${location.id}|${serviceId}|${staffId}|${weekStart}`, () =>
    api.listSlots({ locationId: location.id, serviceId, staffId, from: weekStart, to: weekEnd }),
  );
  const { reload } = slots;

  // The list stays as the visitor saw it, as on any booking page: a time
  // taken meanwhile is reported when it is picked, and then the list reloads.
  useEffect(() => {
    if (refreshKey > 0) reload();
  }, [refreshKey, reload]);

  const days = useMemo(() => {
    const hidden = new Set(hiddenStarts);
    return (slots.data?.days ?? []).map((day) => {
      const starts = day.slots.filter((s) => !hidden.has(s));
      if (heldStart && dateOf(Date.parse(heldStart), zone) === day.date && !starts.includes(heldStart)) {
        starts.push(heldStart);
        starts.sort((a, b) => Date.parse(a) - Date.parse(b));
      }
      return { date: day.date, starts };
    });
  }, [slots.data, hiddenStarts, heldStart, zone]);

  const firstOpen = days.find((d) => d.starts.length > 0)?.date ?? null;
  const chosenIsOpen = days.some((d) => d.date === chosenDate && d.starts.length > 0);
  const selectedDate = chosenIsOpen ? chosenDate : firstOpen;
  const selected = days.find((d) => d.date === selectedDate);

  useEffect(() => {
    publishVisibleSlots(
      selected ? { serviceId, staffId, zone, starts: selected.starts.filter((s) => s !== heldStart) } : null,
    );
  }, [selected, serviceId, staffId, zone, heldStart]);
  useEffect(() => () => publishVisibleSlots(null), []);

  // Offsets can change within the week (DST), so compare them on the day shown.
  const shownInstant = instantAt(selectedDate ?? weekStart, 12 * 60, zone);
  const showVisitorTime = zonesDiffer(zone, ownZone, shownInstant);

  const canGoBack = weekStart > today;
  const canGoForward = addDays(weekStart, WEEK) <= lastBookable;
  const moveWeek = (direction: 1 | -1) => {
    setWeekStart(addDays(weekStart, direction * WEEK));
    setChosenDate(null);
  };

  const grouped = PERIODS.map((period) => ({
    period,
    starts: (selected?.starts ?? []).filter((s) => dayPeriod(minutesOfDay(s, zone)) === period),
  })).filter((g) => g.starts.length > 0);

  return (
    <div className={styles.stack}>
      <div>
        <div className={styles.weekHeader}>
          <span className={styles.month} aria-live="polite">
            {weekLabel(weekStart, locale)}
          </span>
          <span className={styles.weekNav}>
            <button
              type="button"
              className={styles.iconButton}
              onClick={() => moveWeek(-1)}
              disabled={!canGoBack}
              aria-label={t('slots.prevWeek')}
            >
              <ChevronLeft />
            </button>
            <button
              type="button"
              className={styles.iconButton}
              onClick={() => moveWeek(1)}
              disabled={!canGoForward}
              aria-label={t('slots.nextWeek')}
            >
              <ChevronRight />
            </button>
          </span>
        </div>
        <div className={styles.days} role="group" aria-label={t('slots.days')}>
          {Array.from({ length: WEEK }, (_, i) => {
            const date = addDays(weekStart, i);
            const open = days.find((d) => d.date === date)?.starts.length ?? 0;
            const long = formatDateLabel(date, locale, 'long');
            return (
              <button
                key={date}
                type="button"
                className={styles.day}
                disabled={open === 0}
                aria-pressed={date === selectedDate}
                aria-label={open === 0 ? `${long}, ${t('slots.dayFull')}` : long}
                onClick={() => setChosenDate(date)}
              >
                <span className={styles.dayName}>{formatDateLabel(date, locale, 'weekday')}</span>
                <span className={styles.dayNumber}>{formatDateLabel(date, locale, 'day')}</span>
              </button>
            );
          })}
        </div>
        <p className={styles.zoneNote}>
          {t('slots.zoneNote', { zone: zoneLabel(zone, locale, shownInstant) })}
          {showVisitorTime && ` ${t('slots.visitorZoneNote', { zone: zoneLabel(ownZone, locale, shownInstant) })}`}
        </p>
      </div>

      {notice}

      {slots.error !== undefined && !slots.data ? (
        <ErrorState error={slots.error} onRetry={reload} />
      ) : !slots.data ? (
        <div className={styles.slotSkeleton} role="status" aria-label={t('common.loading')}>
          {Array.from({ length: 8 }, (_, i) => (
            <span key={i} />
          ))}
        </div>
      ) : !selected ? (
        <div className={styles.empty}>
          <p>{t('slots.noneThisWeek')}</p>
          {canGoForward && (
            <Button variant="secondary" onClick={() => moveWeek(1)}>
              {t('slots.showNextWeek')}
            </Button>
          )}
        </div>
      ) : (
        <div className={`${styles.slotArea} ${slots.loading ? styles.refreshing : ''}`}>
          {grouped.map(({ period, starts }) => (
            <section key={period} role="group" aria-label={t(`slots.${period}`)}>
              <h3 className={styles.groupTitle}>{t(`slots.${period}`)}</h3>
              <ul className={styles.slots}>
                {starts.map((start) => {
                  const time = formatTime(start, zone, locale);
                  const own = showVisitorTime ? formatTime(start, ownZone, locale) : null;
                  return (
                    <li key={start}>
                      <button
                        type="button"
                        className={styles.slot}
                        aria-pressed={start === heldStart || start === pendingStart}
                        aria-busy={start === pendingStart || undefined}
                        disabled={pendingStart !== null}
                        aria-label={own ? `${time}, ${t('slots.yourTime', { time: own })}` : time}
                        onClick={() => onPick(start)}
                      >
                        <span>{time}</span>
                        {own && (
                          <span className={styles.slotLocal} aria-hidden="true">
                            {own}
                          </span>
                        )}
                      </button>
                    </li>
                  );
                })}
              </ul>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}
