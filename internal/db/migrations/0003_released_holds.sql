-- A customer who goes back and picks another time gives the first hold back
-- (DELETE /v1/holds/{id}); the row stays as history with status 'released'.
-- bookings_no_overlap covers only 'held' and 'confirmed', so a released row
-- already blocks nothing. Like an expired hold, it has no customer details.
ALTER TABLE bookings
    DROP CONSTRAINT bookings_status_check,
    ADD CONSTRAINT bookings_status_check
        CHECK (status IN ('held', 'confirmed', 'cancelled', 'expired', 'released')),
    DROP CONSTRAINT bookings_customer,
    ADD CONSTRAINT bookings_customer CHECK (
        kind = 'block' OR status IN ('held', 'expired', 'released')
        OR (customer_name IS NOT NULL AND customer_phone IS NOT NULL AND manage_token_hash IS NOT NULL)
    );
