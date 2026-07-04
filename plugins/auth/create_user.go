package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type createUserDescriptor struct{}

func (d *createUserDescriptor) Name() string { return "create_user" }
func (d *createUserDescriptor) Description() string {
	return "Creates a user with an argon2id-hashed password"
}
func (d *createUserDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"auth":     {Prefix: "auth", Required: true},
		"database": {Prefix: "db", Required: true},
	}
}
func (d *createUserDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"email":    map[string]any{"type": "string", "description": "Email address (expression)"},
			"password": map[string]any{"type": "string", "description": "Plaintext password (expression); never stored"},
			"roles":    map[string]any{"type": "array", "description": "Role names; defaults to [\"user\"]"},
			"metadata": map[string]any{"type": "object", "description": "Arbitrary user metadata"},
		},
		"required": []any{"email", "password"},
	}
}
func (d *createUserDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Created user (id, email, status, roles, metadata, timestamps; no password_hash)",
		"exists":  "A user with this email already exists",
		"error":   "Infrastructure error",
	}
}

type createUserExecutor struct{}

func newCreateUserExecutor(_ map[string]any) api.NodeExecutor { return &createUserExecutor{} }

func (e *createUserExecutor) Outputs() []string { return []string{"success", "exists", "error"} }

func (e *createUserExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "auth")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_user: %w", err)
	}
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_user: %w", err)
	}
	email, err := plugin.ResolveString(nCtx, config, "email")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_user: %w", err)
	}
	password, err := plugin.ResolveString(nCtx, config, "password")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_user: %w", err)
	}
	email = normalizeEmail(email)
	if email == "" {
		return "", nil, fmt.Errorf("auth.create_user: email is empty")
	}
	if err := validatePassword(password); err != nil {
		return "", nil, fmt.Errorf("auth.create_user: %w", err)
	}

	roles := []string{"user"}
	if arr, err := plugin.ResolveOptionalArray(nCtx, config, "roles"); err != nil {
		return "", nil, fmt.Errorf("auth.create_user: %w", err)
	} else if arr != nil {
		roles = roles[:0]
		for _, r := range arr {
			if s, ok := r.(string); ok {
				roles = append(roles, s)
			}
		}
	}
	metadata, err := plugin.ResolveOptionalMap(nCtx, config, "metadata")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_user: %w", err)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}

	hash, err := svc.HashPassword(password)
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_user: %w", err)
	}
	rolesJSON, _ := json.Marshal(roles)
	metaJSON, _ := json.Marshal(metadata)

	now := time.Now().UTC()
	row := map[string]any{
		"id":            uuid.NewString(),
		"email":         email,
		"password_hash": hash,
		"status":        "active",
		"roles":         string(rolesJSON),
		"metadata":      string(metaJSON),
		"created_at":    now,
		"updated_at":    now,
	}
	if err := db.WithContext(ctx).Table("auth_users").Create(row).Error; err != nil {
		if isUniqueViolation(err) {
			return "exists", map[string]any{}, nil
		}
		return "", nil, fmt.Errorf("auth.create_user: %w", err)
	}
	return api.OutputSuccess, userView(row), nil
}
