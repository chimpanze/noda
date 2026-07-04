package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

// lastUsedThrottle limits last_used_at writes to once per interval per session.
const lastUsedThrottle = time.Minute

// DatabaseServiceName implements api.SessionAuthenticator.
func (s *Service) DatabaseServiceName() string { return s.DatabaseName }

// SessionCookieName implements api.SessionAuthenticator.
func (s *Service) SessionCookieName() string { return s.Cookie.Name }

// AuthenticateSession implements api.SessionAuthenticator.
func (s *Service) AuthenticateSession(ctx context.Context, dbAny any, rawToken string) (*api.AuthData, error) {
	db, ok := dbAny.(*gorm.DB)
	if !ok {
		return nil, fmt.Errorf("auth: AuthenticateSession: expected *gorm.DB, got %T", dbAny)
	}
	if rawToken == "" {
		return nil, nil
	}
	hash := HashToken(rawToken)
	now := time.Now().UTC()

	var row struct {
		SessionID       string
		UserID          string
		Email           string
		EmailVerifiedAt *time.Time
		Roles           string
		LastUsedAt      *time.Time
	}
	err := db.WithContext(ctx).Table("auth_sessions").
		Select("auth_sessions.id AS session_id, auth_sessions.last_used_at AS last_used_at, "+
			"auth_users.id AS user_id, auth_users.email AS email, "+
			"auth_users.email_verified_at AS email_verified_at, auth_users.roles AS roles").
		Joins("JOIN auth_users ON auth_users.id = auth_sessions.user_id").
		Where("auth_sessions.token_hash = ? AND auth_sessions.revoked_at IS NULL AND auth_sessions.expires_at > ? AND auth_users.status = ?",
			hash, now, "active").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: AuthenticateSession: %w", err)
	}

	if row.LastUsedAt == nil || now.Sub(*row.LastUsedAt) > lastUsedThrottle {
		// best-effort; a failed touch must not fail authentication
		db.WithContext(ctx).Table("auth_sessions").
			Where("id = ?", row.SessionID).Update("last_used_at", now)
	}

	roles := parseRoles(row.Roles)
	return &api.AuthData{
		UserID: row.UserID,
		Roles:  roles,
		Claims: map[string]any{
			"sub":            row.UserID,
			"email":          row.Email,
			"email_verified": row.EmailVerifiedAt != nil,
			"session_id":     row.SessionID,
			"roles":          roles,
		},
	}, nil
}
