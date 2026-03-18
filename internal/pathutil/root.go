package pathutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Root represents a validated, absolute base directory for safe path resolution.
type Root struct {
	abs string // absolute, Clean'd
}

// NewRoot creates a Root from a directory path, resolving it to an absolute path.
func NewRoot(configDir string) (Root, error) {
	abs, err := filepath.Abs(configDir)
	if err != nil {
		return Root{}, fmt.Errorf("cannot resolve config directory %q: %w", configDir, err)
	}
	return Root{abs: filepath.Clean(abs)}, nil
}

// String returns the absolute path of the root directory.
func (r Root) String() string {
	return r.abs
}

// Resolve converts a relative path to a safe absolute path within the root.
// Returns an error if the path is absolute or escapes the root directory.
func (r Root) Resolve(relPath string) (string, error) {
	cleaned := filepath.Clean(relPath)
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("expected relative path, got absolute %q", relPath)
	}
	joined := filepath.Join(r.abs, cleaned)
	if !r.Contains(joined) {
		return "", fmt.Errorf("path %q resolves outside root", relPath)
	}
	return joined, nil
}

// Contains reports whether absPath is within (or equal to) the root directory.
// Uses separator-aware prefix checking to prevent sibling directory attacks.
func (r Root) Contains(absPath string) bool {
	cleaned := filepath.Clean(absPath)
	if cleaned == r.abs {
		return true
	}
	return strings.HasPrefix(cleaned, r.abs+string(filepath.Separator))
}

// Rel returns a relative path from the root to absPath.
// If Rel fails, returns the original path unchanged.
func (r Root) Rel(absPath string) string {
	if absPath == "" {
		return ""
	}
	rel, err := filepath.Rel(r.abs, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

// Join constructs a path under the root from known-safe subpath elements.
func (r Root) Join(elem ...string) string {
	parts := append([]string{r.abs}, elem...)
	return filepath.Join(parts...)
}
