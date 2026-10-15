/** Error codes from docs/api.md, plus "network" when no response arrived. */
export type ErrorCode =
  | 'bad_request'
  | 'unauthorized'
  | 'not_found'
  | 'hold_not_found'
  | 'method_not_allowed'
  | 'slot_taken'
  | 'already_confirmed'
  | 'too_late'
  | 'not_active'
  | 'hold_expired'
  | 'payload_too_large'
  | 'invalid_slot'
  | 'invalid_staff'
  | 'validation_failed'
  | 'idempotency_key_reused'
  | 'rate_limited'
  | 'internal'
  | 'network';

export class ApiError extends Error {
  readonly status: number;
  readonly code: ErrorCode;
  /** Seconds to wait before trying again, from Retry-After. */
  readonly retryAfter: number | undefined;

  constructor(status: number, code: ErrorCode, message: string, retryAfter?: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.retryAfter = retryAfter;
  }
}

export function errorCode(err: unknown): ErrorCode | undefined {
  return err instanceof ApiError ? err.code : undefined;
}
