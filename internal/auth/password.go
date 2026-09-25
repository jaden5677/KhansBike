// Package auth implements identity for the admin surfaces: argon2id password
// hashing, database-backed browser sessions with CSRF tokens, and device
// tokens for the owner's paired phone (bootstrapped by a one-time QR code).
//
// Every credential the server stores is a one-way hash. Session cookies,
// device tokens and pairing redemptions carry random opaque tokens whose
// SHA-256 is what the database holds, so a database leak yields nothing a
// client could present.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters (RFC 9106's second recommended option, 64 MiB). They
// are recorded in each encoded hash, so raising them later only affects new
// hashes; existing ones are upgraded on the next successful login.
const (
	argonMemoryKiB = 64 * 1024
	argonTime      = 3
	argonThreads   = 4
	argonSaltLen   = 16
	argonKeyLen    = 32
)

// Password length bounds. The upper bound stops a multi-megabyte "password"
// from being used to burn CPU in the hash.
const (
	MinPasswordLen = 12
	MaxPasswordLen = 256
)

var errMalformedHash = errors.New("auth: malformed password hash")

// HashPassword returns a PHC-format argon2id hash of password:
// $argon2id$v=19$m=65536,t=3,p=4$<salt>$<key>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// argonParams is the parsed form of an encoded hash.
type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
	salt    []byte
	key     []byte
}

func parseHash(encoded string) (argonParams, error) {
	var p argonParams
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[1] != "argon2id" {
		return p, errMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return p, errMalformedHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return p, errMalformedHash
	}
	var err error
	if p.salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return p, errMalformedHash
	}
	if p.key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil || len(p.key) == 0 {
		return p, errMalformedHash
	}
	return p, nil
}

// VerifyPassword reports whether password matches encoded. The comparison is
// constant-time. A malformed hash is an error, not a mismatch, so a corrupted
// row is noticed instead of silently locking the account.
func VerifyPassword(encoded, password string) (bool, error) {
	p, err := parseHash(encoded)
	if err != nil {
		return false, err
	}
	key := argon2.IDKey([]byte(password), p.salt, p.time, p.memory, p.threads, uint32(len(p.key)))
	return subtle.ConstantTimeCompare(key, p.key) == 1, nil
}

// needsRehash reports whether encoded was made with parameters other than the
// current ones, so a successful login can transparently upgrade it.
func needsRehash(encoded string) bool {
	p, err := parseHash(encoded)
	if err != nil {
		return true
	}
	return p.memory != argonMemoryKiB || p.time != argonTime || p.threads != argonThreads || len(p.key) != argonKeyLen
}

// ValidatePassword enforces the password policy. Length is the policy: NIST
// SP 800-63B recommends length over composition rules.
func ValidatePassword(password string) error {
	switch n := len([]rune(password)); {
	case n < MinPasswordLen:
		return fmt.Errorf("password must be at least %d characters", MinPasswordLen)
	case len(password) > MaxPasswordLen:
		return fmt.Errorf("password must be at most %d bytes", MaxPasswordLen)
	}
	return nil
}
