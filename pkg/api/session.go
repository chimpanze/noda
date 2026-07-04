package api

import "context"

// SessionAuthenticator is implemented by auth services that validate opaque
// session tokens. db is the GORM handle of the service named by
// DatabaseServiceName (typed any to keep pkg/api free of gorm).
// AuthenticateSession returns (nil, nil) when the token is invalid.
type SessionAuthenticator interface {
	AuthenticateSession(ctx context.Context, db any, rawToken string) (*AuthData, error)
	DatabaseServiceName() string
	SessionCookieName() string
}
