package scaffold

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateJWTSecret(t *testing.T) {
	a, err := GenerateJWTSecret()
	require.NoError(t, err)
	b, err := GenerateJWTSecret()
	require.NoError(t, err)
	assert.Len(t, a, 64)
	assert.NotEqual(t, a, b, "secrets must be unique per call")
	_, err = hex.DecodeString(a)
	assert.NoError(t, err)
}

func TestApplyJWTSecret_LineAtStart(t *testing.T) {
	env := "JWT_SECRET=replace-with-at-least-32-bytes\nOTHER=1\n"
	got := ApplyJWTSecret(env, "deadbeef")
	assert.Equal(t, "JWT_SECRET=deadbeef\nOTHER=1\n", got)
}

func TestApplyJWTSecret_LineInMiddle(t *testing.T) {
	env := "# Database\nDATABASE_URL=postgres://x\n\n# JWT\nJWT_SECRET=replace-with-at-least-32-bytes\n"
	got := ApplyJWTSecret(env, "cafebabe")
	assert.Equal(t, "# Database\nDATABASE_URL=postgres://x\n\n# JWT\nJWT_SECRET=cafebabe\n", got)
}

func TestApplyJWTSecret_PreservesOtherLines(t *testing.T) {
	env := "# Database\nDATABASE_URL=postgres://x\n\n# Redis\nREDIS_URL=redis://y\n\n# JWT\n# auth.jwt requires a secret of at least 32 bytes; a generated one is written to .env\nJWT_SECRET=replace-with-at-least-32-bytes\n"
	got := ApplyJWTSecret(env, "0123456789abcdef0123456789abcdef")
	assert.Contains(t, got, "DATABASE_URL=postgres://x")
	assert.Contains(t, got, "REDIS_URL=redis://y")
	assert.Contains(t, got, "# auth.jwt requires a secret of at least 32 bytes; a generated one is written to .env")
	assert.Contains(t, got, "JWT_SECRET=0123456789abcdef0123456789abcdef")
	assert.NotContains(t, got, "replace-with-at-least-32-bytes")
}
