package util

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/golang-jwt/jwt/v5"
)

type jwtSignDescriptor struct{}

func (d *jwtSignDescriptor) Name() string                           { return "jwt_sign" }
func (d *jwtSignDescriptor) Description() string                    { return "Signs a JWT token with the given claims" }
func (d *jwtSignDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *jwtSignDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"claims": map[string]any{
				"type":        "object",
				"description": "Claims to include in the token (expression values are resolved)",
			},
			"secret": map[string]any{
				"type":        "string",
				"description": "Signing secret (expression)",
			},
			"algorithm": map[string]any{
				"type":        "string",
				"enum":        []any{"HS256", "HS384", "HS512"},
				"default":     "HS256",
				"description": "Signing algorithm: HS256, HS384, or HS512",
			},
			"expiry": map[string]any{
				"type":        "string",
				"description": "Token expiry duration (e.g. \"1h\", \"24h\", \"7d\"). Sets the exp claim.",
			},
		},
		"required": []any{"claims", "secret"},
	}
}
func (d *jwtSignDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Object with token string field",
		"error":   "Signing error",
	}
}

type jwtSignExecutor struct{}

func newJWTSignExecutor(config map[string]any) api.NodeExecutor {
	return &jwtSignExecutor{}
}

func (e *jwtSignExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *jwtSignExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	// Resolve secret
	secretExpr, _ := config["secret"].(string)
	secretVal, err := nCtx.Resolve(secretExpr)
	if err != nil {
		return "", nil, fmt.Errorf("util.jwt_sign: secret: %w", err)
	}
	secret, ok := secretVal.(string)
	if !ok {
		return "", nil, fmt.Errorf("util.jwt_sign: secret must resolve to a string, got %T", secretVal)
	}

	// Resolve claims
	claimsCfg, _ := config["claims"].(map[string]any)
	if claimsCfg == nil {
		return "", nil, fmt.Errorf("util.jwt_sign: claims is required")
	}
	mapClaims := jwt.MapClaims{}
	for k, v := range claimsCfg {
		if exprStr, ok := v.(string); ok {
			resolved, err := nCtx.Resolve(exprStr)
			if err != nil {
				return "", nil, fmt.Errorf("util.jwt_sign: claim %q: %w", k, err)
			}
			mapClaims[k] = resolved
		} else {
			mapClaims[k] = v
		}
	}

	// Resolve and parse expiry
	if expiryExpr, ok := config["expiry"].(string); ok && expiryExpr != "" {
		expiryVal, err := nCtx.Resolve(expiryExpr)
		if err != nil {
			return "", nil, fmt.Errorf("util.jwt_sign: expiry: %w", err)
		}
		expiryStr, ok := expiryVal.(string)
		if !ok {
			return "", nil, fmt.Errorf("util.jwt_sign: expiry must resolve to a string, got %T", expiryVal)
		}
		dur, err := parseDuration(expiryStr)
		if err != nil {
			return "", nil, fmt.Errorf("util.jwt_sign: expiry: %w", err)
		}
		mapClaims["exp"] = time.Now().Add(dur).Unix()
	}

	// Determine signing method
	algorithm, _ := config["algorithm"].(string)
	if algorithm == "" {
		algorithm = "HS256"
	}
	var signingMethod jwt.SigningMethod
	switch algorithm {
	case "HS256":
		signingMethod = jwt.SigningMethodHS256
	case "HS384":
		signingMethod = jwt.SigningMethodHS384
	case "HS512":
		signingMethod = jwt.SigningMethodHS512
	default:
		return "", nil, fmt.Errorf("util.jwt_sign: unsupported algorithm %q", algorithm)
	}

	token := jwt.NewWithClaims(signingMethod, mapClaims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", nil, fmt.Errorf("util.jwt_sign: %w", err)
	}

	return api.OutputSuccess, signed, nil
}

// parseDuration parses duration strings like "1h", "24h", "7d", "30m".
func parseDuration(s string) (time.Duration, error) {
	// Handle "d" suffix for days
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}
