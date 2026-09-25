package domain

import "github.com/google/uuid"

// NewID returns a new UUIDv7. v7 ids are time-ordered, so primary-key indexes
// stay compact and ids sort by creation time (the importer relies on this to
// keep staged rows in workbook order).
func NewID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		// NewV7 fails only if the system entropy source does.
		return uuid.New()
	}
	return id
}
