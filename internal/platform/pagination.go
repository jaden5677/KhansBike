package platform

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidCursor is returned when a pagination cursor cannot be decoded.
// Cursors come straight from query strings, so a malformed one is a client
// error, never a server fault.
var ErrInvalidCursor = errors.New("invalid cursor")

// ClampLimit bounds a client-requested page size. Zero or negative means "not
// specified" and yields def; anything above maxLimit is capped so a single
// request cannot ask for an unbounded page.
func ClampLimit(n, def, maxLimit int) int {
	switch {
	case n <= 0:
		return def
	case n > maxLimit:
		return maxLimit
	default:
		return n
	}
}

// EncodeCursor serialises v (the sort keys of the last row on a page) into an
// opaque, URL-safe token.
//
// Why opaque: clients must treat the cursor as a bookmark, not as data they can
// construct, which leaves the server free to change what it encodes. The token
// is not signed; decoding validates its shape, and a forged cursor can only
// move a caller to a different position in data it may read anyway.
func EncodeCursor(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DecodeCursor parses a token produced by EncodeCursor into v. Every failure
// is reported as ErrInvalidCursor.
func DecodeCursor(token string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return ErrInvalidCursor
	}
	if err := json.Unmarshal(b, v); err != nil {
		return ErrInvalidCursor
	}
	return nil
}
