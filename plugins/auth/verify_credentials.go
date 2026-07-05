package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type verifyCredentialsDescriptor struct{}

func (d *verifyCredentialsDescriptor) Name() string { return "verify_credentials" }
func (d *verifyCredentialsDescriptor) Description() string {
	return "Verifies email+password with timing-safe comparison"
}
func (d *verifyCredentialsDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"auth":     {Prefix: "auth", Required: true},
		"database": {Prefix: "db", Required: true},
	}
}
func (d *verifyCredentialsDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"email":    map[string]any{"type": "string", "description": "Email (expression)"},
			"password": map[string]any{"type": "string", "description": "Plaintext password (expression)"},
		},
		"required": []any{"email", "password"},
	}
}
func (d *verifyCredentialsDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Authenticated user (no password_hash)",
		"invalid": "Credentials rejected (no reason disclosed)",
		"error":   "Infrastructure error",
	}
}

type verifyCredentialsExecutor struct{}

func newVerifyCredentialsExecutor(_ map[string]any) api.NodeExecutor {
	return &verifyCredentialsExecutor{}
}

func (e *verifyCredentialsExecutor) Outputs() []string {
	return []string{"success", "invalid", "error"}
}

func (e *verifyCredentialsExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "auth")
	if err != nil {
		return "", nil, fmt.Errorf("auth.verify_credentials: %w", err)
	}
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.verify_credentials: %w", err)
	}
	email, err := plugin.ResolveString(nCtx, config, "email")
	if err != nil {
		return "", nil, fmt.Errorf("auth.verify_credentials: %w", err)
	}
	password, err := plugin.ResolveString(nCtx, config, "password")
	if err != nil {
		return "", nil, fmt.Errorf("auth.verify_credentials: %w", err)
	}

	row := map[string]any{}
	err = db.WithContext(ctx).Table("auth_users").Where("email = ?", normalizeEmail(email)).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		VerifyDummy(password) // burn the same time as a real verification
		nCtx.Log("debug", "auth.verify_credentials: unknown email", nil)
		return "invalid", map[string]any{}, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("auth.verify_credentials: %w", err)
	}

	storedHash, _ := row["password_hash"].(string)
	ok, needsRehash, err := VerifyPassword(password, storedHash)
	if err != nil {
		// VerifyPassword is pure CPU: any error means a corrupted or
		// unrecognized stored hash (e.g. a bad import), never infrastructure.
		// Surfacing it would 500 exactly those accounts — an enumerable
		// signal and a lockout. Treat as invalid, keep timing flat.
		VerifyDummy(password)
		nCtx.Log("debug", "auth.verify_credentials: unusable password_hash", map[string]any{"error": err.Error()})
		return "invalid", map[string]any{}, nil
	}
	if !ok {
		nCtx.Log("debug", "auth.verify_credentials: wrong password", nil)
		return "invalid", map[string]any{}, nil
	}
	if status, _ := row["status"].(string); status != "active" {
		nCtx.Log("debug", "auth.verify_credentials: user not active", nil)
		return "invalid", map[string]any{}, nil
	}

	if needsRehash {
		if newHash, hashErr := svc.HashPassword(password); hashErr == nil {
			// best-effort upgrade; a failure must not fail the login
			db.WithContext(ctx).Table("auth_users").Where("id = ?", row["id"]).
				Updates(map[string]any{"password_hash": newHash, "updated_at": time.Now().UTC()})
		}
	}
	return api.OutputSuccess, userView(row), nil
}
