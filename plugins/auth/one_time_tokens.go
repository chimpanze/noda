package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func validPurpose(p string) bool {
	return p == PurposeVerifyEmail || p == PurposeResetPassword
}

type createTokenDescriptor struct{}

func (d *createTokenDescriptor) Name() string { return "create_token" }
func (d *createTokenDescriptor) Description() string {
	return "Mints a single-use token (email verification, password reset)"
}
func (d *createTokenDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"auth":     {Prefix: "auth", Required: true},
		"database": {Prefix: "db", Required: true},
	}
}
func (d *createTokenDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user_id": map[string]any{"type": "string", "description": "User id (expression)"},
			"purpose": map[string]any{"type": "string", "enum": []any{PurposeVerifyEmail, PurposeResetPassword}, "description": "Token purpose"},
			"ttl":     map[string]any{"type": "string", "description": "Lifetime (e.g. \"1h\"); defaults per purpose from service config"},
		},
		"required": []any{"user_id", "purpose"},
	}
}
func (d *createTokenDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "{token, expires_at} — raw token exists only in workflow state; send it via email.send",
		"error":   "Infrastructure error",
	}
}

type createTokenExecutor struct{}

func newCreateTokenExecutor(_ map[string]any) api.NodeExecutor { return &createTokenExecutor{} }

func (e *createTokenExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *createTokenExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "auth")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_token: %w", err)
	}
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_token: %w", err)
	}
	userID, err := plugin.ResolveString(nCtx, config, "user_id")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_token: %w", err)
	}
	purpose, err := plugin.ResolveString(nCtx, config, "purpose")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_token: %w", err)
	}
	if !validPurpose(purpose) {
		return "", nil, fmt.Errorf("auth.create_token: invalid purpose %q", purpose)
	}
	ttl := svc.TokenTTL(purpose)
	if v, ok, err := plugin.ResolveOptionalString(nCtx, config, "ttl"); err != nil {
		return "", nil, fmt.Errorf("auth.create_token: %w", err)
	} else if ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return "", nil, fmt.Errorf("auth.create_token: ttl: %w", err)
		}
		ttl = d
	}

	now := time.Now().UTC()
	// Invalidate prior unconsumed tokens for the same user+purpose.
	if err := db.WithContext(ctx).Table("auth_tokens").
		Where("user_id = ? AND purpose = ? AND consumed_at IS NULL", userID, purpose).
		Update("consumed_at", now).Error; err != nil {
		return "", nil, fmt.Errorf("auth.create_token: %w", err)
	}

	raw, hash, err := MintToken()
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_token: %w", err)
	}
	expiresAt := now.Add(ttl)
	if err := db.WithContext(ctx).Table("auth_tokens").Create(map[string]any{
		"id": uuid.NewString(), "user_id": userID, "purpose": purpose,
		"token_hash": hash, "expires_at": expiresAt, "created_at": now,
	}).Error; err != nil {
		return "", nil, fmt.Errorf("auth.create_token: %w", err)
	}
	return api.OutputSuccess, map[string]any{"token": raw, "expires_at": expiresAt}, nil
}

// consumeTokenInTx atomically claims the (hash, purpose) token inside tx and
// returns its owner. The WHERE guard on consumed_at makes concurrent
// consumption impossible — exactly one UPDATE can match. invalid reports
// unknown/expired/wrong-purpose/already-consumed without distinguishing them.
func consumeTokenInTx(tx *gorm.DB, hash, purpose string, now time.Time) (userID string, invalid bool, err error) {
	res := tx.Table("auth_tokens").
		Where("token_hash = ? AND purpose = ? AND consumed_at IS NULL AND expires_at > ?", hash, purpose, now).
		Update("consumed_at", now)
	if res.Error != nil {
		return "", false, res.Error
	}
	if res.RowsAffected == 0 {
		return "", true, nil
	}
	if err := tx.Table("auth_tokens").
		Where("token_hash = ?", hash).Pluck("user_id", &userID).Error; err != nil {
		return "", false, err
	}
	if userID == "" {
		return "", false, fmt.Errorf("consumed token row disappeared")
	}
	return userID, false, nil
}

type consumeTokenDescriptor struct{}

func (d *consumeTokenDescriptor) Name() string { return "consume_token" }
func (d *consumeTokenDescriptor) Description() string {
	return "Atomically consumes a single-use token"
}
func (d *consumeTokenDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"auth":     {Prefix: "auth", Required: true},
		"database": {Prefix: "db", Required: true},
	}
}
func (d *consumeTokenDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"token":   map[string]any{"type": "string", "description": "Raw token (expression)"},
			"purpose": map[string]any{"type": "string", "enum": []any{PurposeVerifyEmail, PurposeResetPassword}, "description": "Expected purpose"},
		},
		"required": []any{"token", "purpose"},
	}
}
func (d *consumeTokenDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "{user_id} of the token's owner",
		"invalid": "Token unknown, expired, wrong purpose, or already used (undifferentiated)",
		"error":   "Infrastructure error",
	}
}

type consumeTokenExecutor struct{}

func newConsumeTokenExecutor(_ map[string]any) api.NodeExecutor { return &consumeTokenExecutor{} }

func (e *consumeTokenExecutor) Outputs() []string { return []string{"success", "invalid", "error"} }

func (e *consumeTokenExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.consume_token: %w", err)
	}
	token, err := plugin.ResolveString(nCtx, config, "token")
	if err != nil {
		return "", nil, fmt.Errorf("auth.consume_token: %w", err)
	}
	purpose, err := plugin.ResolveString(nCtx, config, "purpose")
	if err != nil {
		return "", nil, fmt.Errorf("auth.consume_token: %w", err)
	}
	if !validPurpose(purpose) {
		return "", nil, fmt.Errorf("auth.consume_token: invalid purpose %q", purpose)
	}

	now := time.Now().UTC()
	hash := HashToken(token)

	var userID string
	invalid := false
	// The consume-UPDATE, the user_id lookup, and (for verify_email) the
	// email_verified_at UPDATE must commit or fail together: a crash between
	// them would otherwise permanently burn the token without ever marking
	// the user verified. Wrapping the whole path in one transaction makes it
	// atomic — if the verify step fails, the consume-UPDATE rolls back too,
	// so the token remains usable.
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		uid, inv, err := consumeTokenInTx(tx, hash, purpose, now)
		if err != nil {
			return err
		}
		if inv {
			invalid = true
			return nil
		}
		userID = uid

		if purpose == PurposeVerifyEmail {
			if err := tx.Table("auth_users").Where("id = ?", userID).
				Updates(map[string]any{"email_verified_at": now, "updated_at": now}).Error; err != nil {
				return fmt.Errorf("mark verified: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return "", nil, fmt.Errorf("auth.consume_token: %w", err)
	}
	if invalid {
		return "invalid", map[string]any{}, nil
	}
	return api.OutputSuccess, map[string]any{"user_id": userID}, nil
}
