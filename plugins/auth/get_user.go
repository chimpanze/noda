package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type getUserDescriptor struct{}

func (d *getUserDescriptor) Name() string { return "get_user" }
func (d *getUserDescriptor) Description() string {
	return "Fetches a user by id or email (password hash stripped)"
}
func (d *getUserDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"auth":     {Prefix: "auth", Required: true},
		"database": {Prefix: "db", Required: true},
	}
}
func (d *getUserDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user_id": map[string]any{"type": "string", "description": "User id (expression); exactly one of user_id/email"},
			"email":   map[string]any{"type": "string", "description": "Email (expression); exactly one of user_id/email"},
		},
	}
}
func (d *getUserDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success":   "User object (no password_hash)",
		"not_found": "No matching user",
		"error":     "Infrastructure error",
	}
}

type getUserExecutor struct{}

func newGetUserExecutor(_ map[string]any) api.NodeExecutor { return &getUserExecutor{} }

func (e *getUserExecutor) Outputs() []string { return []string{"success", "not_found", "error"} }

func (e *getUserExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.get_user: %w", err)
	}
	userID, _, err := plugin.ResolveOptionalString(nCtx, config, "user_id")
	if err != nil {
		return "", nil, fmt.Errorf("auth.get_user: %w", err)
	}
	email, _, err := plugin.ResolveOptionalString(nCtx, config, "email")
	if err != nil {
		return "", nil, fmt.Errorf("auth.get_user: %w", err)
	}
	if (userID == "") == (email == "") {
		return "", nil, fmt.Errorf("auth.get_user: exactly one of 'user_id' or 'email' is required")
	}

	q := db.WithContext(ctx).Table("auth_users")
	if userID != "" {
		q = q.Where("id = ?", userID)
	} else {
		q = q.Where("email = ?", normalizeEmail(email))
	}
	row := map[string]any{}
	if err := q.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "not_found", map[string]any{}, nil
		}
		return "", nil, fmt.Errorf("auth.get_user: %w", err)
	}
	return api.OutputSuccess, userView(row), nil
}
