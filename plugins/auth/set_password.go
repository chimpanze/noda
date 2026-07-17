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
	return "Sets a new password (argon2id), optionally consuming a reset token atomically, and revokes the user's sessions"
}
func (d *setPasswordDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"auth":     {Prefix: "auth", Required: true},
		"database": {Prefix: "db", Required: true},
	}
}
func (d *setPasswordDescriptor) ConfigSchema() map[string]any {
	fields := map[string]any{
		"user_id":         map[string]any{"type": "string", "description": "User id (expression); exactly one of user_id/token"},
		"token":           map[string]any{"type": "string", "description": "Password-reset token to consume atomically in the same transaction (expression); exactly one of user_id/token"},
		"password":        map[string]any{"type": "string", "description": "New plaintext password (expression)"},
		"revoke_sessions": map[string]any{"type": "boolean", "description": "Revoke all existing sessions (default true)"},
	}
	return map[string]any{
		"oneOf": []any{
			map[string]any{"type": "object", "title": "By user_id", "properties": fields, "required": []any{"user_id", "password"}},
			map[string]any{"type": "object", "title": "By reset token", "properties": fields, "required": []any{"token", "password"}},
		},
	}
}
func (d *setPasswordDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "{revoked_sessions} count",
		"invalid": "Reset token unknown, expired, or already used (token mode only)",
		"error":   "Infrastructure error, unknown user, or invalid new password",
	}
}

type setPasswordExecutor struct{}

func newSetPasswordExecutor(_ map[string]any) api.NodeExecutor { return &setPasswordExecutor{} }

func (e *setPasswordExecutor) Outputs() []string { return []string{"success", "invalid", "error"} }

func (e *setPasswordExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "auth")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	userID, hasUserID, err := plugin.ResolveOptionalString(nCtx, config, "user_id")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	token, hasToken, err := plugin.ResolveOptionalString(nCtx, config, "token")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	if hasUserID == hasToken {
		return "", nil, fmt.Errorf("auth.set_password: exactly one of user_id or token must be set")
	}
	password, err := plugin.ResolveString(nCtx, config, "password")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	// Validate before any DB write: in token mode a rejected password must
	// not consume the token (auth-3).
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

	var revoked int64
	invalid := false
	// Token consumption, the password update, and session revocation commit
	// or fail together: any failure rolls back the consume, so the token
	// stays usable for a retry.
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		uid := userID
		if hasToken {
			var inv bool
			var err error
			uid, inv, err = consumeTokenInTx(tx, HashToken(token), PurposeResetPassword, now)
			if err != nil {
				return err
			}
			if inv {
				invalid = true
				return nil
			}
		}
		res := tx.Table("auth_users").Where("id = ?", uid).
			Updates(map[string]any{"password_hash": hash, "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("user not found")
		}
		if revoke {
			r := tx.Table("auth_sessions").
				Where("user_id = ? AND revoked_at IS NULL", uid).
				Update("revoked_at", now)
			if r.Error != nil {
				return fmt.Errorf("revoke sessions: %w", r.Error)
			}
			revoked = r.RowsAffected
		}
		return nil
	})
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	if invalid {
		return "invalid", map[string]any{}, nil
	}
	return api.OutputSuccess, map[string]any{"revoked_sessions": revoked}, nil
}
