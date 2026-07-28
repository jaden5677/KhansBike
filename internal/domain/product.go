package domain

import (
	"time"

	"github.com/google/uuid"
)

// ProductStatus is the publication lifecycle of a product. needs_review is set
// by the importer on ambiguous rows (e.g. six identical helmet rows whose colour
// was never recorded) so nothing dubious is silently published or discarded.
type ProductStatus string

const (
	StatusDraft        ProductStatus = "draft"
	StatusActive       ProductStatus = "active"
	StatusDiscontinued ProductStatus = "discontinued"
	StatusNeedsReview  ProductStatus = "needs_review"
)

// Valid reports whether s is a known status.
func (s ProductStatus) Valid() bool {
	switch s {
	case StatusDraft, StatusActive, StatusDiscontinued, StatusNeedsReview:
		return true
	default:
		return false
	}
}

// IsPubliclyVisible reports whether a product in this status may appear in public
// listings. Only active products are shown; the public read path filters on this.
func (s ProductStatus) IsPubliclyVisible() bool { return s == StatusActive }

// StockStatus is the availability of a variant. This is a catalogue, not a shop,
// so it is a coarse label ("call to confirm"), never a live quantity.
type StockStatus string

const (
	StockIn           StockStatus = "in_stock"
	StockLow          StockStatus = "low"
	StockOut          StockStatus = "out"
	StockSpecialOrder StockStatus = "special_order"
	StockUnknown      StockStatus = "unknown"
)

// Valid reports whether s is a known stock status.
func (s StockStatus) Valid() bool {
	switch s {
	case StockIn, StockLow, StockOut, StockSpecialOrder, StockUnknown:
		return true
	default:
		return false
	}
}

// Product is a catalogue item. Attributes are the typed EAV source of truth;
// Attrs is the denormalised JSONB projection rebuilt in the same transaction and
// used only as the fast filter path — it is read-only outside the repository.
// A product always has at least one Variant (the default), even when it exposes
// no variant axes, so pricing and SKUs always hang off a variant.
type Product struct {
	ID                  uuid.UUID
	CategoryID          uuid.UUID
	Category            *Category
	BrandID             *uuid.UUID
	Brand               *Brand
	Name                string
	Slug                string
	Summary             *string
	Description         *string
	Status              ProductStatus
	IsFeatured          bool
	RetailPriceIsPublic bool
	Attributes          []AttributeValue
	Attrs               map[string]any
	Variants            []Variant
	Media               []ProductMedia
	CreatedAt           time.Time
	UpdatedAt           time.Time
	PublishedAt         *time.Time
}

// DefaultVariant returns the variant marked default, or the first variant, or
// nil if none exist. Callers that need "the price to show" start here.
func (p *Product) DefaultVariant() *Variant {
	for i := range p.Variants {
		if p.Variants[i].IsDefault {
			return &p.Variants[i]
		}
	}
	if len(p.Variants) > 0 {
		return &p.Variants[0]
	}
	return nil
}

// Variant is a purchasable configuration of a product. SupplierID and
// SupplierItemNo are ADMIN ONLY and must never reach a public DTO. Prices are
// held here and filtered by role (see FilterPublicPrices) before serialisation.
type Variant struct {
	ID             uuid.UUID
	ProductID      uuid.UUID
	SKU            string
	SupplierID     *uuid.UUID // ADMIN ONLY
	SupplierItemNo *string    // ADMIN ONLY
	ModelNo        *string
	NameSuffix     *string
	Position       int
	StockStatus    StockStatus
	IsDefault      bool
	Attributes     []AttributeValue
	Attrs          map[string]any
	Prices         []Price
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
