package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// ArgonParams configures argon2id hashing. Defaults follow OWASP recommendations.
type ArgonParams struct {
	MemoryKiB   uint32
	Iterations  uint32
	SaltLen     uint32
	KeyLen      uint32
	Parallelism uint8
}

func DefaultArgonParams() ArgonParams {
	return ArgonParams{MemoryKiB: 65536, Iterations: 3, Parallelism: 2, SaltLen: 16, KeyLen: 32}
}

// HashPassword returns a PHC-encoded argon2id hash: $argon2id$v=19$m=...,t=...,p=...$salt$key
func (s *Service) HashPassword(pw string) (string, error) {
	p := s.Argon
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, p.Iterations, p.MemoryKiB, p.Parallelism, p.KeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.MemoryKiB, p.Iterations, p.Parallelism, b64(salt), b64(key)), nil
}

// VerifyPassword checks pw against an argon2id (PHC) or bcrypt hash.
// needsRehash is true when a successfully verified hash should be upgraded to
// the service's configured params: always for bcrypt, and for argon2id when
// the PHC-embedded params differ from s.Argon (e.g. a deployment raised
// argon2.memory_kib after hashes were created).
func (s *Service) VerifyPassword(pw, encoded string) (ok bool, needsRehash bool, err error) {
	switch {
	case strings.HasPrefix(encoded, "$argon2id$"):
		ok, stored, err := verifyArgon2id(pw, encoded)
		if err != nil || !ok {
			return ok, false, err
		}
		return true, stored != s.Argon, nil
	case strings.HasPrefix(encoded, "$2a$"), strings.HasPrefix(encoded, "$2b$"), strings.HasPrefix(encoded, "$2y$"):
		err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(pw))
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, false, nil
		}
		if err != nil {
			return false, false, fmt.Errorf("auth: bcrypt verify: %w", err)
		}
		return true, true, nil
	default:
		return false, false, fmt.Errorf("auth: unrecognized password hash format")
	}
}

// verifyArgon2id also returns the PHC-embedded params so callers can detect
// drift from the currently configured ones.
func verifyArgon2id(pw, encoded string) (bool, ArgonParams, error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, key]
	if len(parts) != 6 {
		return false, ArgonParams{}, fmt.Errorf("auth: malformed argon2id hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ArgonParams{}, fmt.Errorf("auth: malformed argon2id version: %w", err)
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, ArgonParams{}, fmt.Errorf("auth: malformed argon2id params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ArgonParams{}, fmt.Errorf("auth: malformed argon2id salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ArgonParams{}, fmt.Errorf("auth: malformed argon2id key: %w", err)
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	stored := ArgonParams{MemoryKiB: m, Iterations: t, Parallelism: p, SaltLen: uint32(len(salt)), KeyLen: uint32(len(want))}
	return subtle.ConstantTimeCompare(got, want) == 1, stored, nil
}

// VerifyDummy is called when no user matches, so response timing does not
// reveal account existence. The dummy hash is derived from the service's
// *currently configured* params, which only equalizes CPU time while stored
// hashes carry those same params. After a cost raise, existing hashes verify
// at their embedded (cheaper) params until rehash-on-login upgrades them, so
// a wrong password on a legacy account completes faster than this dummy — a
// drift oracle no single dummy can close, because stored hash costs are
// heterogeneous. The scaffolded login flow closes it with a wall-clock pad
// on the invalid path (util.timestamp + util.delay to a fixed deadline);
// custom login flows should copy that pattern.
func (s *Service) VerifyDummy(pw string) {
	s.dummyOnce.Do(func() {
		// best-effort: on the (rand.Read) failure path VerifyDummy degrades
		// to a no-op rather than failing the login flow
		s.dummyHash, _ = s.HashPassword("noda-dummy-password-for-timing")
	})
	if s.dummyHash != "" {
		_, _, _ = s.VerifyPassword(pw, s.dummyHash)
	}
}

// MintToken returns a 256-bit random token (base64url raw) and its SHA-256 hex hash.
func MintToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: generate token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashToken(raw), nil
}

// HashToken returns the SHA-256 hex digest of a raw token.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
