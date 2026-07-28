package platform

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Slugify converts an arbitrary string into a URL-safe slug: lower-case ASCII
// words separated by single hyphens, with diacritics folded to their base
// letters. It is deterministic and idempotent (Slugify(Slugify(x)) == Slugify(x)).
//
// Why fold diacritics: product and brand names may contain accented characters;
// folding keeps slugs ASCII and stable while remaining recognisable.
func Slugify(s string) string {
	// Decompose accented runes (é -> e + combining mark), then drop the marks.
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	folded, _, err := transform.String(t, s)
	if err != nil {
		folded = s // fall back to the raw string; the pass below still sanitises it
	}

	var b strings.Builder
	prevHyphen := false
	for _, r := range folded {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
			prevHyphen = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevHyphen = false
		default:
			// Any run of non-alphanumerics collapses to a single hyphen.
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// SlugifyWithSuffix appends a numeric suffix to a base slug for collision
// resolution, e.g. SlugifyWithSuffix("star-grips", 2) == "star-grips-2". A
// suffix <= 1 returns the bare slug, so the first occurrence stays clean.
func SlugifyWithSuffix(s string, n int) string {
	base := Slugify(s)
	if n <= 1 {
		return base
	}
	if base == "" {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%s-%d", base, n)
}

// UniqueSlug returns the first slug derived from name that is not already
// present in exists. It probes name, name-2, name-3, ... The exists check is a
// caller-supplied closure so this works against an in-memory set or a database
// uniqueness query without this package importing either.
func UniqueSlug(name string, exists func(candidate string) bool) string {
	for n := 1; ; n++ {
		candidate := SlugifyWithSuffix(name, n)
		if candidate == "" {
			candidate = fmt.Sprintf("item-%d", n)
		}
		if !exists(candidate) {
			return candidate
		}
	}
}
