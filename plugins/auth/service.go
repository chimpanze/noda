package auth

import (
	"fmt"
	"sync"
	"time"
)

const (
	PurposeVerifyEmail   = "verify_email"
	PurposeResetPassword = "reset_password"
)

// CookieConfig describes the session cookie shape used by auth.create_session
// outputs and the auth.session middleware.
type CookieConfig struct {
	Name     string
	Path     string
	Domain   string
	SameSite string
	Secure   bool
	HTTPOnly bool
}

// Service holds validated auth configuration. It has no DB handle; nodes and
// middleware receive the DB separately.
type Service struct {
	DatabaseName string
	SessionTTL   time.Duration
	Cookie       CookieConfig
	Argon        ArgonParams
	TokenTTLs    map[string]time.Duration

	// dummy hash for timing-safe unknown-email verification, derived from
	// Argon on first use (see VerifyDummy)
	dummyOnce sync.Once
	dummyHash string
}

func (s *Service) TokenTTL(purpose string) time.Duration {
	if ttl, ok := s.TokenTTLs[purpose]; ok {
		return ttl
	}
	return time.Hour
}

// SessionCookieObject builds a cookie map consumable by response.json's
// `cookies` config (see plugins/core/response/json.go toCookies): numbers must
// be float64, keys are snake_case.
func (s *Service) SessionCookieObject(rawToken string, ttl time.Duration) map[string]any {
	return map[string]any{
		"name":      s.Cookie.Name,
		"value":     rawToken,
		"path":      s.Cookie.Path,
		"domain":    s.Cookie.Domain,
		"max_age":   float64(int(ttl.Seconds())),
		"secure":    s.Cookie.Secure,
		"http_only": s.Cookie.HTTPOnly,
		"same_site": s.Cookie.SameSite,
	}
}

// ClearCookieObject builds a cookie map that deletes the session cookie.
func (s *Service) ClearCookieObject() map[string]any {
	c := s.SessionCookieObject("", 0)
	c["max_age"] = float64(-1)
	return c
}

func newService(config map[string]any) (*Service, error) {
	dbName, _ := config["database"].(string)
	if dbName == "" {
		return nil, fmt.Errorf("auth: 'database' (service name) is required")
	}
	svc := &Service{
		DatabaseName: dbName,
		SessionTTL:   720 * time.Hour,
		Cookie: CookieConfig{
			Name: "noda_session", Path: "/", SameSite: "Lax", Secure: true, HTTPOnly: true,
		},
		Argon: DefaultArgonParams(),
		TokenTTLs: map[string]time.Duration{
			PurposeVerifyEmail:   24 * time.Hour,
			PurposeResetPassword: time.Hour,
		},
	}
	if sess, ok := config["session"].(map[string]any); ok {
		if v, ok := sess["ttl"].(string); ok {
			d, err := time.ParseDuration(v)
			if err != nil {
				return nil, fmt.Errorf("auth: session.ttl: %w", err)
			}
			svc.SessionTTL = d
		}
		if ck, ok := sess["cookie"].(map[string]any); ok {
			if v, ok := ck["name"].(string); ok && v != "" {
				svc.Cookie.Name = v
			}
			if v, ok := ck["path"].(string); ok && v != "" {
				svc.Cookie.Path = v
			}
			if v, ok := ck["domain"].(string); ok {
				svc.Cookie.Domain = v
			}
			if v, ok := ck["same_site"].(string); ok && v != "" {
				svc.Cookie.SameSite = v
			}
			if v, ok := ck["secure"].(bool); ok {
				svc.Cookie.Secure = v
			}
			if v, ok := ck["http_only"].(bool); ok {
				svc.Cookie.HTTPOnly = v
			}
		}
	}
	if ar, ok := config["argon2"].(map[string]any); ok {
		setU32 := func(key string, dst *uint32) {
			if f, ok := ar[key].(float64); ok && f > 0 {
				*dst = uint32(f)
			}
		}
		setU32("memory_kib", &svc.Argon.MemoryKiB)
		setU32("iterations", &svc.Argon.Iterations)
		setU32("salt_len", &svc.Argon.SaltLen)
		setU32("key_len", &svc.Argon.KeyLen)
		if f, ok := ar["parallelism"].(float64); ok && f > 0 {
			svc.Argon.Parallelism = uint8(f)
		}
	}
	if tk, ok := config["tokens"].(map[string]any); ok {
		parse := func(key, purpose string) error {
			if v, ok := tk[key].(string); ok {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("auth: tokens.%s: %w", key, err)
				}
				svc.TokenTTLs[purpose] = d
			}
			return nil
		}
		if err := parse("verify_email_ttl", PurposeVerifyEmail); err != nil {
			return nil, err
		}
		if err := parse("reset_password_ttl", PurposeResetPassword); err != nil {
			return nil, err
		}
	}
	return svc, nil
}
