package domain

import (
	"time"

	"github.com/google/uuid"
)

// Category is a node in the catalogue taxonomy. The 33 workbook sheets map to
// top-level categories; Bikes is added as a first-class category even though it
// has no sheet. Path is an ltree materialised path (e.g. "bikes.tyres") enabling
// single-query subtree reads. A category's attribute set is defined by its
// CategoryAttribute bindings, which is what lets the owner reshape a category's
// fields entirely from the admin UI with no code change.
type Category struct {
	ID          uuid.UUID
	ParentID    *uuid.UUID
	Name        string
	Slug        string
	Path        string // ltree text form
	Position    int
	Description *string
	HeroAssetID *uuid.UUID
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Children is populated only when a tree is requested; nil otherwise.
	Children []Category
	// ProductCount is populated for tree/listing responses; zero otherwise.
	ProductCount int
}
