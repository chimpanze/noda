package api

// Fiber locals keys for JWT/OIDC authentication data.
// These keys are set by the auth.jwt and auth.oidc middleware and read by
// trigger mapping, Casbin, and connection managers (WebSocket/SSE).
const (
	LocalJWTClaims = "jwt_claims"
	LocalJWTUserID = "jwt_user_id"
	LocalJWTRoles  = "jwt_roles"
)
