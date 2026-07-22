package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/dberr"
	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type createSessionDescriptor struct{}

func (d *createSessionDescriptor) Name() string { return "create_session" }
func (d *createSessionDescriptor) Description() string {
	return "Mints an opaque session token and stores its hash"
}
func (d *createSessionDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"auth":     {Prefix: "auth", Required: true},
		"database": {Prefix: "db", Required: true},
	}
}
func (d *createSessionDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user_id": map[string]any{"type": "string", "description": "User id (expression)"},
			"ttl":     map[string]any{"type": "string", "description": "Session lifetime (e.g. \"720h\"); defaults to service config"},
		},
		"required": []any{"user_id"},
	}
}
func (d *createSessionDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "{token, session_id, expires_at, cookie} — pass cookie to response.json cookies",
		"error":   "Infrastructure error",
	}
}

type createSessionExecutor struct{}

func newCreateSessionExecutor(_ map[string]any) api.NodeExecutor { return &createSessionExecutor{} }

func (e *createSessionExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *createSessionExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "auth")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_session: %w", err)
	}
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_session: %w", err)
	}
	userID, err := plugin.ResolveString(nCtx, config, "user_id")
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_session: %w", err)
	}
	ttl := svc.SessionTTL
	if v, ok, err := plugin.ResolveOptionalString(nCtx, config, "ttl"); err != nil {
		return "", nil, fmt.Errorf("auth.create_session: %w", err)
	} else if ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return "", nil, fmt.Errorf("auth.create_session: ttl: %w", err)
		}
		ttl = d
	}

	raw, hash, err := MintToken()
	if err != nil {
		return "", nil, fmt.Errorf("auth.create_session: %w", err)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	sessionID := uuid.NewString()
	row := map[string]any{
		"id": sessionID, "user_id": userID, "token_hash": hash,
		"created_at": now, "expires_at": expiresAt,
	}
	// request metadata is only present for HTTP-triggered workflows; leave
	// the columns NULL elsewhere (schedules, events, tests)
	trig := nCtx.Trigger()
	if trig.ClientIP != "" {
		row["ip"] = trig.ClientIP
	}
	if trig.UserAgent != "" {
		row["user_agent"] = trig.UserAgent
	}
	if err := db.WithContext(ctx).Table("auth_sessions").Create(row).Error; err != nil {
		return "", nil, dberr.ClassifyOr(err, "session", "auth.create_session")
	}
	return api.OutputSuccess, map[string]any{
		"token":      raw,
		"session_id": sessionID,
		"expires_at": expiresAt,
		"cookie":     svc.SessionCookieObject(raw, ttl),
	}, nil
}
