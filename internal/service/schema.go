package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/store"
)

// This file is the dynamic attribute system's rulebook. Attribute values
// travel through the API in one JSON shape per data type:
//
//	text          "Aluminium"
//	number        26
//	number_range  {"low": 1.95, "high": 2.125}
//	boolean       true
//	enum, color   "black"                (an option's value token)
//	multi_enum    ["road", "gravel"]     (option value tokens)
//
// The same shapes are what the products.attrs / product_variants.attrs JSONB
// projections store, so the admin API's read format, its write format and the
// filter path all agree.

// maxTextValue bounds a free-text attribute value.
const maxTextValue = 1000

// categorySchema is a category's attribute bindings, each with its options,
// indexed for validating, rendering and filtering product values.
type categorySchema struct {
	bindings []domain.CategoryAttribute // in the admin's order
	byKey    map[string]*domain.CategoryAttribute
	byID     map[uuid.UUID]*domain.CategoryAttribute
}

func loadCategorySchema(ctx context.Context, q *store.Queries, categoryID uuid.UUID) (*categorySchema, error) {
	rows, err := q.ListCategoryAttributes(ctx, categoryID)
	if err != nil {
		return nil, fmt.Errorf("load category attributes: %w", err)
	}
	s := &categorySchema{
		bindings: make([]domain.CategoryAttribute, len(rows)),
		byKey:    make(map[string]*domain.CategoryAttribute, len(rows)),
		byID:     make(map[uuid.UUID]*domain.CategoryAttribute, len(rows)),
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		s.bindings[i] = store.CategoryAttribute(r)
		ids[i] = r.AttributeID
	}
	options, err := loadOptions(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	for i := range s.bindings {
		b := &s.bindings[i]
		b.Attribute.Options = options[b.Attribute.ID]
		s.byKey[b.Attribute.Key] = b
		s.byID[b.Attribute.ID] = b
	}
	return s, nil
}

// loadAttributes returns every attribute with its options, keyed by key: the
// schema for queries not scoped to one category (global search, fitment).
func loadAttributes(ctx context.Context, q *store.Queries) (map[string]*domain.Attribute, error) {
	rows, err := q.ListAttributes(ctx)
	if err != nil {
		return nil, fmt.Errorf("load attributes: %w", err)
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	options, err := loadOptions(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*domain.Attribute, len(rows))
	for _, r := range rows {
		a := store.Attribute(r)
		a.Options = options[a.ID]
		out[a.Key] = &a
	}
	return out, nil
}

func loadOptions(ctx context.Context, q *store.Queries, attributeIDs []uuid.UUID) (map[uuid.UUID][]domain.AttributeOption, error) {
	out := map[uuid.UUID][]domain.AttributeOption{}
	if len(attributeIDs) == 0 {
		return out, nil
	}
	rows, err := q.ListOptionsByAttributeIDs(ctx, attributeIDs)
	if err != nil {
		return nil, fmt.Errorf("load attribute options: %w", err)
	}
	for _, r := range rows {
		out[r.AttributeID] = append(out[r.AttributeID], store.AttributeOption(r))
	}
	return out, nil
}

func optionByValue(a *domain.Attribute, value string) *domain.AttributeOption {
	for i := range a.Options {
		if a.Options[i].Value == value {
			return &a.Options[i]
		}
	}
	return nil
}

func optionByID(a *domain.Attribute, id uuid.UUID) *domain.AttributeOption {
	for i := range a.Options {
		if a.Options[i].ID == id {
			return &a.Options[i]
		}
	}
	return nil
}

// decodeValue parses an API value for attribute a into a typed AttributeValue,
// resolving option tokens to option ids. It reports a client-facing reason on
// failure; the caller attaches the field path.
func decodeValue(a *domain.Attribute, raw json.RawMessage, variantID *uuid.UUID) (domain.AttributeValue, error) {
	var av domain.AttributeValue
	dec := func(dst any) error {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		return d.Decode(dst)
	}
	switch a.DataType {
	case domain.DataTypeText:
		var s string
		if err := dec(&s); err != nil {
			return av, fmt.Errorf("must be a string")
		}
		s = strings.TrimSpace(s)
		if s == "" || utf8.RuneCountInString(s) > maxTextValue {
			return av, fmt.Errorf("must be 1 to %d characters", maxTextValue)
		}
		av.Text = &s
	case domain.DataTypeNumber:
		var f float64
		if err := dec(&f); err != nil || !finite(f) {
			return av, fmt.Errorf("must be a number")
		}
		av.Num = &f
	case domain.DataTypeNumberRange:
		var r struct {
			Low  *float64 `json:"low"`
			High *float64 `json:"high"`
		}
		if err := dec(&r); err != nil || r.Low == nil || r.High == nil || !finite(*r.Low) || !finite(*r.High) {
			return av, fmt.Errorf(`must be an object {"low": number, "high": number}`)
		}
		av.NumLow, av.NumHigh = r.Low, r.High
	case domain.DataTypeBoolean:
		var b bool
		if err := dec(&b); err != nil {
			return av, fmt.Errorf("must be true or false")
		}
		av.Bool = &b
	case domain.DataTypeEnum, domain.DataTypeColor:
		var s string
		if err := dec(&s); err != nil {
			return av, fmt.Errorf("must be one of the attribute's option values")
		}
		opt := optionByValue(a, s)
		if opt == nil {
			return av, fmt.Errorf("%q is not an option of %s", s, a.Key)
		}
		av.OptionIDs = []uuid.UUID{opt.ID}
	case domain.DataTypeMultiEnum:
		var vals []string
		if err := dec(&vals); err != nil {
			return av, fmt.Errorf("must be an array of the attribute's option values")
		}
		for _, s := range vals {
			opt := optionByValue(a, s)
			if opt == nil {
				return av, fmt.Errorf("%q is not an option of %s", s, a.Key)
			}
			if !slices.Contains(av.OptionIDs, opt.ID) {
				av.OptionIDs = append(av.OptionIDs, opt.ID)
			}
		}
	}
	// NewAttributeValue re-checks the one-slot-per-type invariant centrally.
	v, err := domain.NewAttributeValue(a.ID, a.DataType, variantID, av)
	if err != nil {
		return v, fmt.Errorf("is not a valid %s value", a.DataType)
	}
	return v, nil
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// apiValue renders a typed value back into its API/projection JSON shape.
func apiValue(a *domain.Attribute, v domain.AttributeValue) any {
	switch a.DataType {
	case domain.DataTypeText:
		return deref(v.Text)
	case domain.DataTypeNumber:
		return deref(v.Num)
	case domain.DataTypeNumberRange:
		return map[string]float64{"low": deref(v.NumLow), "high": deref(v.NumHigh)}
	case domain.DataTypeBoolean:
		return deref(v.Bool)
	case domain.DataTypeEnum, domain.DataTypeColor:
		if len(v.OptionIDs) == 1 {
			if o := optionByID(a, v.OptionIDs[0]); o != nil {
				return o.Value
			}
		}
		return nil
	case domain.DataTypeMultiEnum:
		vals := make([]string, 0, len(v.OptionIDs))
		for _, o := range a.Options { // option order, not insertion order
			if slices.Contains(v.OptionIDs, o.ID) {
				vals = append(vals, o.Value)
			}
		}
		return vals
	default:
		return nil
	}
}

// displayValue renders a value for people: option labels, formatted numbers
// with their unit, Yes/No.
func displayValue(a *domain.Attribute, v domain.AttributeValue) string {
	withUnit := func(s string) string {
		if a.Unit != nil && *a.Unit != "" {
			return s + " " + *a.Unit
		}
		return s
	}
	switch a.DataType {
	case domain.DataTypeText:
		return deref(v.Text)
	case domain.DataTypeNumber:
		return withUnit(formatNumber(deref(v.Num)))
	case domain.DataTypeNumberRange:
		lo, hi := deref(v.NumLow), deref(v.NumHigh)
		if lo == hi {
			return withUnit(formatNumber(lo))
		}
		return withUnit(formatNumber(lo) + "–" + formatNumber(hi))
	case domain.DataTypeBoolean:
		if deref(v.Bool) {
			return "Yes"
		}
		return "No"
	default: // enum, color, multi_enum
		var labels []string
		for _, o := range a.Options {
			if slices.Contains(v.OptionIDs, o.ID) {
				labels = append(labels, o.Label)
			}
		}
		return strings.Join(labels, ", ")
	}
}

func formatNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// AttributeView is one attribute value resolved against its category schema
// for display: the effective label, the API value and a human rendering.
type AttributeView struct {
	Key       string
	Label     string
	DataType  domain.DataType
	Unit      *string
	Value     any
	Display   string
	SwatchHex *string // colour chip, for single-option color values
}

// mergeValues groups raw EAV rows into one value per (attribute, variant):
// a multi_enum value is stored as one row per selected option.
func mergeValues(rows []domain.AttributeValue) []domain.AttributeValue {
	type key struct {
		attr    uuid.UUID
		variant uuid.UUID
	}
	index := map[key]int{}
	var out []domain.AttributeValue
	for _, r := range rows {
		k := key{attr: r.AttributeID}
		if r.VariantID != nil {
			k.variant = *r.VariantID
		}
		if i, ok := index[k]; ok {
			out[i].OptionIDs = append(out[i].OptionIDs, r.OptionIDs...)
			continue
		}
		index[k] = len(out)
		out = append(out, r)
	}
	return out
}

// views renders the values (all for one product, or all for one variant)
// in the schema's order, skipping any value whose attribute is not bound.
func (s *categorySchema) views(values []domain.AttributeValue) []AttributeView {
	byAttr := make(map[uuid.UUID]domain.AttributeValue, len(values))
	for _, v := range values {
		byAttr[v.AttributeID] = v
	}
	var out []AttributeView
	for i := range s.bindings {
		b := &s.bindings[i]
		v, ok := byAttr[b.Attribute.ID]
		if !ok {
			continue
		}
		view := AttributeView{
			Key:      b.Attribute.Key,
			Label:    b.EffectiveLabel(),
			DataType: b.Attribute.DataType,
			Unit:     b.Attribute.Unit,
			Value:    apiValue(&b.Attribute, v),
			Display:  displayValue(&b.Attribute, v),
		}
		if b.Attribute.DataType == domain.DataTypeColor && len(v.OptionIDs) == 1 {
			if o := optionByID(&b.Attribute, v.OptionIDs[0]); o != nil {
				view.SwatchHex = o.SwatchHex
			}
		}
		out = append(out, view)
	}
	return out
}

// projection builds the JSONB filter projection (key -> API value) for one
// set of values.
func (s *categorySchema) projection(values []domain.AttributeValue) ([]byte, error) {
	m := make(map[string]any, len(values))
	for _, v := range values {
		if b, ok := s.byID[v.AttributeID]; ok {
			m[b.Attribute.Key] = apiValue(&b.Attribute, v)
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode attribute projection: %w", err)
	}
	return b, nil
}

// ---- filters ----------------------------------------------------------------

// filterPath compiles one listing filter into a JSONPath predicate over a
// product's combined attributes (see store.ProductFilter). Values are embedded
// as JSON string literals, which is exactly JSONPath's string syntax, so user
// input cannot alter the path's structure.
//
// Query strings carry text, so a single value is interpreted by data type
// here: attr.tubeless=true is a boolean filter, and attr.width=2 on a numeric
// attribute means "exactly 2" (for a range: "a range containing 2").
func filterPath(a *domain.Attribute, f domain.AttributeFilter) (string, error) {
	key := jsonString(a.Key)
	switch a.DataType {
	case domain.DataTypeBoolean:
		if f.BoolValue == nil && len(f.Values) == 1 {
			b, err := strconv.ParseBool(f.Values[0])
			if err != nil {
				return "", fmt.Errorf("needs true or false")
			}
			f.BoolValue = &b
		}
	case domain.DataTypeNumber, domain.DataTypeNumberRange:
		if f.NumMin == nil && f.NumMax == nil && len(f.Values) == 1 {
			n, err := strconv.ParseFloat(f.Values[0], 64)
			if err != nil || !finite(n) {
				return "", fmt.Errorf("needs a number")
			}
			f.NumMin, f.NumMax = &n, &n
		}
	}
	switch a.DataType {
	case domain.DataTypeEnum, domain.DataTypeColor, domain.DataTypeMultiEnum:
		if len(f.Values) == 0 {
			return "", fmt.Errorf("needs one or more option values")
		}
		conds := make([]string, len(f.Values))
		for i, val := range f.Values {
			if optionByValue(a, val) == nil {
				return "", fmt.Errorf("%q is not an option of %s", val, a.Key)
			}
			conds[i] = "@ == " + jsonString(val) // lax mode also matches inside multi_enum arrays
		}
		return "$." + key + " ? (" + strings.Join(conds, " || ") + ")", nil
	case domain.DataTypeBoolean:
		if f.BoolValue == nil {
			return "", fmt.Errorf("needs true or false")
		}
		return "$." + key + " ? (@ == " + strconv.FormatBool(*f.BoolValue) + ")", nil
	case domain.DataTypeText:
		if len(f.Values) != 1 {
			return "", fmt.Errorf("needs exactly one value")
		}
		return "$." + key + " ? (@ == " + jsonString(f.Values[0]) + ")", nil
	case domain.DataTypeNumber:
		conds, err := boundConds("@", "@", f)
		if err != nil {
			return "", err
		}
		return "$." + key + " ? (" + conds + ")", nil
	case domain.DataTypeNumberRange:
		// A stored range matches when it overlaps the requested one: a 20"
		// wheel taking 1.95-2.125 fits a request for "about 2.0 wide".
		conds, err := boundConds("@.high", "@.low", f)
		if err != nil {
			return "", err
		}
		return "$." + key + " ? (" + conds + ")", nil
	default:
		return "", fmt.Errorf("cannot be filtered")
	}
}

// boundConds renders min/max bounds. For a plain number both operands are
// "@"; for a range, the value's high end is compared with the minimum and its
// low end with the maximum (interval overlap).
func boundConds(highOperand, lowOperand string, f domain.AttributeFilter) (string, error) {
	lo, hi := f.NumMin, f.NumMax
	if lo == nil && hi == nil {
		return "", fmt.Errorf("needs a _min and/or _max bound")
	}
	if lo != nil && hi != nil && *lo > *hi {
		return "", fmt.Errorf("_min must not exceed _max")
	}
	var conds []string
	if lo != nil {
		conds = append(conds, highOperand+" >= "+formatNumber(*lo))
	}
	if hi != nil {
		conds = append(conds, lowOperand+" <= "+formatNumber(*hi))
	}
	return strings.Join(conds, " && "), nil
}

// jsonString encodes s as a JSON (and therefore JSONPath) string literal.
func jsonString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // encoding a string cannot fail
	return strings.TrimSuffix(buf.String(), "\n")
}
