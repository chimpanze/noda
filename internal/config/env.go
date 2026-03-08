package config

import (
	"fmt"
	"os"
	"regexp"
)

var validEnvName = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// DetectEnvironment determines the active environment.
// Priority: flagValue (if non-empty) → NODA_ENV env var → "development".
func DetectEnvironment(flagValue string) (string, error) {
	env := flagValue
	if env == "" {
		env = os.Getenv("NODA_ENV")
	}
	if env == "" {
		env = "development"
	}

	if !validEnvName.MatchString(env) {
		return "", fmt.Errorf("invalid environment name %q: must be alphanumeric with hyphens only", env)
	}

	return env, nil
}
