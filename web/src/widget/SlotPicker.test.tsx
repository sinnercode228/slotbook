import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { SlotQuery } from '../api/client';
import type { Location, Slots } from '../api/types';
import { addDays, dateRange, daysBetween } from '../schedule/date';
import { dateOf, formatInstant, instantAt } from '../schedule/zone';
import { fakeApi, renderWith } from '../test/render';
import { SlotPicker } from './SlotPicker';

const berlin: Location = {
  id: 2,
  slug: 'snipwell-berlin',
  name: 'Snipwell Berlin',
  time_zone: 'Europe/Berlin',
  address: 'Beispielstrasse 12, Berlin',
  settings: { slot_step_minutes: 15, min_notice_minutes: 60, horizon_days: 60, hold_ttl_seconds: 600, cancel_cutoff_minutes: 1440 },
};

const zone = berlin.time_zone;
const today = dateOf(Date.now(), zone);
const at = (dayOffset: number, hhmm: string) => {
  const [h, m] = hhmm.split(':').map(Number);
  return formatInstant(instantAt(addDays(today, dayOffset), h! * 60 + m!, zone), zone);
};

// Free times by day offset from today: tomorrow has one slot in each part of
// the day, the day after has none, the third day has one.
const free: Record<number, string[]> = { 1: ['09:00', '13:30', '18:00'], 3: ['10:15'] };

function slotsApi(perDay: Record<number, string[]> = free) {
  const listSlots = vi.fn(async (q: SlotQuery): Promise<Slots> => ({
    location_id: q.locationId,
    service_id: q.serviceId,
    staff_id: q.staffId,
    time_zone: zone,
    days: dateRange(q.from, q.to).map((date) => {
      const offset = daysBetween(today, date);
      return { date, slots: (perDay[offset] ?? []).map((hhmm) => at(offset, hhmm)) };
    }),
  }));
  return { api: fakeApi({ listSlots }), listSlots };
}

function renderPicker(props: Partial<Parameters<typeof SlotPicker>[0]> = {}, perDay?: Record<number, string[]>) {
  const { api, listSlots } = slotsApi(perDay);
  const onPick = vi.fn();
  renderWith(<SlotPicker location={berlin} serviceId={5} staffId={null} onPick={onPick} visitorZone={zone} {...props} />, {
    api,
  });
  return { onPick, listSlots };
}

describe('SlotPicker', () => {
  it('asks for one week from today in the salon calendar', async () => {
    const { listSlots } = renderPicker();
    await screen.findByRole('button', { name: /09:00/ });
    expect(listSlots).toHaveBeenCalledWith({ locationId: 2, serviceId: 5, staffId: null, from: today, to: addDays(today, 6) });
  });

  it('greys out days without free times and opens the first day that has some', async () => {
    renderPicker();
    await screen.findByRole('button', { name: /09:00/ });
    const days = within(screen.getByRole('group', { name: 'Days' })).getAllByRole('button');
    expect(days).toHaveLength(7);
    expect(days[0]).toBeDisabled();
    expect(days[1]).toHaveAttribute('aria-pressed', 'true');
    expect(days[2]).toBeDisabled();
    expect(days[3]).toBeEnabled();
  });

  it('groups the times into morning, afternoon and evening', async () => {
    renderPicker();
    await screen.findByRole('button', { name: /09:00/ });
    const morning = screen.getByRole('group', { name: 'Morning' });
    const afternoon = screen.getByRole('group', { name: 'Afternoon' });
    const evening = screen.getByRole('group', { name: 'Evening' });
    expect(within(morning).getByRole('button')).toHaveTextContent('09:00');
    expect(within(afternoon).getByRole('button')).toHaveTextContent('13:30');
    expect(within(evening).getByRole('button')).toHaveTextContent('18:00');
  });

  it('reports the picked start as the server wrote it', async () => {
    const { onPick } = renderPicker();
    await userEvent.click(await screen.findByRole('button', { name: /13:30/ }));
    expect(onPick).toHaveBeenCalledWith(at(1, '13:30'));
  });

  it('shows another day when it is chosen', async () => {
    renderPicker();
    await screen.findByRole('button', { name: /09:00/ });
    const days = within(screen.getByRole('group', { name: 'Days' })).getAllByRole('button');
    await userEvent.click(days[3]!);
    expect(screen.getByRole('button', { name: /10:15/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /09:00/ })).not.toBeInTheDocument();
  });

  it('drops a start that was just taken without waiting for the reload', async () => {
    renderPicker({ hiddenStarts: [at(1, '13:30')] });
    await screen.findByRole('button', { name: /09:00/ });
    expect(screen.queryByRole('button', { name: /13:30/ })).not.toBeInTheDocument();
  });

  it('says when the visitor is in another zone and shows their own time too', async () => {
    renderPicker({ visitorZone: 'Europe/Moscow' });
    const slot = await screen.findByRole('button', { name: /09:00/ });
    expect(screen.getByText(/Your device is on/)).toBeInTheDocument();
    // Moscow is an hour ahead of Berlin in October and two in winter.
    expect(slot).toHaveAccessibleName(/^09:00, your time (10|11):00$/);
    expect(slot).toHaveTextContent(/^09:00(10|11):00$/);
  });

  it('keeps quiet about zones when the visitor shares the salon offset', async () => {
    renderPicker({ visitorZone: 'Europe/Paris' });
    await screen.findByRole('button', { name: /09:00/ });
    expect(screen.queryByText(/Your device is on/)).not.toBeInTheDocument();
  });

  it('offers the next week when this one is fully booked', async () => {
    const { listSlots } = renderPicker({}, { 9: ['11:00'] });
    expect(await screen.findByText('No free times this week.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Show next week' }));
    expect(await screen.findByRole('button', { name: /11:00/ })).toBeInTheDocument();
    expect(listSlots).toHaveBeenLastCalledWith(expect.objectContaining({ from: addDays(today, 7), to: addDays(today, 13) }));
  });
});
