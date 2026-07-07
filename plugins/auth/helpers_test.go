package auth

import (
	"strings"
	"testing"
)

// validatePassword must count runes (code points), not bytes, so it agrees
// exactly with the scaffolded route schemas' JSON-Schema minLength/maxLength
// (which count code points). Divergence burns real requests: a password that
// passes the route 400-check but fails here surfaces as a 500 — and, before
// set_password consumed tokens atomically, burned the reset token (auth-3).
func TestValidatePasswordCountsRunes(t *testing.T) {
	cases := []struct {
		name    string
		pw      string
		wantErr error
	}{
		{"7 ascii too short", "abcdefg", errPasswordTooShort},
		{"8 ascii ok", "abcdefgh", nil},
		{"4 emoji (16 bytes) too short by runes", "😀😀😀😀", errPasswordTooShort},
		{"8 emoji (32 bytes) ok", "😀😀😀😀😀😀😀😀", nil},
		{"512 two-byte runes (1024 bytes) ok", strings.Repeat("é", 512), nil},
		{"513 runes too long", strings.Repeat("a", 513), errPasswordTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePassword(tc.pw)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
			if tc.wantErr != nil && err != tc.wantErr {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}
