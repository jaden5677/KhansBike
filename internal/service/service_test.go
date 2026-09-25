package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func colourAttr() *domain.Attribute {
	id := uuid.New()
	return &domain.Attribute{
		ID: id, Key: "colour", Label: "Colour", DataType: domain.DataTypeColor, IsFilterable: true,
		Options: []domain.AttributeOption{
			{ID: uuid.New(), AttributeID: id, Value: "black", Label: "Black", SwatchHex: ptr("#000000")},
			{ID: uuid.New(), AttributeID: id, Value: "red", Label: "Red", SwatchHex: ptr("#d32f2f")},
		},
	}
}

func TestDecodeValueRoundTrips(t *testing.T) {
	tests := []struct {
		name    string
		attr    *domain.Attribute
		raw     string
		display string
	}{
		{"text", &domain.Attribute{ID: uuid.New(), Key: "material", DataType: domain.DataTypeText}, `" Alloy "`, "Alloy"},
		{"number", &domain.Attribute{ID: uuid.New(), Key: "speeds", DataType: domain.DataTypeNumber, Unit: ptr("speed")}, `21`, "21 speed"},
		{"range", &domain.Attribute{ID: uuid.New(), Key: "width", DataType: domain.DataTypeNumberRange, Unit: ptr("in")}, `{"low":1.95,"high":2.125}`, "1.95–2.125 in"},
		{"bool", &domain.Attribute{ID: uuid.New(), Key: "tubeless", DataType: domain.DataTypeBoolean}, `true`, "Yes"},
		{"color", colourAttr(), `"red"`, "Red"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := decodeValue(tc.attr, json.RawMessage(tc.raw), nil)
			if err != nil {
				t.Fatalf("decodeValue: %v", err)
			}
			if got := displayValue(tc.attr, v); got != tc.display {
				t.Errorf("display = %q, want %q", got, tc.display)
			}
			// The API value must decode back to the same typed value.
			b, _ := json.Marshal(apiValue(tc.attr, v))
			again, err := decodeValue(tc.attr, b, nil)
			if err != nil {
				t.Fatalf("re-decode %s: %v", b, err)
			}
			if displayValue(tc.attr, again) != tc.display {
				t.Errorf("round trip changed the value: %s", b)
			}
		})
	}
}

func TestDecodeValueRejectsBadShapes(t *testing.T) {
	tests := []struct {
		name string
		attr *domain.Attribute
		raw  string
	}{
		{"unknown_option", colourAttr(), `"purple"`},
		{"option_wrong_type", colourAttr(), `3`},
		{"range_inverted", &domain.Attribute{ID: uuid.New(), DataType: domain.DataTypeNumberRange}, `{"low":3,"high":1}`},
		{"range_missing_high", &domain.Attribute{ID: uuid.New(), DataType: domain.DataTypeNumberRange}, `{"low":1}`},
		{"range_unknown_field", &domain.Attribute{ID: uuid.New(), DataType: domain.DataTypeNumberRange}, `{"low":1,"high":2,"x":3}`},
		{"empty_text", &domain.Attribute{ID: uuid.New(), DataType: domain.DataTypeText}, `"   "`},
		{"bool_as_string", &domain.Attribute{ID: uuid.New(), DataType: domain.DataTypeBoolean}, `"yes"`},
		{"empty_multi", &domain.Attribute{ID: uuid.New(), DataType: domain.DataTypeMultiEnum}, `[]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeValue(tc.attr, json.RawMessage(tc.raw), nil); err == nil {
				t.Errorf("decodeValue(%s) accepted invalid input", tc.raw)
			}
		})
	}
}

func TestFilterPath(t *testing.T) {
	colour := colourAttr()
	width := &domain.Attribute{Key: "width", DataType: domain.DataTypeNumberRange}
	speeds := &domain.Attribute{Key: "speeds", DataType: domain.DataTypeNumber}
	tubeless := &domain.Attribute{Key: "tubeless", DataType: domain.DataTypeBoolean}
	tests := []struct {
		name string
		attr *domain.Attribute
		f    domain.AttributeFilter
		want string
	}{
		{"enum_or", colour, domain.AttributeFilter{Values: []string{"black", "red"}}, `$."colour" ? (@ == "black" || @ == "red")`},
		{"range_overlap", width, domain.AttributeFilter{NumMin: ptr(1.9), NumMax: ptr(2.2)}, `$."width" ? (@.high >= 1.9 && @.low <= 2.2)`},
		{"range_single_value", width, domain.AttributeFilter{Values: []string{"2"}}, `$."width" ? (@.high >= 2 && @.low <= 2)`},
		{"number_min_only", speeds, domain.AttributeFilter{NumMin: ptr(18.0)}, `$."speeds" ? (@ >= 18)`},
		{"bool_from_text", tubeless, domain.AttributeFilter{Values: []string{"true"}}, `$."tubeless" ? (@ == true)`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := filterPath(tc.attr, tc.f)
			if err != nil {
				t.Fatalf("filterPath: %v", err)
			}
			if got != tc.want {
				t.Errorf("filterPath = %s\n want %s", got, tc.want)
			}
		})
	}
}

func TestFilterPathRejectsInjectionAndBadInput(t *testing.T) {
	colour := colourAttr()
	// A value that tries to break out of the string literal must be rejected
	// (it is not an option) rather than spliced into the path.
	if _, err := filterPath(colour, domain.AttributeFilter{Values: []string{`black") || (true`}}); err == nil {
		t.Error("unknown option value accepted")
	}
	speeds := &domain.Attribute{Key: "speeds", DataType: domain.DataTypeNumber}
	for _, f := range []domain.AttributeFilter{
		{},                                   // no bounds
		{NumMin: ptr(5.0), NumMax: ptr(1.0)}, // inverted
		{Values: []string{"NaN"}},            // not finite
	} {
		if _, err := filterPath(speeds, f); err == nil {
			t.Errorf("filter %+v accepted", f)
		}
	}
	// Text values are embedded as escaped JSON string literals.
	text := &domain.Attribute{Key: "model", DataType: domain.DataTypeText}
	got, err := filterPath(text, domain.AttributeFilter{Values: []string{`a"b\c`}})
	if err != nil || got != `$."model" ? (@ == "a\"b\\c")` {
		t.Errorf("text filter = %s, %v", got, err)
	}
}

func TestCheckIfMatch(t *testing.T) {
	at := time.Date(2026, 5, 1, 12, 0, 0, 123456000, time.UTC)
	tag := ETag(at)
	for _, ok := range []string{"", "*", tag, "W/" + tag, `"other", ` + tag} {
		if err := checkIfMatch(ok, at); err != nil {
			t.Errorf("checkIfMatch(%q) = %v, want nil", ok, err)
		}
	}
	if err := checkIfMatch(ETag(at.Add(time.Microsecond)), at); !errors.Is(err, domain.ErrVersionMismatch) {
		t.Errorf("stale tag: err = %v, want ErrVersionMismatch", err)
	}
}

func TestCSVCellNeutralisesFormulas(t *testing.T) {
	for in, want := range map[string]string{
		"=HYPERLINK(\"x\")": "'=HYPERLINK(\"x\")",
		"+1234":             "'+1234",
		"-2":                "'-2",
		"@SUM(A1)":          "'@SUM(A1)",
		"Jane Doe":          "Jane Doe",
		"":                  "",
	} {
		if got := csvCell(in); got != want {
			t.Errorf("csvCell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueSlug(t *testing.T) {
	taken := map[string]bool{"star-grips": true, "star-grips-2": true}
	exists := func(_ context.Context, s string) (bool, error) { return taken[s], nil }
	got, err := uniqueSlug(context.Background(), "Star Grips", exists)
	if err != nil || got != "star-grips-3" {
		t.Errorf("uniqueSlug = %q, %v; want star-grips-3", got, err)
	}
	long, _ := uniqueSlug(context.Background(), strings.Repeat("wheel ", 60), exists)
	if len(long) > maxSlugLen || !slugPattern.MatchString(long) {
		t.Errorf("long name produced invalid slug %q", long)
	}
	boom := errors.New("db down")
	if _, err := uniqueSlug(context.Background(), "x", func(context.Context, string) (bool, error) { return false, boom }); !errors.Is(err, boom) {
		t.Errorf("database error not propagated: %v", err)
	}
}

func TestMergeValuesCombinesMultiEnumRows(t *testing.T) {
	attr, variant := uuid.New(), uuid.New()
	o1, o2 := uuid.New(), uuid.New()
	rows := []domain.AttributeValue{
		{AttributeID: attr, OptionIDs: []uuid.UUID{o1}},
		{AttributeID: attr, OptionIDs: []uuid.UUID{o2}},
		{AttributeID: attr, VariantID: &variant, OptionIDs: []uuid.UUID{o1}}, // same attribute, other level
	}
	got := mergeValues(rows)
	if len(got) != 2 || len(got[0].OptionIDs) != 2 || got[1].VariantID == nil {
		t.Errorf("mergeValues = %+v", got)
	}
}
