package domain

import (
	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/platform"
)

// AttributeFilter is one facet constraint applied to a product query. It maps to
// a query param like attr.wheel_size=20 or attr.width_min=1.9&attr.width_max=2.2.
// Which fields are set depends on the bound attribute's DataType; the search
// layer translates it into a JSONB containment or numeric-range predicate.
type AttributeFilter struct {
	Key       string   // attribute key, e.g. "wheel_size"
	Values    []string // for enum/multi_enum/color: option values (OR-ed)
	NumMin    *float64 // for number/number_range lower bound
	NumMax    *float64 // for number/number_range upper bound
	BoolValue *bool    // for boolean
}

// ProductQuery is the fully-parsed set of constraints for a product listing.
// Cursor drives keyset pagination; Limit is clamped by the service.
type ProductQuery struct {
	CategorySlug string
	Text         string // free-text q
	BrandSlug    string
	Attributes   []AttributeFilter
	Sort         string // see the Sort* constants; "" picks the default
	Cursor       string
	Limit        int
	// Status narrows admin listings; public listings always show only active
	// products and ignore it.
	Status ProductStatus
}

// Listing sort orders. Relevance needs a text query; Featured is the default
// browse order; Updated is the admin default (most recently edited first).
const (
	SortRelevance = "relevance"
	SortFeatured  = "featured"
	SortName      = "name"
	SortNewest    = "-created"
	SortUpdated   = "-updated"
)

// ProductSummary is a product as it appears in a listing: the product row
// (with Brand and Category populated, no variants) plus the few aggregates a
// card needs, gathered in batch queries rather than per product.
type ProductSummary struct {
	Product      Product
	Cover        *ProductMedia   // first image to show; nil when none is ready
	FromPrice    *platform.Money // lowest current PUBLIC retail price; nil when hidden or unpriced
	Availability StockStatus     // best stock status across the variants
	VariantCount int
}

// ProductPage is one keyset page of a listing. NextCursor is empty on the
// last page; Total counts every match, not just this page.
type ProductPage struct {
	Items      []ProductSummary
	Total      int
	NextCursor string
}

// FacetValue is one selectable option within a facet, with the count of matching
// products under the current filter set, so the UI can show "Black (12)".
type FacetValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// Facet is a filterable attribute plus its available values for the current
// result set. Returned by the category facets endpoint to build filter UIs.
type Facet struct {
	Key      string       `json:"key"`
	Label    string       `json:"label"`
	DataType DataType     `json:"dataType"`
	Values   []FacetValue `json:"values,omitempty"`
	// For numeric facets, the observed bounds across the result set.
	NumMin *float64 `json:"numMin,omitempty"`
	NumMax *float64 `json:"numMax,omitempty"`
}

// SearchGroup is a set of search hits within one category, used by the
// cross-category search endpoint to present per-category groupings.
type SearchGroup struct {
	CategoryID   uuid.UUID        `json:"categoryId"`
	CategoryName string           `json:"categoryName"`
	CategorySlug string           `json:"categorySlug"`
	Total        int              `json:"total"`
	Products     []ProductSummary `json:"-"` // mapped to a public DTO by the transport layer
}

// Suggestion is one trigram-backed typeahead result, kept minimal for latency.
type Suggestion struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}
