package importer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/platform"
)

// Cell parsers. Spreadsheet data is typed by hand, so these are forgiving
// about presentation (case, spacing, units, inch marks) and strict about
// meaning: anything they cannot interpret becomes an issue for a person to
// review, never a guess.

// collapse trims a cell and collapses internal runs of whitespace.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// parseStock reads a stock cell: a quantity or a word. ok is false for text
// it does not recognise.
func parseStock(cell string) (status domain.StockStatus, ok bool) {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return domain.StockUnknown, true
	}
	if n, err := strconv.ParseFloat(cell, 64); err == nil {
		if n > 0 {
			return domain.StockIn, true
		}
		return domain.StockOut, true
	}
	switch platform.Slugify(cell) {
	case "in-stock", "in", "yes", "y", "available", "instock":
		return domain.StockIn, true
	case "low", "low-stock", "limited":
		return domain.StockLow, true
	case "out", "out-of-stock", "no", "n", "none", "sold-out", "oos":
		return domain.StockOut, true
	case "special-order", "order", "on-order", "to-order":
		return domain.StockSpecialOrder, true
	}
	return domain.StockUnknown, false
}

var (
	// A number with an optional trailing unit: 26, 26in, 26", 2.125 mm.
	numberCell = regexp.MustCompile(`^(-?\d+(?:\.\d+)?)\s*(?:[a-zA-Z"']+)?$`)
	// A range: 1.95/2.125, 1.95-2.125, 1.95 – 2.125, 1.95 to 2.125, with an optional unit.
	rangeCell = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(?:/|-|–|to)\s*(\d+(?:\.\d+)?)\s*(?:[a-zA-Z"']+)?$`)
)

// parseAttributeCell converts a cell into the attribute's API JSON value.
func parseAttributeCell(a *domain.Attribute, cell string) (json.RawMessage, error) {
	cell = collapse(cell)
	var v any
	switch a.DataType {
	case domain.DataTypeText:
		v = cell
	case domain.DataTypeNumber:
		m := numberCell.FindStringSubmatch(cell)
		if m == nil {
			return nil, fmt.Errorf("%q is not a number", cell)
		}
		v, _ = strconv.ParseFloat(m[1], 64)
	case domain.DataTypeNumberRange:
		var lo, hi float64
		if m := rangeCell.FindStringSubmatch(cell); m != nil {
			lo, _ = strconv.ParseFloat(m[1], 64)
			hi, _ = strconv.ParseFloat(m[2], 64)
		} else if m := numberCell.FindStringSubmatch(cell); m != nil {
			lo, _ = strconv.ParseFloat(m[1], 64)
			hi = lo
		} else {
			return nil, fmt.Errorf("%q is not a number or a range like 1.95/2.125", cell)
		}
		if lo > hi {
			return nil, fmt.Errorf("%q has its larger number first", cell)
		}
		v = map[string]float64{"low": lo, "high": hi}
	case domain.DataTypeBoolean:
		switch strings.ToLower(cell) {
		case "yes", "y", "true", "1", "x":
			v = true
		case "no", "n", "false", "0":
			v = false
		default:
			return nil, fmt.Errorf("%q is not yes or no", cell)
		}
	case domain.DataTypeEnum, domain.DataTypeColor:
		o, err := matchOption(a, cell)
		if err != nil {
			return nil, err
		}
		v = o.Value
	case domain.DataTypeMultiEnum:
		var vals []string
		for _, part := range strings.FieldsFunc(cell, func(r rune) bool { return strings.ContainsRune(",;|", r) }) {
			o, err := matchOption(a, collapse(part))
			if err != nil {
				return nil, err
			}
			vals = append(vals, o.Value)
		}
		if len(vals) == 0 {
			return nil, fmt.Errorf("no values")
		}
		v = vals
	default:
		return nil, fmt.Errorf("unsupported attribute type %s", a.DataType)
	}
	return json.Marshal(v)
}

// matchOption finds the option a cell names by value or label, ignoring case,
// punctuation and spacing (so `20"`, "20" and "20 inch" all find value "20"
// when the label is `20"`). When nothing matches it suggests the closest
// option, catching typos such as "Blck".
func matchOption(a *domain.Attribute, cell string) (*domain.AttributeOption, error) {
	want := platform.Slugify(cell)
	for i := range a.Options {
		o := &a.Options[i]
		if o.Value == cell || platform.Slugify(o.Value) == want || platform.Slugify(o.Label) == want {
			return o, nil
		}
	}
	if s := strings.TrimSuffix(want, "-inch"); s != want { // "20 inch" -> "20"
		for i := range a.Options {
			if platform.Slugify(a.Options[i].Value) == s {
				return &a.Options[i], nil
			}
		}
	}
	// Ignoring punctuation entirely ("A/V" is value "av") is looser, so it is
	// only trusted when it singles out one option.
	var loose *domain.AttributeOption
	for i := range a.Options {
		if o := &a.Options[i]; compact(o.Value) == compact(cell) || compact(o.Label) == compact(cell) {
			if loose != nil && loose != o {
				loose = nil
				break
			}
			loose = o
		}
	}
	if loose != nil {
		return loose, nil
	}
	best, bestDist := "", -1
	for _, o := range a.Options {
		for _, candidate := range []string{o.Value, o.Label} {
			if d := levenshtein(want, platform.Slugify(candidate)); bestDist < 0 || d < bestDist {
				best, bestDist = o.Label, d
			}
		}
	}
	if best != "" && bestDist <= max(2, len(want)/3) {
		return nil, fmt.Errorf("%q is not an option of %s; did you mean %q?", cell, a.Label, best)
	}
	return nil, fmt.Errorf("%q is not an option of %s", cell, a.Label)
}

// compact lower-cases s and drops everything but letters and digits.
func compact(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// levenshtein is the edit distance between two short strings.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
