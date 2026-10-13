import { screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { manageRoute } from '../app/router';
import { createMockApi } from '../mock/mockApi';
import { memoryStorage } from '../mock/storage';
import { renderWith } from '../test/render';
import { ManagePage } from './ManagePage';

// Monday 2026-10-12, 10:00 in Berlin; Lena (4) cuts hair (5) on weekdays.
const now = Date.parse('2026-10-12T08:00:00Z');
const anna = { name: 'Anna', phone: '+7 900 123-45-67' };

afterEach(() => {
  window.history.replaceState(null, '', '#/');
});

describe('ManagePage', () => {
  it('follows a link to a booking that was moved on to the booking that replaced it', async () => {
    const api = createMockApi({ storage: memoryStorage(), clock: () => now, samples: false });
    const hold = await api.createHold({ serviceId: 5, staffId: 4, start: '2026-10-14T10:00:00+02:00' });
    const { booking, manage_token: token } = await api.confirmHold(hold.hold_token, anna, '');
    const moved = await api.rescheduleBooking(booking.id, token, '2026-10-15T11:00:00+02:00');

    renderWith(<ManagePage bookingId={booking.id} manageToken={token} />, { api });

    await waitFor(() => expect(window.location.hash).toBe(`#${manageRoute(moved.id, token)}`));
    expect(screen.queryByText(/cancelled/i)).not.toBeInTheDocument();
  });
});
