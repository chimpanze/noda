// Package scaffold holds helpers shared by the CLI and MCP project scaffolds.
package scaffold

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// GenerateJWTSecret returns a fresh 32-byte hex secret (64 chars) —
// noda's auth.jwt middleware requires >=32 bytes (#381).
func GenerateJWTSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating jwt secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ApplyJWTSecret rewrites the JWT_SECRET= line in envContent to use secret,
// leaving every other line (including comments) untouched. If no JWT_SECRET=
// line is found, envContent is returned unmodified.
func ApplyJWTSecret(envContent, secret string) string {
	lines := strings.Split(envContent, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "JWT_SECRET=") {
			lines[i] = "JWT_SECRET=" + secret
		}
	}
	return strings.Join(lines, "\n")
}
