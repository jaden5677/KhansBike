package domain

import (
	"time"

	"github.com/google/uuid"
)

// SubscriberStatus is the double opt-in lifecycle of a mailing-list address.
// A signup starts Pending and only becomes Confirmed after the emailed link is
// clicked; Unsubscribed is terminal until the person re-subscribes.
type SubscriberStatus string

const (
	SubscriberPending      SubscriberStatus = "pending"
	SubscriberConfirmed    SubscriberStatus = "confirmed"
	SubscriberUnsubscribed SubscriberStatus = "unsubscribed"
)

// Valid reports whether s is a known status.
func (s SubscriberStatus) Valid() bool {
	switch s {
	case SubscriberPending, SubscriberConfirmed, SubscriberUnsubscribed:
		return true
	default:
		return false
	}
}

// Subscriber is one mailing-list address. It is the only customer-owned record
// in the system. Token hashes are SHA-256 of the values emailed to the customer;
// the raw tokens are never stored, so the confirm and unsubscribe links cannot
// be reconstructed from the database.
type Subscriber struct {
	ID                   uuid.UUID
	Email                string
	Name                 *string
	Status               SubscriberStatus
	ConfirmTokenHash     []byte
	ConfirmExpiresAt     *time.Time
	UnsubscribeTokenHash []byte
	Source               *string
	ConfirmedAt          *time.Time
	UnsubscribedAt       *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// IsConfirmed reports whether this address has completed double opt-in and may
// be included in a mailing-list export.
func (s *Subscriber) IsConfirmed() bool { return s.Status == SubscriberConfirmed }
