import { describe, expect, it } from 'vitest';
import { normalizeCustomer } from './customer';

// The cases mirror internal/booking/customer_test.go: the form must accept
// exactly what the server accepts.
describe('normalizeCustomer', () => {
  it('trims fields and keeps the phone as typed', () => {
    const result = normalizeCustomer({ name: '  Anna  ', phone: ' +7 (900) 123-45-67 ', email: ' anna@example.com ' });
    expect(result).toEqual({
      ok: true,
      customer: { name: 'Anna', phone: '+7 (900) 123-45-67', email: 'anna@example.com' },
    });
  });

  it('drops an empty email', () => {
    expect(normalizeCustomer({ name: 'Anna', phone: '9001234567', email: '  ' })).toEqual({
      ok: true,
      customer: { name: 'Anna', phone: '9001234567' },
    });
  });

  it.each([
    [{ name: ' ', phone: '9001234567' }, 'name', 'required'],
    [{ name: 'x'.repeat(101), phone: '9001234567' }, 'name', 'too_long'],
    [{ name: 'Anna', phone: '' }, 'phone', 'required'],
    [{ name: 'Anna', phone: '900 123 45 67 ext 2' }, 'phone', 'phone_characters'],
    [{ name: 'Anna', phone: '123-456' }, 'phone', 'phone_digits'],
    [{ name: 'Anna', phone: '1234567890123456' }, 'phone', 'phone_digits'],
    [{ name: 'Anna', phone: '9001234567', email: 'anna' }, 'email', 'email_invalid'],
    [{ name: 'Anna', phone: '9001234567', email: 'Anna <anna@example.com>' }, 'email', 'email_invalid'],
    [{ name: 'Anna', phone: '9001234567', email: 'anna@@example.com' }, 'email', 'email_invalid'],
  ] as const)('rejects %j: %s %s', (input, field, problem) => {
    const result = normalizeCustomer(input);
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.problems).toContainEqual({ field, problem });
  });

  it('counts a name in characters, not UTF-16 units', () => {
    expect(normalizeCustomer({ name: '🙂'.repeat(100), phone: '9001234567' }).ok).toBe(true);
  });

  it('reports every bad field at once for the form', () => {
    const result = normalizeCustomer({ name: '', phone: '12', email: 'x' });
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.problems.map((p) => p.field)).toEqual(['name', 'phone', 'email']);
  });
});
