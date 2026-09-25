package auth

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Errorf("unexpected encoding: %s", hash)
	}
	if ok, err := VerifyPassword(hash, "correct horse battery staple"); err != nil || !ok {
		t.Errorf("VerifyPassword(correct) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := VerifyPassword(hash, "wrong password entirely"); err != nil || ok {
		t.Errorf("VerifyPassword(wrong) = %v, %v; want false, nil", ok, err)
	}
	if needsRehash(hash) {
		t.Error("fresh hash reported as needing rehash")
	}
}

func TestHashPasswordUsesUniqueSalts(t *testing.T) {
	a, _ := HashPassword("same password here")
	b, _ := HashPassword("same password here")
	if a == b {
		t.Error("two hashes of the same password are identical; salt is not random")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	for _, h := range []string{"", "plaintext", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$a2V5", "$argon2id$v=19$m=x$salt$key"} {
		if _, err := VerifyPassword(h, "whatever"); err == nil {
			t.Errorf("VerifyPassword(%q) returned no error", h)
		}
	}
}

func TestNeedsRehashOnOldParameters(t *testing.T) {
	old := "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0c2FsdA$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2U"
	if !needsRehash(old) {
		t.Error("hash with weaker parameters not flagged for rehash")
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short"); err == nil {
		t.Error("short password accepted")
	}
	if err := ValidatePassword(strings.Repeat("a", MaxPasswordLen+1)); err == nil {
		t.Error("overlong password accepted")
	}
	if err := ValidatePassword("a perfectly long passphrase"); err != nil {
		t.Errorf("valid password rejected: %v", err)
	}
}

func TestLockoutFor(t *testing.T) {
	tests := []struct {
		failures int
		want     time.Duration
	}{
		{1, 0},
		{4, 0},
		{5, time.Minute},
		{6, 2 * time.Minute},
		{7, 4 * time.Minute},
		{12, time.Hour}, // 128m capped
		{1000, time.Hour},
	}
	for _, tc := range tests {
		if got := lockoutFor(tc.failures); got != tc.want {
			t.Errorf("lockoutFor(%d) = %v, want %v", tc.failures, got, tc.want)
		}
	}
}

func TestCSRFToken(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	s1, s2 := uuid.New(), uuid.New()
	tok := CSRFToken(key, s1)
	if !VerifyCSRFToken(key, s1, tok) {
		t.Error("valid CSRF token rejected")
	}
	if VerifyCSRFToken(key, s2, tok) {
		t.Error("CSRF token accepted for a different session")
	}
	if VerifyCSRFToken([]byte("another-key-another-key-another!!"), s1, tok) {
		t.Error("CSRF token accepted under a different key")
	}
}

func TestTokensAreRandomAndHashStable(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()
	if a == b || len(a) != 43 {
		t.Errorf("tokens %q, %q: want distinct 43-char values", a, b)
	}
	h1, h2 := HashToken(a), HashToken(a)
	if !bytes.Equal(h1, h2) || len(h1) != 32 {
		t.Error("HashToken is not a stable 32-byte digest")
	}
}

func TestPairingCode(t *testing.T) {
	code, err := newPairingCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != pairingCodeLen {
		t.Fatalf("code %q has length %d", code, len(code))
	}
	for _, r := range code {
		if !strings.ContainsRune(pairingAlphabet, r) {
			t.Errorf("code %q contains %q outside the alphabet", code, r)
		}
	}
	if got := normalizePairingCode(" abcd-2345 "); got != "ABCD2345" {
		t.Errorf("normalizePairingCode = %q", got)
	}
}

func TestTruncateKeepsValidUTF8(t *testing.T) {
	s := "héllo wörld"
	for n := 0; n <= len(s); n++ {
		if got := truncate(s, n); !utf8.ValidString(got) || len(got) > n {
			t.Errorf("truncate(%q, %d) = %q", s, n, got)
		}
	}
}
