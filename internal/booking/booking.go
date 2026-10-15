// Package booking implements holds, confirmations and changes to bookings.
// Overlaps are ruled out by the bookings_no_overlap exclusion constraint;
// this package maps its violations to ErrSlotTaken.
package booking

import (
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/clock"
)

type Kind string

const (
	KindAppointment Kind = "appointment"
	KindBlock       Kind = "block"
)

type Status string

const (
	StatusHeld      Status = "held"
	StatusConfirmed Status = "confirmed"
	StatusCancelled Status = "cancelled"
	StatusExpired   Status = "expired"
	StatusReleased  Status = "released"
)

type Booking struct {
	ID              string     `json:"id"`
	Kind            Kind       `json:"kind"`
	Status          Status     `json:"status"`
	LocationID      int64      `json:"location_id"`
	StaffID         int64      `json:"staff_id"`
	ServiceID       int64      `json:"service_id,omitempty"`
	Start           time.Time  `json:"start"`
	End             time.Time  `json:"end"`
	BusyUntil       time.Time  `json:"busy_until"`
	PriceMinor      int64      `json:"price_minor,omitempty"`
	Currency        string     `json:"currency,omitempty"`
	HoldExpiresAt   *time.Time `json:"hold_expires_at,omitempty"`
	Customer        *Customer  `json:"customer,omitempty"`
	Note            string     `json:"note,omitempty"`
	RescheduledFrom string     `json:"rescheduled_from,omitempty"`
	// RescheduledTo is the booking that replaced this one, if it was moved.
	RescheduledTo string     `json:"rescheduled_to,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ConfirmedAt   *time.Time `json:"confirmed_at,omitempty"`
	CancelledAt   *time.Time `json:"cancelled_at,omitempty"`

	manageTokenHash []byte
}

var (
	ErrNotFound            = errors.New("booking not found")
	ErrServiceNotFound     = errors.New("service not found")
	ErrInvalidStaff        = errors.New("staff member does not offer this service")
	ErrSlotTaken           = errors.New("slot is already taken")
	ErrHoldNotFound        = errors.New("hold not found")
	ErrHoldExpired         = errors.New("hold has expired")
	ErrAlreadyConfirmed    = errors.New("hold is already confirmed")
	ErrTooLate             = errors.New("too late to change this booking")
	ErrNotActive           = errors.New("booking is not active")
	ErrIdempotencyMismatch = errors.New("idempotency key was used with a different request")
)

// SlotError means the requested start is not one the slot generator offers.
type SlotError struct {
	Reason string
}

func (e *SlotError) Error() string { return "start is not bookable: " + e.Reason }

// ValidationError reports a bad input field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Message) }

type Manager struct {
	pool    *pgxpool.Pool
	catalog *catalog.Store
	clock   clock.Clock
}

func NewManager(pool *pgxpool.Pool, cat *catalog.Store, clk clock.Clock) *Manager {
	return &Manager{pool: pool, catalog: cat, clock: clk}
}
