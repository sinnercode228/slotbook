-- Every booking read looks up the booking that replaced it after a
-- reschedule; without this index that is a scan of the whole table.
CREATE INDEX bookings_rescheduled_from_idx ON bookings (rescheduled_from) WHERE rescheduled_from IS NOT NULL;
