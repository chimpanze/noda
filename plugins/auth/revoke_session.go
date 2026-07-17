package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type revokeSessionDescriptor struct{}

func (d *revokeSessionDescriptor) Name() string { return "revoke_session" }
func (d *revokeSessionDescriptor) Description() string {
	return "Revokes one session (by token or id) or all sessions for a user"
}
func (d *revokeSessionDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"auth":     {Prefix: "auth", Required: true},
		"database": {Prefix: "db", Required: true},
	}
}
func (d *revokeSessionDescriptor) ConfigSchema() map[string]any {
	fields := map[string]any{
		"token":      map[string]any{"type": "string", "description": "Raw session token to revoke (expression); exactly one of token/session_id/user_id"},
		"session_id": map[string]any{"type": "string", "description": "Session id to revoke (expression); exactly one of token/session_id/user_id"},
		"user_id":    map[string]any{"type": "string", "description": "Revoke ALL sessions for this user (expression); exactly one of token/session_id/user_id"},
	}
	return map[string]any{
		"oneOf": []any{
			map[string]any{"type": "object", "properties": fields, "required": []any{"token"}},
			map[string]any{"type": "object", "properties": fields, "required": []any{"session_id"}},
			map[string]any{"type": "object", "properties": fields, "required": []any{"user_id"}},
		},
	}
}
func (d *revokeSessionDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "{revoked_count, clear_cookie} — pass clear_cookie to response.json cookies",
		"error":   "Infrastructure error",
	}
}

type revokeSessionExecutor struct{}

func newRevokeSessionExecutor(_ map[string]any) api.NodeExecutor { return &revokeSessionExecutor{} }

func (e *revokeSessionExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *revokeSessionExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "auth")
	if err != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", err)
	}
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", err)
	}
	token, _, err := plugin.ResolveOptionalString(nCtx, config, "token")
	if err != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", err)
	}
	sessionID, _, err := plugin.ResolveOptionalString(nCtx, config, "session_id")
	if err != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", err)
	}
	userID, _, err := plugin.ResolveOptionalString(nCtx, config, "user_id")
	if err != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", err)
	}

	set := 0
	for _, v := range []string{token, sessionID, userID} {
		if v != "" {
			set++
		}
	}
	if set != 1 {
		return "", nil, fmt.Errorf("auth.revoke_session: exactly one of 'token', 'session_id', 'user_id' is required")
	}

	q := db.WithContext(ctx).Table("auth_sessions").Where("revoked_at IS NULL")
	switch {
	case token != "":
		q = q.Where("token_hash = ?", HashToken(token))
	case sessionID != "":
		q = q.Where("id = ?", sessionID)
	default:
		q = q.Where("user_id = ?", userID)
	}
	res := q.Update("revoked_at", time.Now().UTC())
	if res.Error != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", res.Error)
	}
	return api.OutputSuccess, map[string]any{
		"revoked_count": res.RowsAffected,
		"clear_cookie":  svc.ClearCookieObject(),
	}, nil
}
