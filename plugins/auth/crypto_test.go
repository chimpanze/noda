package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashAndVerifyPassword(t *testing.T) {
	s := &Service{Argon: DefaultArgonParams()}
	hash, err := s.HashPassword("hunter2!")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("expected PHC argon2id hash, got %q", hash)
	}
	ok, rehash, err := VerifyPassword("hunter2!", hash)
	if err != nil || !ok || rehash {
		t.Fatalf("want ok=true rehash=false err=nil, got ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	ok, _, err = VerifyPassword("wrong", hash)
	if err != nil || ok {
		t.Fatalf("wrong password must not verify")
	}
}

func TestVerifyPasswordBcryptCompat(t *testing.T) {
	// bcrypt hash of "hunter2!" (cost 10) — generated with golang.org/x/crypto/bcrypt
	ok, rehash, err := VerifyPassword("hunter2!", mustBcrypt(t, "hunter2!"))
	if err != nil || !ok {
		t.Fatalf("bcrypt hash must verify: ok=%v err=%v", ok, err)
	}
	if !rehash {
		t.Fatal("bcrypt hashes must be flagged for rehash")
	}
}

func TestVerifyPasswordMalformedHash(t *testing.T) {
	if _, _, err := VerifyPassword("x", "not-a-hash"); err == nil {
		t.Fatal("malformed hash must error")
	}
}

func TestMintToken(t *testing.T) {
	raw, hash, err := MintToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 40 { // 32 bytes base64url ≈ 43 chars
		t.Fatalf("token too short: %d", len(raw))
	}
	if HashToken(raw) != hash {
		t.Fatal("HashToken(raw) must equal minted hash")
	}
	raw2, _, _ := MintToken()
	if raw == raw2 {
		t.Fatal("tokens must be unique")
	}
}

func mustBcrypt(t *testing.T, pw string) string {
	t.Helper()
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
