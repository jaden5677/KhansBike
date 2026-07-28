package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/platform"
)

// PriceTier identifies which of the four workbook price tiers a Price row holds.
// The tiering is a load-bearing security boundary, not a display convenience:
// three of the four tiers must never reach a non-admin.
type PriceTier string

const (
	// TierCostUSD is the supplier's price. ADMIN ONLY.
	TierCostUSD PriceTier = "cost_usd"
	// TierLandedTTD is cost plus duty/freight. ADMIN ONLY.
	TierLandedTTD PriceTier = "landed_ttd"
	// TierWholesaleTTD is the trade price. ADMIN ONLY.
	TierWholesaleTTD PriceTier = "wholesale_ttd"
	// TierRetailTTD is the only tier that may ever appear publicly, and then only
	// when the product's RetailPriceIsPublic flag is true.
	TierRetailTTD PriceTier = "retail_ttd"
)

// IsPublic reports whether a tier may ever be serialised into a public response.
// The public read path must call this and nothing else; do not inline the
// comparison elsewhere, so the rule has exactly one definition to audit.
func (t PriceTier) IsPublic() bool { return t == TierRetailTTD }

// Valid reports whether t is a known tier.
func (t PriceTier) Valid() bool {
	switch t {
	case TierCostUSD, TierLandedTTD, TierWholesaleTTD, TierRetailTTD:
		return true
	default:
		return false
	}
}

// Price is one tier of pricing for one variant at one effective date. Amount is
// exact integer-cents Money; the domain never represents money as a float.
type Price struct {
	ID            uuid.UUID
	VariantID     uuid.UUID
	Tier          PriceTier
	Amount        platform.Money
	EffectiveFrom time.Time
	SourceNote    *string
	CreatedAt     time.Time
}

// FilterPublicPrices returns only the prices that a public response may carry:
// the retail tier, and only when the product permits it. It centralises the
// rule so callers cannot accidentally leak a cost by hand-filtering.
func FilterPublicPrices(prices []Price, retailPriceIsPublic bool) []Price {
	if !retailPriceIsPublic {
		return nil
	}
	out := make([]Price, 0, 1)
	for _, p := range prices {
		if p.Tier.IsPublic() {
			out = append(out, p)
		}
	}
	return out
}
