package platform

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestMoneyDecimalAndString(t *testing.T) {
	tests := []struct {
		name        string
		minor       int64
		currency    string
		wantDecimal string
		wantString  string
	}{
		{"whole", 15000, "TTD", "150.00", "150.00 TTD"},
		{"cents", 14999, "TTD", "149.99", "149.99 TTD"},
		{"single_cent", 1, "USD", "0.01", "0.01 USD"},
		{"zero", 0, "USD", "0.00", "0.00 USD"},
		{"negative", -12345, "USD", "-123.45", "-123.45 USD"},
		{"under_a_dollar", 5, "TTD", "0.05", "0.05 TTD"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMoney(tc.minor, tc.currency)
			if got := m.Decimal(); got != tc.wantDecimal {
				t.Errorf("Decimal() = %q, want %q", got, tc.wantDecimal)
			}
			if got := m.String(); got != tc.wantString {
				t.Errorf("String() = %q, want %q", got, tc.wantString)
			}
		})
	}
}

func TestMoneyAddSub(t *testing.T) {
	a := NewMoney(10000, "TTD")
	b := NewMoney(2550, "TTD")
	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if sum.AmountMinor() != 12550 {
		t.Errorf("Add amount = %d, want 12550", sum.AmountMinor())
	}
	diff, err := a.Sub(b)
	if err != nil {
		t.Fatalf("Sub: %v", err)
	}
	if diff.AmountMinor() != 7450 {
		t.Errorf("Sub amount = %d, want 7450", diff.AmountMinor())
	}
}

func TestMoneyCurrencyMismatch(t *testing.T) {
	usd := NewMoney(100, "USD")
	ttd := NewMoney(100, "TTD")
	if _, err := usd.Add(ttd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Add mismatch err = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := usd.Sub(ttd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Sub mismatch err = %v, want ErrCurrencyMismatch", err)
	}
	// Zero identity of empty currency is compatible with anything.
	if _, err := usd.Add(Money{}); err != nil {
		t.Errorf("Add zero identity: unexpected err %v", err)
	}
}

func TestMoneyMulRatio(t *testing.T) {
	tests := []struct {
		name      string
		minor     int64
		num, den  int64
		want      int64
		wantError bool
	}{
		{"apply_15pct_duty", 10000, 115, 100, 11500, false},
		{"round_half_up", 101, 1, 2, 51, false},   // 50.5 -> 51
		{"round_down", 100, 1, 3, 33, false},      // 33.33 -> 33
		{"round_up", 200, 1, 3, 67, false},        // 66.66 -> 67
		{"negative_round", -101, 1, 2, -51, false}, // -50.5 -> -51 (away from zero)
		{"div_zero", 100, 1, 0, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewMoney(tc.minor, "USD").MulRatio(tc.num, tc.den)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.AmountMinor() != tc.want {
				t.Errorf("MulRatio = %d, want %d", got.AmountMinor(), tc.want)
			}
		})
	}
}

func TestMoneyJSONRoundTrip(t *testing.T) {
	m := NewMoney(149900, "TTD") // 1499.00
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"amount":"1499.00","currency":"TTD"}`
	if string(b) != want {
		t.Errorf("marshal = %s, want %s", b, want)
	}
	// Must not serialise as a bare float number.
	var probe any
	_ = json.Unmarshal(b, &probe)
	if _, isNumber := probe.(float64); isNumber {
		t.Error("Money marshalled as a JSON number; must be an object with a string amount")
	}
	var back Money
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.AmountMinor() != m.AmountMinor() || back.Currency() != m.Currency() {
		t.Errorf("round trip = %v, want %v", back, m)
	}
}

func TestParseDecimalToMinor(t *testing.T) {
	tests := []struct {
		in        string
		want      int64
		wantError bool
	}{
		{"149.99", 14999, false},
		{"149.9", 14990, false},
		{"149", 14900, false},
		{"0.01", 1, false},
		{"-5.50", -550, false},
		{"1.234", 0, true}, // too many decimals
		{"", 0, true},
		{"abc", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseDecimalToMinor(tc.in)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("parseDecimalToMinor(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
