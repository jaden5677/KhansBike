package platform

import (
	"errors"
	"testing"
	"time"
)

func TestClampLimit(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"unset_uses_default", 0, 24},
		{"negative_uses_default", -5, 24},
		{"within_bounds", 10, 10},
		{"capped", 1000, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClampLimit(tc.in, 24, 100); got != tc.want {
				t.Errorf("ClampLimit(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestCursorRoundTrip(t *testing.T) {
	type key struct {
		Name string    `json:"n"`
		At   time.Time `json:"t"`
	}
	in := key{Name: "Star Grips", At: time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC)}
	tok, err := EncodeCursor(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var out key
	if err := DecodeCursor(tok, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Name != in.Name || !out.At.Equal(in.At) {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	var v struct{}
	for _, tok := range []string{"%%%", "bm90LWpzb24"} { // not base64; base64 of "not-json"
		if err := DecodeCursor(tok, &v); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("DecodeCursor(%q) = %v, want ErrInvalidCursor", tok, err)
		}
	}
}
