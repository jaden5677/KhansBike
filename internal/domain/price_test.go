package domain

import (
	"testing"

	"github.com/khansbikezone/bikezone-api/internal/platform"
)

func TestPriceTierIsPublic(t *testing.T) {
	tests := []struct {
		tier PriceTier
		want bool
	}{
		{TierCostUSD, false},
		{TierLandedTTD, false},
		{TierWholesaleTTD, false},
		{TierRetailTTD, true},
	}
	for _, tc := range tests {
		if got := tc.tier.IsPublic(); got != tc.want {
			t.Errorf("%s.IsPublic() = %v, want %v", tc.tier, got, tc.want)
		}
	}
}

func TestFilterPublicPrices(t *testing.T) {
	all := []Price{
		{Tier: TierCostUSD, Amount: platform.NewMoney(500, "USD")},
		{Tier: TierLandedTTD, Amount: platform.NewMoney(4000, "TTD")},
		{Tier: TierWholesaleTTD, Amount: platform.NewMoney(6000, "TTD")},
		{Tier: TierRetailTTD, Amount: platform.NewMoney(9000, "TTD")},
	}

	// Flag off: nothing is public, even the retail tier.
	if got := FilterPublicPrices(all, false); len(got) != 0 {
		t.Errorf("flag off returned %d prices, want 0", len(got))
	}

	// Flag on: exactly the retail tier, and nothing else.
	got := FilterPublicPrices(all, true)
	if len(got) != 1 {
		t.Fatalf("flag on returned %d prices, want 1", len(got))
	}
	if got[0].Tier != TierRetailTTD {
		t.Errorf("public price tier = %s, want retail_ttd", got[0].Tier)
	}
}
