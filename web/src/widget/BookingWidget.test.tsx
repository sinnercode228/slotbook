import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { BookingApi } from '../api/client';
import { ApiError } from '../api/errors';
import { createMockApi } from '../mock/mockApi';
import { memoryStorage } from '../mock/storage';
import { renderWith } from '../test/render';
import { BookingWidget } from './BookingWidget';

// The widget against the in-browser API, with failures injected where the
// server would answer 409, 410 or not at all.
function setup(overrides: (api: BookingApi) => Partial<BookingApi> = () => ({})) {
  const base = createMockApi({ storage: memoryStorage(), samples: false });
  const api: BookingApi = { ...base, ...overrides(base) };
  const view = renderWith(<BookingWidget />, { api });
  /** Unmounts the widget and mounts a fresh one, as a page reload does. */
  const reload = () => {
    view.unmount();
    renderWith(<BookingWidget />, { api });
  };
  return { user: userEvent.setup(), api, reload };
}

async function reachSlots(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: /Snipwell Berlin/ }));
  await user.click(await screen.findByRole('button', { name: /^Haircut/ }));
  await user.click(await screen.findByRole('button', { name: /Lena H\./ }));
  const [firstGroup] = await screen.findAllByRole('group', { name: /^(Morning|Afternoon|Evening)$/ });
  return within(firstGroup!).getAllByRole('button');
}

/** The salon time of a slot button, without the visitor's own time under it. */
const salonTime = (slot: HTMLElement) => slot.firstElementChild!.textContent!;

async function fillAndConfirm(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText('Name'), 'Anna');
  await user.type(screen.getByLabelText('Phone'), '+7 900 123-45-67');
  await user.click(screen.getByRole('button', { name: 'Confirm booking' }));
}

describe('BookingWidget', () => {
  it('books a slot and ends on a confirmation with the manage link', async () => {
    const { user } = setup();
    const [slot] = await reachSlots(user);
    await user.click(slot!);
    expect(await screen.findByRole('timer')).toHaveTextContent('10:00');
    await fillAndConfirm(user);
    expect(await screen.findByRole('heading', { name: 'You’re booked' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Manage booking' })).toHaveAttribute(
      'href',
      expect.stringMatching(/^#\/booking\/[0-9a-f-]{36}\?token=[\w-]{43}$/),
    );
  });

  it('removes a slot someone else took, says so and stays on the list', async () => {
    const { user } = setup(() => ({
      createHold: vi.fn().mockRejectedValue(new ApiError(409, 'slot_taken', 'slot is already taken')),
    }));
    const [slot] = await reachSlots(user);
    const time = salonTime(slot!);
    await user.click(slot!);
    expect(await screen.findByText(`${time} was just booked by someone else. Please choose another time.`)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Choose a time' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: new RegExp(`^${time}`) })).not.toBeInTheDocument();
  });

  it('asks the visitor to wait when the server refuses a hold for too many attempts', async () => {
    const { user } = setup(() => ({
      createHold: vi
        .fn()
        .mockRejectedValue(new ApiError(429, 'rate_limited', 'too many requests from this address, try again in 30 s')),
    }));
    const [slot] = await reachSlots(user);
    const time = salonTime(slot!);
    await user.click(slot!);
    expect(await screen.findByText('Too many reservations from your connection in a short time. Please wait a minute and try again.')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Choose a time' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: new RegExp(`^${time}`) })).toBeInTheDocument();
  });

  it('tells the visitor how long to wait when the server says so', async () => {
    const { user } = setup(() => ({
      createHold: vi.fn().mockRejectedValue(new ApiError(429, 'rate_limited', 'try again in 90 s', 90)),
    }));
    const [slot] = await reachSlots(user);
    await user.click(slot!);
    expect(
      await screen.findByText('Too many reservations from your connection in a short time. Please try again in 2 min.'),
    ).toBeInTheDocument();
  });

  it('sends the visitor back to the times when the hold ran out on the server', async () => {
    const { user } = setup(() => ({
      confirmHold: vi.fn().mockRejectedValue(new ApiError(410, 'hold_expired', 'hold has expired')),
    }));
    const [slot] = await reachSlots(user);
    await user.click(slot!);
    await fillAndConfirm(user);
    expect(await screen.findByText(/Your reservation ran out before it was confirmed/)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Choose a time' })).toBeInTheDocument();
  });

  it('keeps the hold after a network failure and retries with the same idempotency key', async () => {
    const confirm = vi.fn();
    const { user } = setup((base) => ({
      confirmHold: confirm.mockRejectedValueOnce(new ApiError(0, 'network', 'Failed to fetch')).mockImplementation(base.confirmHold),
    }));
    const [slot] = await reachSlots(user);
    await user.click(slot!);
    await fillAndConfirm(user);
    expect(await screen.findByText(/The booking was not confirmed because the connection failed/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Confirm booking' }));
    expect(await screen.findByRole('heading', { name: 'You’re booked' })).toBeInTheDocument();
    expect(confirm).toHaveBeenCalledTimes(2);
    expect(confirm.mock.calls[1]![2]).toBe(confirm.mock.calls[0]![2]);
  });

  it('keeps the running hold and the typed details across a reload', async () => {
    const createHold = vi.fn();
    const { user, reload } = setup((base) => ({ createHold: createHold.mockImplementation(base.createHold) }));
    const [slot] = await reachSlots(user);
    await user.click(slot!);
    await user.type(await screen.findByLabelText('Name'), 'Anna');
    reload();
    expect(await screen.findByRole('timer')).toHaveTextContent('10:00');
    expect(screen.getByLabelText('Name')).toHaveValue('Anna');
    await user.type(screen.getByLabelText('Phone'), '+7 900 123-45-67');
    await user.click(screen.getByRole('button', { name: 'Confirm booking' }));
    expect(await screen.findByRole('heading', { name: 'You’re booked' })).toBeInTheDocument();
    expect(createHold).toHaveBeenCalledTimes(1);
  });

  it('still shows the confirmation and its manage link after a reload', async () => {
    const { user, reload } = setup();
    const [slot] = await reachSlots(user);
    await user.click(slot!);
    await fillAndConfirm(user);
    const link = await screen.findByRole('link', { name: 'Manage booking' });
    const href = link.getAttribute('href');
    reload();
    expect(await screen.findByRole('heading', { name: 'You’re booked' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Manage booking' })).toHaveAttribute('href', href);
  });

  it('starts from the first step after "Book another visit"', async () => {
    const { user, reload } = setup();
    const [slot] = await reachSlots(user);
    await user.click(slot!);
    await fillAndConfirm(user);
    await user.click(await screen.findByRole('button', { name: /Book another/ }));
    reload();
    expect(await screen.findByRole('heading', { name: 'Choose a salon' })).toBeInTheDocument();
  });

  it('lets the visitor return to the time list without losing their own hold', async () => {
    const createHold = vi.fn();
    const { user } = setup((base) => ({ createHold: createHold.mockImplementation(base.createHold) }));
    const [slot] = await reachSlots(user);
    const time = salonTime(slot!);
    await user.click(slot!);
    await screen.findByRole('timer');
    await user.click(screen.getByRole('button', { name: 'Back' }));
    const held = await screen.findByRole('button', { name: new RegExp(`^${time}`) });
    expect(held).toHaveAttribute('aria-pressed', 'true');
    await user.click(held);
    expect(await screen.findByRole('timer')).toBeInTheDocument();
    expect(createHold).toHaveBeenCalledTimes(1);
  });
});
