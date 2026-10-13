import type { Customer } from '../api/types';

// A port of Customer.Normalize in internal/booking/customer.go, so that the
// contact form and the in-browser mock accept exactly what the server does.

export interface CustomerInput {
  name: string;
  phone: string;
  email?: string;
}

export type CustomerField = 'name' | 'phone' | 'email';
export type CustomerProblem =
  | 'required'
  | 'too_long'
  | 'phone_characters'
  | 'phone_digits'
  | 'email_invalid';

export type NormalizeResult =
  | { ok: true; customer: Customer }
  | { ok: false; problems: { field: CustomerField; problem: CustomerProblem }[] };

export const MAX_NAME_LENGTH = 100;
const PHONE_SEPARATORS = '+-() ';

// The dot-atom form that Go's net/mail accepts for a bare address.
const ATOM = "[A-Za-z0-9!#$%&'*+/=?^_`{|}~\\u0080-\\uFFFF-]+";
const DOT_ATOM = `${ATOM}(?:\\.${ATOM})*`;
const EMAIL = new RegExp(`^${DOT_ATOM}@${DOT_ATOM}$`);

function phoneProblem(phone: string): CustomerProblem | undefined {
  if (phone === '') return 'required';
  let digits = 0;
  for (const ch of phone) {
    if (ch >= '0' && ch <= '9') digits++;
    else if (!PHONE_SEPARATORS.includes(ch)) return 'phone_characters';
  }
  return digits < 7 || digits > 15 ? 'phone_digits' : undefined;
}

export function normalizeCustomer(input: CustomerInput): NormalizeResult {
  const name = input.name.trim();
  const phone = input.phone.trim();
  const email = (input.email ?? '').trim();
  const problems: { field: CustomerField; problem: CustomerProblem }[] = [];

  if (name === '') problems.push({ field: 'name', problem: 'required' });
  else if ([...name].length > MAX_NAME_LENGTH) problems.push({ field: 'name', problem: 'too_long' });

  const phoneIssue = phoneProblem(phone);
  if (phoneIssue) problems.push({ field: 'phone', problem: phoneIssue });

  if (email !== '' && (email.length > 254 || !EMAIL.test(email))) {
    problems.push({ field: 'email', problem: 'email_invalid' });
  }

  if (problems.length > 0) return { ok: false, problems };
  return { ok: true, customer: email ? { name, phone, email } : { name, phone } };
}

/** The server's wording for a problem, used by the mock's 422 responses. */
export function serverMessage(field: CustomerField, problem: CustomerProblem): string {
  const messages: Record<CustomerProblem, string> = {
    required: field === 'phone' ? 'must have 7 to 15 digits' : 'must be 1 to 100 characters',
    too_long: 'must be 1 to 100 characters',
    phone_characters: 'may contain only digits, spaces, +, -, ( and )',
    phone_digits: 'must have 7 to 15 digits',
    email_invalid: 'is not a valid address',
  };
  return `customer.${field}: ${messages[problem]}`;
}
