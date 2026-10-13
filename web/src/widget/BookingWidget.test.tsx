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
  renderWith(<BookingWidget />, { api });
  return { user: userEvent.setup(), api };
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
