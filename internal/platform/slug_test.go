package platform

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "Star Grips", "star-grips"},
		{"punctuation", "Sprockets/Freewheels/Cassettes", "sprockets-freewheels-cassettes"},
		{"ampersand", "Bells & Horns", "bells-horns"},
		{"quotes_and_size", `20" Tyre`, "20-tyre"},
		{"trailing_space", "Kenda ", "kenda"},
		{"diacritics", "Café Réglage", "cafe-reglage"},
		{"collapse_separators", "a---b__c   d", "a-b-c-d"},
		{"leading_trailing_junk", "  --Hello--  ", "hello"},
		{"already_slug", "star-grips", "star-grips"},
		{"empty", "   ", ""},
		{"fraction", `Pedal 9/16"`, "pedal-9-16"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Slugify(tc.in); got != tc.want {
				t.Errorf("Slugify(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSlugifyIdempotent(t *testing.T) {
	inputs := []string{"Star Grips", "Sprockets/Freewheels", `20" Tyre`, "Café"}
	for _, in := range inputs {
		once := Slugify(in)
		twice := Slugify(once)
		if once != twice {
			t.Errorf("not idempotent: Slugify(%q)=%q, Slugify again=%q", in, once, twice)
		}
	}
}

func TestSlugifyWithSuffix(t *testing.T) {
	if got := SlugifyWithSuffix("Star Grips", 1); got != "star-grips" {
		t.Errorf("suffix 1 = %q, want star-grips", got)
	}
	if got := SlugifyWithSuffix("Star Grips", 2); got != "star-grips-2" {
		t.Errorf("suffix 2 = %q, want star-grips-2", got)
	}
}

func TestUniqueSlug(t *testing.T) {
	taken := map[string]bool{"star-grips": true, "star-grips-2": true}
	got := UniqueSlug("Star Grips", func(c string) bool { return taken[c] })
	if got != "star-grips-3" {
		t.Errorf("UniqueSlug = %q, want star-grips-3", got)
	}
	// First-use stays clean.
	got2 := UniqueSlug("Fresh Name", func(c string) bool { return false })
	if got2 != "fresh-name" {
		t.Errorf("UniqueSlug first use = %q, want fresh-name", got2)
	}
}
