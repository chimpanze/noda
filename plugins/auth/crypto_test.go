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
	ok, rehash, err := s.VerifyPassword("hunter2!", hash)
	if err != nil || !ok || rehash {
		t.Fatalf("want ok=true rehash=false err=nil, got ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	ok, _, err = s.VerifyPassword("wrong", hash)
	if err != nil || ok {
		t.Fatalf("wrong password must not verify")
	}
}

func TestVerifyPasswordBcryptCompat(t *testing.T) {
	// bcrypt hash of "hunter2!" (cost 10) — generated with golang.org/x/crypto/bcrypt
	s := &Service{Argon: DefaultArgonParams()}
	ok, rehash, err := s.VerifyPassword("hunter2!", mustBcrypt(t, "hunter2!"))
	if err != nil || !ok {
		t.Fatalf("bcrypt hash must verify: ok=%v err=%v", ok, err)
	}
	if !rehash {
		t.Fatal("bcrypt hashes must be flagged for rehash")
	}
}

func TestVerifyPasswordMalformedHash(t *testing.T) {
	s := &Service{Argon: DefaultArgonParams()}
	if _, _, err := s.VerifyPassword("x", "not-a-hash"); err == nil {
		t.Fatal("malformed hash must error")
	}
}

func TestVerifyPasswordParamDriftRehash(t *testing.T) {
	weak := &Service{Argon: ArgonParams{MemoryKiB: 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}}
	hash, err := weak.HashPassword("hunter2!")
	if err != nil {
		t.Fatal(err)
	}

	// same params → no rehash
	ok, rehash, err := weak.VerifyPassword("hunter2!", hash)
	if err != nil || !ok || rehash {
		t.Fatalf("same params: want ok=true rehash=false, got ok=%v rehash=%v err=%v", ok, rehash, err)
	}

	// raised params → opportunistic rehash
	strong := &Service{Argon: ArgonParams{MemoryKiB: 2048, Iterations: 2, Parallelism: 1, SaltLen: 16, KeyLen: 32}}
	ok, rehash, err = strong.VerifyPassword("hunter2!", hash)
	if err != nil || !ok || !rehash {
		t.Fatalf("param drift: want ok=true rehash=true, got ok=%v rehash=%v err=%v", ok, rehash, err)
	}

	// wrong password → never flagged for rehash
	ok, rehash, err = strong.VerifyPassword("wrong", hash)
	if err != nil || ok || rehash {
		t.Fatalf("wrong password: want ok=false rehash=false, got ok=%v rehash=%v err=%v", ok, rehash, err)
	}
}

func TestVerifyDummyUsesServiceParams(t *testing.T) {
	s := &Service{Argon: ArgonParams{MemoryKiB: 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}}
	s.VerifyDummy("whatever")
	if !strings.Contains(s.dummyHash, "m=1024,t=1,p=1") {
		t.Fatalf("dummy hash must use the service's configured params, got %q", s.dummyHash)
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
