CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE locations (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug                  text NOT NULL UNIQUE,
    name                  text NOT NULL,
    time_zone             text NOT NULL,
    address               text NOT NULL DEFAULT '',
    -- Per-location settings; NULL means the default from internal/config.
    slot_step_minutes     integer CHECK (slot_step_minutes > 0),
    min_notice_minutes    integer CHECK (min_notice_minutes >= 0),
    horizon_days          integer CHECK (horizon_days >= 0),
    hold_ttl_seconds      integer CHECK (hold_ttl_seconds > 0),
    cancel_cutoff_minutes integer CHECK (cancel_cutoff_minutes >= 0)
);

CREATE TABLE staff (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    location_id bigint NOT NULL REFERENCES locations (id),
    name        text NOT NULL,
    active      boolean NOT NULL DEFAULT true
);
CREATE INDEX staff_location_idx ON staff (location_id);

CREATE TABLE services (
    id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    location_id          bigint NOT NULL REFERENCES locations (id),
    name                 text NOT NULL,
    duration_minutes     integer NOT NULL CHECK (duration_minutes > 0),
    buffer_after_minutes integer NOT NULL DEFAULT 0 CHECK (buffer_after_minutes >= 0),
    price_minor          bigint NOT NULL CHECK (price_minor >= 0),
    currency             text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    active               boolean NOT NULL DEFAULT true
);
CREATE INDEX services_location_idx ON services (location_id);

CREATE TABLE staff_services (
    staff_id   bigint NOT NULL REFERENCES staff (id) ON DELETE CASCADE,
    service_id bigint NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    PRIMARY KEY (staff_id, service_id)
);
CREATE INDEX staff_services_service_idx ON staff_services (service_id);

-- Local wall-clock hours; weekday follows Go's time.Weekday (0 is Sunday)
-- and end_time may be 24:00.
CREATE TABLE working_hours (
    staff_id   bigint NOT NULL REFERENCES staff (id) ON DELETE CASCADE,
    weekday    smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time time NOT NULL,
    end_time   time NOT NULL CHECK (start_time < end_time),
    PRIMARY KEY (staff_id, weekday, start_time)
);

-- An override replaces the weekly hours on one date. An override without
-- intervals closes the day.
CREATE TABLE schedule_overrides (
    staff_id bigint NOT NULL REFERENCES staff (id) ON DELETE CASCADE,
    on_date  date NOT NULL,
    PRIMARY KEY (staff_id, on_date)
);

CREATE TABLE schedule_override_intervals (
    staff_id   bigint NOT NULL,
    on_date    date NOT NULL,
    start_time time NOT NULL,
    end_time   time NOT NULL CHECK (start_time < end_time),
    PRIMARY KEY (staff_id, on_date, start_time),
    FOREIGN KEY (staff_id, on_date) REFERENCES schedule_overrides (staff_id, on_date) ON DELETE CASCADE
);

-- Appointments and admin time blocks share one table so that a single
-- exclusion constraint covers both.
CREATE TABLE bookings (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind              text NOT NULL CHECK (kind IN ('appointment', 'block')),
    status            text NOT NULL CHECK (status IN ('held', 'confirmed', 'cancelled', 'expired')),
    location_id       bigint NOT NULL REFERENCES locations (id),
    staff_id          bigint NOT NULL REFERENCES staff (id),
    service_id        bigint REFERENCES services (id),
    starts_at         timestamptz NOT NULL,
    ends_at           timestamptz NOT NULL,
    -- ends_at plus the service's cleanup buffer.
    busy_until        timestamptz NOT NULL,
    period            tstzrange GENERATED ALWAYS AS (tstzrange(starts_at, busy_until, '[)')) STORED,
    price_minor       bigint,
    currency          text,
    hold_token_hash   bytea UNIQUE,
    hold_expires_at   timestamptz,
    manage_token_hash bytea,
    customer_name     text,
    customer_phone    text,
    customer_email    text,
    telegram_chat_id  bigint,
    note              text NOT NULL DEFAULT '',
    rescheduled_from  uuid REFERENCES bookings (id),
    created_at        timestamptz NOT NULL,
    confirmed_at      timestamptz,
    cancelled_at      timestamptz,
    CONSTRAINT bookings_times_ordered CHECK (starts_at < ends_at AND ends_at <= busy_until),
    CONSTRAINT bookings_kind_fields CHECK (
        (kind = 'appointment' AND service_id IS NOT NULL)
        OR (kind = 'block' AND service_id IS NULL AND status = 'confirmed')
    ),
    CONSTRAINT bookings_hold_expiry CHECK (status <> 'held' OR hold_expires_at IS NOT NULL),
    CONSTRAINT bookings_customer CHECK (
        kind = 'block' OR status IN ('held', 'expired')
        OR (customer_name IS NOT NULL AND customer_phone IS NOT NULL AND manage_token_hash IS NOT NULL)
    ),
    -- The double-booking guarantee: active rows of one staff member never overlap.
    CONSTRAINT bookings_no_overlap EXCLUDE USING gist (staff_id WITH =, period WITH &&)
        WHERE (status IN ('held', 'confirmed'))
);
CREATE INDEX bookings_location_starts_idx ON bookings (location_id, starts_at);
CREATE INDEX bookings_held_expiry_idx ON bookings (hold_expires_at) WHERE status = 'held';

CREATE TABLE notifications (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    booking_id      uuid NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    kind            text NOT NULL CHECK (kind IN ('confirmation', 'reminder_24h', 'reminder_2h', 'cancellation')),
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'sent', 'failed', 'voided', 'skipped')),
    send_at         timestamptz NOT NULL,
    -- When a worker may next pick the row up: send_at at first, then the end
    -- of a claim lease or of a retry backoff.
    next_attempt_at timestamptz NOT NULL,
    attempts        integer NOT NULL DEFAULT 0,
    last_error      text,
    created_at      timestamptz NOT NULL,
    sent_at         timestamptz,
    UNIQUE (booking_id, kind)
);
CREATE INDEX notifications_due_idx ON notifications (next_attempt_at) WHERE status = 'pending';

CREATE TABLE idempotency_keys (
    scope        text NOT NULL,
    key          text NOT NULL,
    request_hash bytea NOT NULL,
    response     jsonb,
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (scope, key)
);
CREATE INDEX idempotency_keys_created_idx ON idempotency_keys (created_at);
