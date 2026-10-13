package httpapi

import (
	"errors"
	"net/http"

	"github.com/sinnercode228/slotbook/internal/booking"
)

// fail writes the JSON error for err. Domain errors map to stable codes the
// clients switch on; anything unexpected is logged and reported as internal.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var (
		apiErr  *apiError
		slotErr *booking.SlotError
		valErr  *booking.ValidationError
	)
	switch {
	case errors.As(err, &apiErr):
		writeError(w, apiErr.status, apiErr.code, apiErr.message)
	case errors.As(err, &slotErr):
		writeError(w, http.StatusUnprocessableEntity, "invalid_slot", slotErr.Error())
	case errors.As(err, &valErr):
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", valErr.Error())
	case errors.Is(err, booking.ErrNotFound), errors.Is(err, booking.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, booking.ErrHoldNotFound):
		writeError(w, http.StatusNotFound, "hold_not_found", err.Error())
	case errors.Is(err, booking.ErrInvalidStaff):
		writeError(w, http.StatusUnprocessableEntity, "invalid_staff", err.Error())
	case errors.Is(err, booking.ErrSlotTaken):
		writeError(w, http.StatusConflict, "slot_taken", err.Error())
	case errors.Is(err, booking.ErrAlreadyConfirmed):
		writeError(w, http.StatusConflict, "already_confirmed", err.Error())
	case errors.Is(err, booking.ErrTooLate):
		writeError(w, http.StatusConflict, "too_late", err.Error())
	case errors.Is(err, booking.ErrNotActive):
		writeError(w, http.StatusConflict, "not_active", err.Error())
	case errors.Is(err, booking.ErrHoldExpired):
		writeError(w, http.StatusGone, "hold_expired", err.Error())
	case errors.Is(err, booking.ErrIdempotencyMismatch):
		writeError(w, http.StatusUnprocessableEntity, "idempotency_key_reused", err.Error())
	default:
		s.logger(r).ErrorContext(r.Context(), "request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}
