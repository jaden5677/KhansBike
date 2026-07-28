// Package platform holds small, dependency-free building blocks shared across
// the codebase: money, slugs, pagination, and validation helpers. Nothing here
// imports another internal package, so it can be used from domain, store, and
// http without creating cycles.
package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Money is an exact monetary amount stored as an integer number of minor units
// (cents) plus an ISO-4217 currency code.
//
// Why not float64: the source workbook already shows the damage floating point
// does to money (4.4000000000000004). Currency arithmetic must be exact, so the
// amount is an int64 of minor units and every operation is integer or rational
// arithmetic. Money marshals to a decimal *string*, never a JSON number, so no
// consumer can reintroduce float rounding on the wire.
type Money struct {
	amountMinor int64
	currency    string // 3-letter ISO code, upper-case; "" only for the zero value
}

// ErrCurrencyMismatch is returned when two Money values of different currencies
// are combined; there is no exchange-rate logic in this system.
var ErrCurrencyMismatch = errors.New("money: currency mismatch")

// NewMoney constructs a Money from minor units (cents) and a currency code.
// The currency is normalised to upper case. It does not validate the code
// against the ISO register — callers pass "USD" or "TTD".
func NewMoney(amountMinor int64, currency string) Money {
	return Money{amountMinor: amountMinor, currency: strings.ToUpper(strings.TrimSpace(currency))}
}

// AmountMinor returns the raw integer amount in minor units, for persistence.
func (m Money) AmountMinor() int64 { return m.amountMinor }

// Currency returns the ISO currency code.
func (m Money) Currency() string { return m.currency }

// IsZero reports whether the amount is zero. It ignores currency so the Money
// zero value is usefully "nothing".
func (m Money) IsZero() bool { return m.amountMinor == 0 }

// sameCurrency guards binary operations. Two zero-currency values, or a value
// combined with a zero-amount value of empty currency, are treated as
// compatible so the additive identity works without a currency.
func (m Money) sameCurrency(o Money) bool {
	if m.currency == o.currency {
		return true
	}
	return (m.currency == "" && m.amountMinor == 0) || (o.currency == "" && o.amountMinor == 0)
}

func (m Money) resolveCurrency(o Money) string {
	if m.currency != "" {
		return m.currency
	}
	return o.currency
}

// Add returns m+o. It errors on a currency mismatch rather than silently
// producing a nonsense total.
func (m Money) Add(o Money) (Money, error) {
	if !m.sameCurrency(o) {
		return Money{}, fmt.Errorf("%w: %s + %s", ErrCurrencyMismatch, m.currency, o.currency)
	}
	return Money{amountMinor: m.amountMinor + o.amountMinor, currency: m.resolveCurrency(o)}, nil
}

// Sub returns m-o, erroring on a currency mismatch.
func (m Money) Sub(o Money) (Money, error) {
	if !m.sameCurrency(o) {
		return Money{}, fmt.Errorf("%w: %s - %s", ErrCurrencyMismatch, m.currency, o.currency)
	}
	return Money{amountMinor: m.amountMinor - o.amountMinor, currency: m.resolveCurrency(o)}, nil
}

// MulRatio multiplies the amount by num/den using big.Int and rounds half-up to
// the nearest minor unit. It is used for duty/markup style calculations where a
// fractional multiplier must be applied without ever leaving integer money. It
// errors on a zero denominator.
func (m Money) MulRatio(num, den int64) (Money, error) {
	if den == 0 {
		return Money{}, errors.New("money: division by zero in MulRatio")
	}
	// value = amount * num / den, rounded half away from zero.
	prod := new(big.Int).Mul(big.NewInt(m.amountMinor), big.NewInt(num))
	d := big.NewInt(den)
	q := new(big.Int)
	r := new(big.Int)
	q.QuoRem(prod, d, r)
	// Round half away from zero: compare 2*|r| against |den|.
	twoRem := new(big.Int).Abs(r)
	twoRem.Lsh(twoRem, 1)
	absDen := new(big.Int).Abs(d)
	if twoRem.Cmp(absDen) >= 0 {
		if (prod.Sign() < 0) != (den < 0) {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	if !q.IsInt64() {
		return Money{}, fmt.Errorf("money: MulRatio overflow")
	}
	return Money{amountMinor: q.Int64(), currency: m.currency}, nil
}

// String renders the amount as a fixed two-decimal string with the currency
// suffix, e.g. "149.99 TTD". It assumes 2 minor digits, which holds for USD and
// TTD; this system handles no zero- or three-decimal currencies.
func (m Money) String() string {
	return m.Decimal() + " " + m.currency
}

// Decimal returns just the numeric part as a two-decimal string, e.g. "149.99".
func (m Money) Decimal() string {
	neg := m.amountMinor < 0
	v := m.amountMinor
	if neg {
		v = -v
	}
	whole := v / 100
	frac := v % 100
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%02d", sign, whole, frac)
}

// MarshalJSON emits an object with a decimal string amount and the currency,
// deliberately not a bare number, so the exact value survives the round trip and
// no client re-parses it as a float.
func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"amount":%q,"currency":%q}`, m.Decimal(), m.currency)), nil
}

// UnmarshalJSON parses the object form produced by MarshalJSON.
func (m *Money) UnmarshalJSON(b []byte) error {
	var raw struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	minor, err := parseDecimalToMinor(raw.Amount)
	if err != nil {
		return err
	}
	m.amountMinor = minor
	m.currency = strings.ToUpper(raw.Currency)
	return nil
}

// parseDecimalToMinor converts a decimal string like "149.9" or "149.99" into
// integer cents. It rejects more than two fractional digits rather than
// silently truncating value.
func parseDecimalToMinor(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("money: empty amount")
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	parts := strings.SplitN(s, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("money: invalid whole part %q: %w", parts[0], err)
	}
	var frac int64
	if len(parts) == 2 {
		fracStr := parts[1]
		if len(fracStr) > 2 {
			return 0, fmt.Errorf("money: more than 2 decimal places in %q", s)
		}
		for len(fracStr) < 2 {
			fracStr += "0"
		}
		frac, err = strconv.ParseInt(fracStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("money: invalid fraction %q: %w", parts[1], err)
		}
	}
	minor := whole*100 + frac
	if neg {
		minor = -minor
	}
	return minor, nil
}
