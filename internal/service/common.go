// Package service holds the business rules. Services validate input, enforce
// invariants (price visibility, attribute schemas, publication rules), run
// multi-step writes in one transaction with an audit entry, and assemble the
// read models the HTTP layer renders. They speak domain types and domain
// errors; the store translates database errors before they arrive here.
package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/platform"
)

// Background job kinds and their payloads.
const (
	// JobReindexSearch rebuilds product search vectors. Database triggers
	// enqueue it whenever variants or attribute values change; services
	// enqueue it when a name that feeds the vector (category, brand, option
	// label) changes.
	JobReindexSearch = "reindex_search"
	// JobProcessMedia derives renditions and placeholders for an upload.
	JobProcessMedia = "process_media"
	// JobSendConfirmation emails a mailing-list double opt-in link.
	JobSendConfirmation = "send_subscriber_confirmation"
)

// ETag is the entity tag for a row version, derived from its updated_at
// (microsecond precision, as stored by Postgres).
func ETag(updatedAt time.Time) string {
	return `"` + strconv.FormatInt(updatedAt.UnixMicro(), 36) + `"`
}

// checkIfMatch implements optimistic concurrency. An empty If-Match (or "*")
// means the client did not ask for a check. Weak tags are accepted because a
// proxy (Cloudflare, when it compresses a response) may weaken a strong tag
// in transit.
func checkIfMatch(ifMatch string, updatedAt time.Time) error {
	ifMatch = strings.TrimSpace(ifMatch)
	if ifMatch == "" || ifMatch == "*" {
		return nil
	}
	current := ETag(updatedAt)
	for _, tag := range strings.Split(ifMatch, ",") {
		if strings.TrimPrefix(strings.TrimSpace(tag), "W/") == current {
			return nil
		}
	}
	return domain.ErrVersionMismatch
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const maxSlugLen = 120

// validateSlug checks a client-supplied slug.
func validateSlug(v *domain.ValidationError, field, slug string) {
	if len(slug) > maxSlugLen || !slugPattern.MatchString(slug) {
		v.Add(field, "must be lowercase letters, digits and single hyphens (at most %d characters)", maxSlugLen)
	}
}

// uniqueSlug returns the first free slug derived from name: name, name-2,
// name-3, ... (platform.UniqueSlug does the same for in-memory sets; this
// version can report a database error from the existence check).
func uniqueSlug(ctx context.Context, name string, exists func(context.Context, string) (bool, error)) (string, error) {
	base := platform.Slugify(name)
	if len(base) > maxSlugLen-6 {
		base = strings.Trim(base[:maxSlugLen-6], "-")
	}
	for n := 1; ; n++ {
		candidate := platform.SlugifyWithSuffix(base, n)
		if candidate == "" {
			candidate = fmt.Sprintf("item-%d", n)
		}
		taken, err := exists(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("check slug %q: %w", candidate, err)
		}
		if !taken {
			return candidate, nil
		}
	}
}

// requireText trims s and records an error unless 1..maxLen characters remain.
func requireText(v *domain.ValidationError, field, s string, maxLen int) string {
	s = strings.TrimSpace(s)
	switch n := utf8.RuneCountInString(s); {
	case n == 0:
		v.Add(field, "is required")
	case n > maxLen:
		v.Add(field, "must be at most %d characters", maxLen)
	}
	return s
}

// optionalText trims an optional string, treating blank as absent, and
// records an error if it exceeds maxLen characters.
func optionalText(v *domain.ValidationError, field string, s *string, maxLen int) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	if utf8.RuneCountInString(t) > maxLen {
		v.Add(field, "must be at most %d characters", maxLen)
	}
	return &t
}

// deref returns *p, or the zero value for nil.
func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
