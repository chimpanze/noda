package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gofiber/fiber/v3"
)

// newOIDCMiddleware creates an OIDC ID token validation middleware.
// It validates tokens against an external identity provider using OIDC discovery
// and stores claims in the same Fiber locals as the JWT middleware.
func newOIDCMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("auth.oidc: security.oidc config is required")
	}

	issuerURL, _ := cfg["issuer_url"].(string)
	if issuerURL == "" {
		return nil, fmt.Errorf("auth.oidc: issuer_url is required")
	}

	clientID, _ := cfg["client_id"].(string)
	if clientID == "" {
		return nil, fmt.Errorf("auth.oidc: client_id is required")
	}

	userIDClaim, _ := cfg["user_id_claim"].(string)
	if userIDClaim == "" {
		userIDClaim = "sub"
	}

	rolesClaim, _ := cfg["roles_claim"].(string)
	if rolesClaim == "" {
		rolesClaim = "roles"
	}

	var requiredScopes []string
	if scopes, ok := cfg["required_scopes"].([]any); ok {
		for _, s := range scopes {
			if str, ok := s.(string); ok {
				requiredScopes = append(requiredScopes, str)
			}
		}
	}

	// Perform OIDC discovery (fetches .well-known/openid-configuration, caches JWKS)
	provider, err := oidc.NewProvider(context.Background(), issuerURL)
	if err != nil {
		return nil, fmt.Errorf("auth.oidc: OIDC discovery failed for %q: %w", issuerURL, err)
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: clientID,
	})

	return func(c fiber.Ctx) error {
		auth := c.Get("Authorization")
		if auth == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing authorization header")
		}

		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		if tokenStr == auth {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid authorization format")
		}

		idToken, err := verifier.Verify(c.Context(), tokenStr)
		if err != nil {
			slog.Debug("oidc token validation failed", "error", err)
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token")
		}

		// Extract claims into map
		var claimsMap map[string]any
		if err := idToken.Claims(&claimsMap); err != nil {
			slog.Debug("oidc claims extraction failed", "error", err)
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token claims")
		}

		// Verify required scopes if configured
		if len(requiredScopes) > 0 {
			tokenScopes := extractScopes(claimsMap)
			for _, required := range requiredScopes {
				if !containsString(tokenScopes, required) {
					return fiber.NewError(fiber.StatusForbidden, fmt.Sprintf("missing required scope: %s", required))
				}
			}
		}

		// Store claims in same Fiber locals as JWT middleware
		c.Locals(api.LocalJWTClaims, claimsMap)

		if userID, ok := claimsMap[userIDClaim].(string); ok {
			c.Locals(api.LocalJWTUserID, userID)
		}

		if roles, ok := claimsMap[rolesClaim].([]any); ok {
			roleStrs := make([]string, 0, len(roles))
			for _, r := range roles {
				if s, ok := r.(string); ok {
					roleStrs = append(roleStrs, s)
				}
			}
			c.Locals(api.LocalJWTRoles, roleStrs)
		}

		return c.Next()
	}, nil
}

// extractScopes extracts scope strings from claims.
// Supports both space-delimited "scope" string and "scopes" array.
func extractScopes(claims map[string]any) []string {
	// Try "scope" as space-delimited string (standard OIDC)
	if scope, ok := claims["scope"].(string); ok {
		return strings.Fields(scope)
	}
	// Try "scopes" as array
	if scopes, ok := claims["scopes"].([]any); ok {
		result := make([]string, 0, len(scopes))
		for _, s := range scopes {
			if str, ok := s.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return nil
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
