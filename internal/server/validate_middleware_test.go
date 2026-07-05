package server

import (
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
)

func vmRC(root map[string]any, routes map[string]map[string]any) *config.ResolvedConfig {
	if root == nil {
		root = map[string]any{}
	}
	return &config.ResolvedConfig{Root: root, Routes: routes}
}

func errsContain(t *testing.T, errs []error, substr string) {
	t.Helper()
	for _, e := range errs {
		if strings.Contains(e.Error(), substr) {
			return
		}
	}
	t.Fatalf("expected an error containing %q, got %v", substr, errs)
}

func TestValidateMiddlewareBuilds_LimiterWithoutConfig(t *testing.T) {
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"limiter"}},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), "max=0")
}

func TestValidateMiddlewareBuilds_ValidConfigPasses(t *testing.T) {
	rc := vmRC(map[string]any{
		"middleware": map[string]any{
			"limiter": map[string]any{"max": float64(20), "expiration": "1m"},
		},
	}, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"limiter", "compress"}},
	})
	if errs := ValidateMiddlewareBuilds(rc); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateMiddlewareBuilds_UnknownMiddleware(t *testing.T) {
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"no.such.middleware"}},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), "unknown middleware")
}

func TestValidateMiddlewareBuilds_RedisStorageNotContacted(t *testing.T) {
	// redis_url points nowhere routable — validation must not try to connect
	rc := vmRC(map[string]any{
		"middleware": map[string]any{
			"limiter":     map[string]any{"max": float64(5), "storage": "redis", "redis_url": "redis://192.0.2.1:6379"},
			"idempotency": map[string]any{"storage": "redis", "redis_url": "redis://192.0.2.1:6379"},
		},
	}, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"limiter", "idempotency"}},
	})
	if errs := ValidateMiddlewareBuilds(rc); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateMiddlewareBuilds_OIDCConfigCheckedWithoutDiscovery(t *testing.T) {
	// missing issuer_url → error
	rc := vmRC(map[string]any{
		"security": map[string]any{"oidc": map[string]any{"client_id": "app"}},
	}, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"auth.oidc"}},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), "issuer_url")

	// complete config → no error and no discovery fetch (unroutable issuer)
	rc = vmRC(map[string]any{
		"security": map[string]any{"oidc": map[string]any{"issuer_url": "https://192.0.2.1/realm", "client_id": "app"}},
	}, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"auth.oidc"}},
	})
	if errs := ValidateMiddlewareBuilds(rc); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateMiddlewareBuilds_AuthSessionSkipped(t *testing.T) {
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"auth.session"}},
	})
	if errs := ValidateMiddlewareBuilds(rc); len(errs) != 0 {
		t.Fatalf("auth.session must not be built at validate time, got %v", errs)
	}
}

func TestValidateMiddlewareBuilds_JWTWithoutSecret(t *testing.T) {
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"auth.jwt"}},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), "auth.jwt")
}

func TestValidateMiddlewareBuilds_PresetExpansion(t *testing.T) {
	rc := vmRC(map[string]any{
		"middleware_presets": map[string]any{
			"public": []any{"limiter"},
		},
	}, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware_preset": "public"},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), "max=0")
}

func TestValidateMiddlewareBuilds_GlobalMiddleware(t *testing.T) {
	rc := vmRC(map[string]any{
		"global_middleware": []any{"timeout"},
		"middleware": map[string]any{
			"timeout": map[string]any{"duration": "not-a-duration"},
		},
	}, nil)
	errsContain(t, ValidateMiddlewareBuilds(rc), "invalid duration")
}

func TestValidateMiddlewareBuilds_ConnectionEndpoints(t *testing.T) {
	rc := vmRC(nil, nil)
	rc.Connections = map[string]map[string]any{
		"chat": {
			"endpoints": map[string]any{
				"room": map[string]any{"middleware": []any{"limiter"}},
			},
		},
	}
	errsContain(t, ValidateMiddlewareBuilds(rc), "max=0")
}

func TestValidateMiddlewareBuilds_OrderingViolation(t *testing.T) {
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"casbin.enforce"}},
	})
	if errs := ValidateMiddlewareBuilds(rc); len(errs) == 0 {
		t.Fatal("casbin.enforce without a preceding auth middleware must fail validation")
	}
}
