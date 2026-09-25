package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"
)

// NewToken returns a random opaque token (32 bytes, base64url, 43 chars) for
// a session cookie, device credential or emailed link.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken is the one-way form of a token that the database stores. Tokens
// carry 256 bits of entropy, so a plain SHA-256 (no salt, no stretching) is
// enough: there is nothing to brute-force.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// CSRFToken derives the anti-CSRF token for a session: HMAC-SHA256 of the
// session id under the server's CSRF key. It is stateless (nothing stored)
// yet unforgeable without the key, and it changes with every new session.
func CSRFToken(key []byte, sessionID uuid.UUID) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("csrf:"))
	mac.Write(sessionID[:])
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyCSRFToken reports, in constant time, whether token is the CSRF token
// of sessionID.
func VerifyCSRFToken(key []byte, sessionID uuid.UUID, token string) bool {
	want := CSRFToken(key, sessionID)
	return hmac.Equal([]byte(want), []byte(token))
}

// pairingAlphabet omits look-alike characters (0/O, 1/I/L) so a code can also
// be typed from the screen if a camera is unavailable.
const pairingAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// pairingCodeLen gives about 40 bits of entropy: ample for a code that lives
// ten minutes, is single-use, and is redeemed through a rate-limited endpoint.
const pairingCodeLen = 8

// newPairingCode returns a random pairing code. crypto/rand.Int is used per
// character so every character is uniformly distributed (no modulo bias).
func newPairingCode() (string, error) {
	var b strings.Builder
	max := big.NewInt(int64(len(pairingAlphabet)))
	for i := 0; i < pairingCodeLen; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("auth: generate pairing code: %w", err)
		}
		b.WriteByte(pairingAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// normalizePairingCode accepts a code as a person might type it: any case,
// with spaces or dashes.
func normalizePairingCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(code)))
}
