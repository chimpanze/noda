package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type setPasswordDescriptor struct{}

func (d *setPasswordDescriptor) Name() string { return "set_password" }
func (d *setPasswordDescriptor) Description() string {
	return "Sets a new password (argon2id) and revokes the user's sessions"
}
func (d *setPasswordDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"auth":     {Prefix: "auth", Required: true},
		"database": {Prefix: "db", Required: true},
	}
}
func (d *setPasswordDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user_id":         map[string]any{"type": "string", "description": "User id (expression)"},
			"password":        map[string]any{"type": "string", "description": "New plaintext password (expression)"},
			"revoke_sessions": map[string]any{"type": "boolean", "description": "Revoke all existing sessions (default true)"},
		},
		"required": []any{"user_id", "password"},
	}
}
func (d *setPasswordDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "{revoked_sessions} count",
		"error":   "Infrastructure error or unknown user",
	}
}

type setPasswordExecutor struct{}

func newSetPasswordExecutor(_ map[string]any) api.NodeExecutor { return &setPasswordExecutor{} }

func (e *setPasswordExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *setPasswordExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "auth")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	userID, err := plugin.ResolveString(nCtx, config, "user_id")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	password, err := plugin.ResolveString(nCtx, config, "password")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	if err := validatePassword(password); err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	revoke := true
	if v, ok := config["revoke_sessions"].(bool); ok {
		revoke = v
	}

	hash, err := svc.HashPassword(password)
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	now := time.Now().UTC()
	res := db.WithContext(ctx).Table("auth_users").Where("id = ?", userID).
		Updates(map[string]any{"password_hash": hash, "updated_at": now})
	if res.Error != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return "", nil, fmt.Errorf("auth.set_password: user not found")
	}

	var revoked int64
	if revoke {
		r := db.WithContext(ctx).Table("auth_sessions").
			Where("user_id = ? AND revoked_at IS NULL", userID).
			Update("revoked_at", now)
		if r.Error != nil {
			return "", nil, fmt.Errorf("auth.set_password: revoke sessions: %w", r.Error)
		}
		revoked = r.RowsAffected
	}
	return api.OutputSuccess, map[string]any{"revoked_sessions": revoked}, nil
}
