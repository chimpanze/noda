package api

// Fiber locals keys for JWT authentication data.
// These keys are set by the JWT middleware and read by trigger mapping, Casbin,
// and connection managers (WebSocket/SSE).
const (
	LocalJWTClaims = "jwt_claims"
	LocalJWTUserID = "jwt_user_id"
	LocalJWTRoles  = "jwt_roles"
)
