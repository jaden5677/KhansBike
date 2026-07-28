package domain

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestNewAttributeValue(t *testing.T) {
	attrID := uuid.New()
	s := "black"
	n := 2.0
	lo, hi := 1.95, 2.125
	b := true
	opt := uuid.New()

	tests := []struct {
		name    string
		dt      DataType
		in      AttributeValue
		wantErr bool
	}{
		{"text_ok", DataTypeText, AttributeValue{Text: &s}, false},
		{"text_missing", DataTypeText, AttributeValue{}, true},
		{"number_ok", DataTypeNumber, AttributeValue{Num: &n}, false},
		{"number_missing", DataTypeNumber, AttributeValue{}, true},
		{"range_ok", DataTypeNumberRange, AttributeValue{NumLow: &lo, NumHigh: &hi}, false},
		{"range_half", DataTypeNumberRange, AttributeValue{NumLow: &lo}, true},
		{"range_inverted", DataTypeNumberRange, AttributeValue{NumLow: &hi, NumHigh: &lo}, true},
		{"bool_ok", DataTypeBoolean, AttributeValue{Bool: &b}, false},
		{"bool_missing", DataTypeBoolean, AttributeValue{}, true},
		{"enum_ok", DataTypeEnum, AttributeValue{OptionIDs: []uuid.UUID{opt}}, false},
		{"enum_none", DataTypeEnum, AttributeValue{}, true},
		{"enum_multiple_rejected", DataTypeEnum, AttributeValue{OptionIDs: []uuid.UUID{opt, uuid.New()}}, true},
		{"color_ok", DataTypeColor, AttributeValue{OptionIDs: []uuid.UUID{opt}}, false},
		{"multi_ok", DataTypeMultiEnum, AttributeValue{OptionIDs: []uuid.UUID{opt, uuid.New()}}, false},
		{"multi_none", DataTypeMultiEnum, AttributeValue{}, true},
		{"unknown_type", DataType("bogus"), AttributeValue{Text: &s}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAttributeValue(attrID, tc.dt, nil, tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, ErrValidation) {
					t.Errorf("error = %v, want wrapped ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDataTypeHelpers(t *testing.T) {
	if !DataTypeEnum.UsesOptions() || !DataTypeColor.UsesOptions() || !DataTypeMultiEnum.UsesOptions() {
		t.Error("enum/color/multi_enum must use options")
	}
	if DataTypeText.UsesOptions() || DataTypeNumber.UsesOptions() {
		t.Error("text/number must not use options")
	}
	if DataType("nope").Valid() {
		t.Error("bogus data type reported valid")
	}
}
