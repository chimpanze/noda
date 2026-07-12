# Server Edge Correctness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Trusted-proxy support (`server.trust_proxy` → Fiber `TrustProxy`) and env-settable numeric server settings with loud validation, closing #300 and #301, plus the homebase config flip that consumes both.

**Architecture:** Coercion/parse helpers live in `internal/config` (`ServerInt`, `ServerDuration`, `ServerTrustProxy`) so every reader — `internal/server`, `internal/registry`, `cmd/noda` — shares one implementation. Load-time validation extends the existing crossrefs duration gate (`internal/config/crossrefs.go`) so `noda validate` catches bad values with JSONPath errors; startup constructors (`NewServer`, `Bootstrap`) additionally propagate helper errors, while per-request/display readers (response_timeout, health_timeout, openapi port, shutdown_deadline) keep their defaults on error because the load gate already ran.

**Tech Stack:** Go, gofiber/fiber/v3 v3.1.0 (`TrustProxy`/`TrustProxyConfig`/`ProxyHeader` — verified in vendored source), santhosh-tekuri/jsonschema (root.json), testify.

**Spec:** `docs/superpowers/specs/2026-07-12-server-edge-correctness-design.md`

## Global Constraints

- Fiber facts (verified in `~/go/pkg/mod/github.com/gofiber/fiber/v3@v3.1.0/`): invalid `Proxies` entries are only `log.Warnf`'d and skipped by Fiber (`app.go:698`); `TrustProxy: true` with empty config trusts nothing silently; `app.Test` connections arrive from remote IP `0.0.0.0` (`helpers.go:903`); the limiter's default `KeyGenerator` is `c.IP()`. Hence WE validate, and tests key off `0.0.0.0`.
- `$env()` (`internal/secrets/resolve.go`) resolves to strings only, has NO default-value form, and a missing variable is a hard config error. Pipeline order: resolve `$env()` → schema validation → crossrefs — so schemas must accept the post-resolution string forms.
- With `trust_proxy` absent or `enabled: false`, server behavior must be byte-for-byte unchanged (no `ProxyHeader` set).
- Deliberate behavior change (CHANGELOG "Changed"): invalid `server.*` scalar values now fail validation/startup instead of silently falling back to defaults.
- Fiber's `unix_socket` trust flag is deliberately NOT exposed (YAGNI).
- Local gate before every commit: `gofmt -l .` (must print nothing), `go vet ./...`, and the tests named in the task. CI runs golangci-lint; formatting alone fails it.
- Conventional commit messages, `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>` trailer.

## Worktree setup (before Task 1)

```bash
git -C /Users/marten/GolandProjects/noda worktree add .worktrees/server-edge-correctness -b feat/server-edge-correctness main
cd /Users/marten/GolandProjects/noda/.worktrees/server-edge-correctness
git add -f docs/superpowers/specs/2026-07-12-server-edge-correctness-design.md docs/superpowers/plans/2026-07-12-server-edge-correctness.md
git commit -m "docs: spec + plan for server edge correctness tranche (#300, #301)"
```

(The spec/plan files were written in the main checkout; copy them into the worktree first if `git add -f` finds them missing: they are gitignored, so `git worktree add` does not carry them over — `cp ../../docs/superpowers/specs/2026-07-12-server-edge-correctness-design.md docs/superpowers/specs/` etc.)

---

### Task 1: Config setting helpers (`ServerInt`, `ServerDuration`, `ServerTrustProxy`)

**Files:**
- Create: `internal/config/settings.go`
- Test: `internal/config/settings_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces (later tasks call these exactly as written):
  - `func ServerInt(root map[string]any, key string) (int, bool, error)` — reads `server.<key>`; accepts `float64` (JSON number) or numeric string (post-`$env()`); `ok=false` when section/key absent; error on garbage.
  - `func ServerDuration(root map[string]any, key string) (time.Duration, bool, error)` — same shape for Go duration strings.
  - `type TrustProxy struct { Proxies []string; Loopback, LinkLocal, Private bool; Header string }`
  - `func ServerTrustProxy(root map[string]any) (*TrustProxy, error)` — `nil, nil` when absent or `enabled: false`; when enabled: `Header` defaults to `"X-Forwarded-For"`, every `proxies` entry must parse as IP or CIDR, and the trusted set must be non-empty (proxies or a class boolean), else error.

- [ ] **Step 1: Write the failing tests**

Create `internal/config/settings_test.go`:

```go
package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rootWith(server map[string]any) map[string]any {
	return map[string]any{"server": server}
}

func TestServerInt(t *testing.T) {
	tests := []struct {
		name    string
		root    map[string]any
		wantVal int
		wantOK  bool
		wantErr string
	}{
		{"json number", rootWith(map[string]any{"body_limit": float64(1048576)}), 1048576, true, ""},
		{"numeric string from env", rootWith(map[string]any{"body_limit": "1073741824"}), 1073741824, true, ""},
		{"numeric string with spaces", rootWith(map[string]any{"body_limit": " 42 "}), 42, true, ""},
		{"absent key", rootWith(map[string]any{}), 0, false, ""},
		{"absent server section", map[string]any{}, 0, false, ""},
		{"garbage string", rootWith(map[string]any{"body_limit": "10GB"}), 0, false, `server.body_limit`},
		{"wrong type", rootWith(map[string]any{"body_limit": true}), 0, false, `server.body_limit`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok, err := ServerInt(tt.root, "body_limit")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantVal, v)
		})
	}
}

func TestServerDuration(t *testing.T) {
	v, ok, err := ServerDuration(rootWith(map[string]any{"read_timeout": "30s"}), "read_timeout")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 30*time.Second, v)

	_, ok, err = ServerDuration(rootWith(map[string]any{}), "read_timeout")
	require.NoError(t, err)
	assert.False(t, ok)

	_, _, err = ServerDuration(rootWith(map[string]any{"read_timeout": "banana"}), "read_timeout")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server.read_timeout")

	_, _, err = ServerDuration(rootWith(map[string]any{"read_timeout": float64(30)}), "read_timeout")
	require.Error(t, err) // durations are strings; numbers were never supported
}

func TestServerTrustProxy(t *testing.T) {
	t.Run("absent -> nil", func(t *testing.T) {
		tp, err := ServerTrustProxy(rootWith(map[string]any{}))
		require.NoError(t, err)
		assert.Nil(t, tp)
	})
	t.Run("disabled -> nil even with entries", func(t *testing.T) {
		tp, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": false, "proxies": []any{"not-an-ip"},
		}}))
		require.NoError(t, err)
		assert.Nil(t, tp)
	})
	t.Run("enabled full object", func(t *testing.T) {
		tp, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true, "proxies": []any{"10.0.0.0/8", "192.0.2.1"},
			"private": true, "loopback": true, "link_local": true, "header": "X-Real-Ip",
		}}))
		require.NoError(t, err)
		require.NotNil(t, tp)
		assert.Equal(t, []string{"10.0.0.0/8", "192.0.2.1"}, tp.Proxies)
		assert.True(t, tp.Private)
		assert.True(t, tp.Loopback)
		assert.True(t, tp.LinkLocal)
		assert.Equal(t, "X-Real-Ip", tp.Header)
	})
	t.Run("header defaults to X-Forwarded-For", func(t *testing.T) {
		tp, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true, "private": true,
		}}))
		require.NoError(t, err)
		assert.Equal(t, "X-Forwarded-For", tp.Header)
	})
	t.Run("enabled but trusts nothing -> error", func(t *testing.T) {
		_, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true,
		}}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trusts nothing")
	})
	t.Run("invalid CIDR -> error", func(t *testing.T) {
		_, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true, "proxies": []any{"10.0.0.0/99"},
		}}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "proxies[0]")
	})
	t.Run("invalid IP -> error", func(t *testing.T) {
		_, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true, "proxies": []any{"caddy"},
		}}))
		require.Error(t, err)
	})
	t.Run("non-string proxies entry -> error", func(t *testing.T) {
		_, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true, "proxies": []any{float64(7)},
		}}))
		require.Error(t, err)
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestServerInt|TestServerDuration|TestServerTrustProxy' -v`
Expected: FAIL — `undefined: ServerInt` (compile error).

- [ ] **Step 3: Implement `internal/config/settings.go`**

```go
package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Server setting readers. Values under "server" may arrive as JSON numbers or
// as strings produced by $env() resolution (internal/secrets/resolve.go always
// substitutes strings), so numeric settings accept both forms. Invalid values
// are errors, never silent fallbacks.

func serverSection(root map[string]any) (map[string]any, bool) {
	m, ok := root["server"].(map[string]any)
	return m, ok
}

// ServerInt reads server.<key> as an integer, accepting a JSON number or a
// numeric string. ok is false when the server section or key is absent.
func ServerInt(root map[string]any, key string) (int, bool, error) {
	m, ok := serverSection(root)
	if !ok {
		return 0, false, nil
	}
	v, ok := m[key]
	if !ok {
		return 0, false, nil
	}
	switch n := v.(type) {
	case float64:
		return int(n), true, nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false, fmt.Errorf("server.%s: %q is not an integer", key, n)
		}
		return i, true, nil
	default:
		return 0, false, fmt.Errorf("server.%s: expected integer or string, got %T", key, v)
	}
}

// ServerDuration reads server.<key> as a Go duration string.
// ok is false when the server section or key is absent.
func ServerDuration(root map[string]any, key string) (time.Duration, bool, error) {
	m, ok := serverSection(root)
	if !ok {
		return 0, false, nil
	}
	v, ok := m[key]
	if !ok {
		return 0, false, nil
	}
	s, isStr := v.(string)
	if !isStr {
		return 0, false, fmt.Errorf("server.%s: expected duration string, got %T", key, v)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, false, fmt.Errorf("server.%s: invalid duration %q: %v", key, s, err)
	}
	return d, true, nil
}

// TrustProxy is the parsed server.trust_proxy block. A nil *TrustProxy means
// the feature is off (block absent or enabled: false).
type TrustProxy struct {
	Proxies   []string
	Loopback  bool
	LinkLocal bool
	Private   bool
	Header    string
}

// ServerTrustProxy parses and validates server.trust_proxy. Fiber itself only
// warn-logs invalid proxy entries and silently trusts nothing when the set is
// empty, so both are hard errors here.
func ServerTrustProxy(root map[string]any) (*TrustProxy, error) {
	m, ok := serverSection(root)
	if !ok {
		return nil, nil
	}
	raw, ok := m["trust_proxy"]
	if !ok {
		return nil, nil
	}
	cfg, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("server.trust_proxy: expected object, got %T", raw)
	}
	if enabled, _ := cfg["enabled"].(bool); !enabled {
		return nil, nil
	}
	tp := &TrustProxy{Header: "X-Forwarded-For"}
	if h, ok := cfg["header"].(string); ok && h != "" {
		tp.Header = h
	}
	tp.Loopback, _ = cfg["loopback"].(bool)
	tp.LinkLocal, _ = cfg["link_local"].(bool)
	tp.Private, _ = cfg["private"].(bool)
	if rawList, ok := cfg["proxies"].([]any); ok {
		for i, item := range rawList {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("server.trust_proxy.proxies[%d]: expected string, got %T", i, item)
			}
			if strings.Contains(s, "/") {
				if _, _, err := net.ParseCIDR(s); err != nil {
					return nil, fmt.Errorf("server.trust_proxy.proxies[%d]: invalid CIDR %q", i, s)
				}
			} else if net.ParseIP(s) == nil {
				return nil, fmt.Errorf("server.trust_proxy.proxies[%d]: invalid IP %q", i, s)
			}
			tp.Proxies = append(tp.Proxies, s)
		}
	}
	if len(tp.Proxies) == 0 && !tp.Loopback && !tp.LinkLocal && !tp.Private {
		return nil, fmt.Errorf("server.trust_proxy: enabled but trusts nothing — set proxies or one of loopback/link_local/private")
	}
	return tp, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run 'TestServerInt|TestServerDuration|TestServerTrustProxy' -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Gate and commit**

```bash
gofmt -l . && go vet ./internal/config/
git add internal/config/settings.go internal/config/settings_test.go
git commit -m "feat(config): typed server-setting readers (int/duration/trust_proxy) with env-string coercion"
```

---

### Task 2: Schema + crossrefs validation

**Files:**
- Modify: `internal/config/schemas/root.json` (server section, ~lines 89-101)
- Modify: `internal/config/crossrefs.go:220-236` (the "Validate duration fields in server config" block)
- Test: `internal/config/crossrefs_test.go` (append; match the file's existing test style — it builds `ResolvedConfig` values in memory)

**Interfaces:**
- Consumes: `ServerInt`, `ServerDuration`, `ServerTrustProxy` from Task 1.
- Produces: `noda validate` (which runs crossrefs) reports bad server scalars/trust_proxy as `ValidationError{FilePath: "noda.json", JSONPath: "/server/<key>"}`.

- [ ] **Step 1: Update `root.json` server section**

Replace the `"server"` property block with:

```json
"server": {
  "type": "object",
  "properties": {
    "port": {
      "oneOf": [
        { "type": "integer", "minimum": 1, "maximum": 65535 },
        { "type": "string" }
      ]
    },
    "read_timeout": { "type": "string" },
    "write_timeout": { "type": "string" },
    "body_limit": {
      "oneOf": [
        { "type": "integer", "minimum": 0 },
        { "type": "string" }
      ]
    },
    "response_timeout": { "type": "string" },
    "shutdown_deadline": { "type": "string" },
    "health_timeout": { "type": "string" },
    "expression_memory_budget": {
      "oneOf": [
        { "type": "integer", "minimum": 0 },
        { "type": "string" }
      ]
    },
    "expression_strict_mode": { "type": "boolean" },
    "trust_proxy": {
      "type": "object",
      "properties": {
        "enabled": { "type": "boolean" },
        "proxies": { "type": "array", "items": { "type": "string" } },
        "loopback": { "type": "boolean" },
        "link_local": { "type": "boolean" },
        "private": { "type": "boolean" },
        "header": { "type": "string" }
      }
    }
  }
}
```

Notes: `health_timeout` was read by `internal/server/health.go` but missing from the schema — this adds it. String branches carry no pattern; crossrefs (next step) does the semantic check with a good error message.

- [ ] **Step 2: Write the failing crossrefs tests**

Append to `internal/config/crossrefs_test.go` (adjust the config-construction boilerplate to match how the existing duration tests in that file build their input — reuse their helper if one exists):

```go
func TestCrossrefs_ServerScalarValidation(t *testing.T) {
	cases := []struct {
		name    string
		server  map[string]any
		wantErr string // substring of the reported message; "" = no error expected
	}{
		{"env-resolved numeric string ok", map[string]any{"body_limit": "1073741824"}, ""},
		{"garbage body_limit", map[string]any{"body_limit": "10GB"}, "body_limit"},
		{"port out of range", map[string]any{"port": "70000"}, "out of range"},
		{"negative body_limit string", map[string]any{"body_limit": "-1"}, "out of range"},
		{"bad shutdown_deadline", map[string]any{"shutdown_deadline": "soon"}, "invalid duration"},
		{"bad health_timeout", map[string]any{"health_timeout": "later"}, "invalid duration"},
		{"trust_proxy trusts nothing", map[string]any{"trust_proxy": map[string]any{"enabled": true}}, "trusts nothing"},
		{"trust_proxy bad cidr", map[string]any{"trust_proxy": map[string]any{"enabled": true, "proxies": []any{"nope"}}}, "invalid IP"},
		{"trust_proxy valid", map[string]any{"trust_proxy": map[string]any{"enabled": true, "private": true}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := &ResolvedConfig{Root: map[string]any{"server": tc.server}}
			errs := ValidateCrossRefs(rc) // use the actual exported entry point of crossrefs.go
			var hit bool
			for _, e := range errs {
				if tc.wantErr != "" && strings.Contains(e.Message, tc.wantErr) {
					hit = true
				}
			}
			if tc.wantErr == "" {
				for _, e := range errs {
					assert.NotContains(t, e.JSONPath, "/server/", "unexpected server error: %s", e.Message)
				}
			} else {
				assert.True(t, hit, "expected error containing %q, got %v", tc.wantErr, errs)
			}
		})
	}
}
```

(If the crossrefs entry point has a different name/signature, use the one the existing duration tests call.)

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/config/ -run TestCrossrefs_ServerScalarValidation -v`
Expected: FAIL — garbage/range/trust_proxy cases produce no errors yet (only read/write/response_timeout are validated today).

- [ ] **Step 4: Extend crossrefs.go**

Replace the existing "Validate duration fields in server config" block (crossrefs.go:220-236) with:

```go
	// Validate server scalar settings. These may be strings after $env()
	// resolution, so the schema admits both forms and the semantic check
	// happens here.
	if rc.Root != nil {
		if _, ok := rc.Root["server"].(map[string]any); ok {
			for _, field := range []string{"read_timeout", "write_timeout", "response_timeout", "shutdown_deadline", "health_timeout"} {
				if _, _, err := ServerDuration(rc.Root, field); err != nil {
					errs = append(errs, ValidationError{
						FilePath: "noda.json",
						JSONPath: "/server/" + field,
						Message:  err.Error(),
					})
				}
			}
			for _, f := range []struct {
				key      string
				min, max int
			}{
				{"port", 1, 65535},
				{"body_limit", 0, math.MaxInt},
				{"expression_memory_budget", 0, math.MaxInt},
			} {
				v, ok, err := ServerInt(rc.Root, f.key)
				switch {
				case err != nil:
					errs = append(errs, ValidationError{
						FilePath: "noda.json",
						JSONPath: "/server/" + f.key,
						Message:  err.Error(),
					})
				case ok && (v < f.min || v > f.max):
					errs = append(errs, ValidationError{
						FilePath: "noda.json",
						JSONPath: "/server/" + f.key,
						Message:  fmt.Sprintf("value %d out of range [%d, %d]", v, f.min, f.max),
					})
				}
			}
			if _, err := ServerTrustProxy(rc.Root); err != nil {
				errs = append(errs, ValidationError{
					FilePath: "noda.json",
					JSONPath: "/server/trust_proxy",
					Message:  err.Error(),
				})
			}
		}
	}
```

Add `"math"` to the imports. Keep the message text `invalid duration` (ServerDuration already produces it) so existing duration-validation tests keep passing.

- [ ] **Step 5: Run the package tests**

Run: `go test ./internal/config/ ./internal/secrets/`
Expected: PASS — including the pre-existing duration-validation tests (the new block must cover read/write/response_timeout exactly as before).

- [ ] **Step 6: Gate and commit**

```bash
gofmt -l . && go vet ./internal/config/
git add internal/config/schemas/root.json internal/config/crossrefs.go internal/config/crossrefs_test.go
git commit -m "feat(config): validate server scalars + trust_proxy at load; schema admits env-string numerics (#300, #301)"
```

---

### Task 3: NewServer wiring — coercion, trust_proxy → fiber.Config, behavioral tests

**Files:**
- Modify: `internal/server/server.go:122-140` (the "Read server settings" block in `NewServer`)
- Test: `internal/server/server_test.go` (append)

**Interfaces:**
- Consumes: `config.ServerInt`, `config.ServerDuration`, `config.ServerTrustProxy` (Task 1). `internal/server` already imports `internal/config`.
- Produces: `NewServer` returns an error for any invalid `server.*` scalar or trust_proxy block. With trust_proxy enabled, the fiber app is built with `TrustProxy: true`, mapped `TrustProxyConfig`, and `ProxyHeader`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/server/server_test.go` (imports to add if missing: `net/http/httptest`, `io`, `github.com/gofiber/fiber/v3`):

```go
func TestNewServer_BodyLimitFromEnvString(t *testing.T) {
	rc := testConfig()
	rc.Root["server"] = map[string]any{"body_limit": "1073741824"}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	assert.Equal(t, 1073741824, srv.App().Config().BodyLimit)
}

func TestNewServer_PortFromEnvString(t *testing.T) {
	rc := testConfig()
	rc.Root["server"] = map[string]any{"port": "8080"}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	assert.Equal(t, 8080, srv.Port())
}

func TestNewServer_InvalidScalarsFailLoudly(t *testing.T) {
	cases := []struct {
		name   string
		server map[string]any
		want   string
	}{
		{"garbage body_limit", map[string]any{"body_limit": "10GB"}, "body_limit"},
		{"garbage port", map[string]any{"port": "http"}, "port"},
		{"bad read_timeout", map[string]any{"read_timeout": "banana"}, "read_timeout"},
		{"bad write_timeout", map[string]any{"write_timeout": "fast"}, "write_timeout"},
		{"trust_proxy trusts nothing", map[string]any{"trust_proxy": map[string]any{"enabled": true}}, "trusts nothing"},
		{"trust_proxy bad entry", map[string]any{"trust_proxy": map[string]any{"enabled": true, "proxies": []any{"10.0.0.0/99"}}}, "proxies[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := testConfig()
			rc.Root["server"] = tc.server
			_, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestNewServer_TrustProxyOffByDefault(t *testing.T) {
	rc := testConfig()
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	assert.False(t, srv.App().Config().TrustProxy)
	assert.Empty(t, srv.App().Config().ProxyHeader)
}

func TestNewServer_TrustProxyMapping(t *testing.T) {
	rc := testConfig()
	rc.Root["server"] = map[string]any{"trust_proxy": map[string]any{
		"enabled": true,
		"proxies": []any{"10.0.0.0/8"},
		"private": true,
		"header":  "X-Real-Ip",
	}}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	cfg := srv.App().Config()
	assert.True(t, cfg.TrustProxy)
	assert.Equal(t, "X-Real-Ip", cfg.ProxyHeader)
	assert.Equal(t, []string{"10.0.0.0/8"}, cfg.TrustProxyConfig.Proxies)
	assert.True(t, cfg.TrustProxyConfig.Private)
	assert.False(t, cfg.TrustProxyConfig.Loopback)
}

// app.Test connections arrive from 0.0.0.0 (fiber v3.1.0 testConn), so
// "0.0.0.0/0" makes the test client a trusted proxy and "198.51.100.0/24"
// makes it untrusted.
func trustProxyTestServer(t *testing.T, proxies []any) *Server {
	t.Helper()
	rc := testConfig()
	rc.Root["server"] = map[string]any{"trust_proxy": map[string]any{
		"enabled": true, "proxies": proxies,
	}}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	srv.App().Get("/ip", func(c fiber.Ctx) error { return c.SendString(c.IP()) })
	return srv
}

func TestServer_TrustedProxy_IPFromForwardedHeader(t *testing.T) {
	srv := trustProxyTestServer(t, []any{"0.0.0.0/0"})
	req := httptest.NewRequest("GET", "/ip", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "203.0.113.7", string(body))
}

// Polarity check: this test MUST fail against a build that blindly trusts the
// header (e.g. ProxyHeader set without TrustProxy filtering).
func TestServer_UntrustedProxy_ForwardedHeaderIgnored(t *testing.T) {
	srv := trustProxyTestServer(t, []any{"198.51.100.0/24"})
	req := httptest.NewRequest("GET", "/ip", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.NotEqual(t, "203.0.113.7", string(body))
	assert.Equal(t, "0.0.0.0", string(body)) // socket remote IP, not the spoofed header
}

func TestServer_LimiterKeysOnForwardedIP(t *testing.T) {
	srv := trustProxyTestServer(t, []any{"0.0.0.0/0"})
	// Check parseLimiterConfig (middleware.go:245) for the exact config keys
	// if this construction fails; "max" is read as float64 at :250.
	h, err := newLimiterMiddleware(map[string]any{"max": float64(1), "expiration": "1m"}, nil)
	require.NoError(t, err)
	srv.App().Use(h)
	srv.App().Get("/limited", func(c fiber.Ctx) error { return c.SendString("ok") })

	do := func(xff string) int {
		req := httptest.NewRequest("GET", "/limited", nil)
		req.Header.Set("X-Forwarded-For", xff)
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		return resp.StatusCode
	}
	assert.Equal(t, 200, do("203.0.113.7"))  // client A, first hit
	assert.Equal(t, 200, do("203.0.113.8"))  // client B gets own bucket
	assert.Equal(t, 429, do("203.0.113.7"))  // client A exhausted its bucket
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/server/ -run 'TestNewServer_BodyLimitFromEnvString|TestNewServer_PortFromEnvString|TestNewServer_InvalidScalarsFailLoudly|TestNewServer_TrustProxy|TestServer_TrustedProxy|TestServer_UntrustedProxy|TestServer_LimiterKeys' -v`
Expected: FAIL — env-string cases silently keep defaults (body limit stays 5 MB / port 3000), invalid cases return no error, trust_proxy cases find `TrustProxy` false.

- [ ] **Step 3: Rewrite the settings block in `NewServer`**

Replace `internal/server/server.go:122-140` (the `if serverCfg, ok := rc.Root["server"]...` block) with:

```go
	// Read server settings from root config. Values may be numeric strings
	// after $env() resolution; invalid values fail startup loudly.
	if p, ok, err := config.ServerInt(rc.Root, "port"); err != nil {
		return nil, err
	} else if ok {
		s.port = p
	}
	if d, ok, err := config.ServerDuration(rc.Root, "read_timeout"); err != nil {
		return nil, err
	} else if ok {
		fiberCfg.ReadTimeout = d
	}
	if d, ok, err := config.ServerDuration(rc.Root, "write_timeout"); err != nil {
		return nil, err
	} else if ok {
		fiberCfg.WriteTimeout = d
	}
	if v, ok, err := config.ServerInt(rc.Root, "body_limit"); err != nil {
		return nil, err
	} else if ok {
		fiberCfg.BodyLimit = v
	}
	tp, err := config.ServerTrustProxy(rc.Root)
	if err != nil {
		return nil, err
	}
	if tp != nil {
		fiberCfg.TrustProxy = true
		fiberCfg.TrustProxyConfig = fiber.TrustProxyConfig{
			Proxies:   tp.Proxies,
			Loopback:  tp.Loopback,
			LinkLocal: tp.LinkLocal,
			Private:   tp.Private,
		}
		fiberCfg.ProxyHeader = tp.Header
	}
```

- [ ] **Step 4: Run the full server package**

Run: `go test ./internal/server/`
Expected: PASS — new tests and all pre-existing ones (`TestNewServer_CustomPort`, `TestNewServer_Timeouts` still use float64/valid strings and must be untouched).

- [ ] **Step 5: Gate and commit**

```bash
gofmt -l . && go vet ./internal/server/
git add internal/server/server.go internal/server/server_test.go
git commit -m "feat(server): trust_proxy support + env-string numeric settings, fail loudly on invalid values (#300, #301)"
```

---

### Task 4: Route the remaining `server.*` readers through the helpers

**Files:**
- Modify: `internal/server/openapi.go:25-31` (port)
- Modify: `internal/server/routes.go:418-423` (response_timeout)
- Modify: `internal/server/health.go:14-23` (health_timeout)
- Modify: `internal/registry/bootstrap.go:94-99` (expression_memory_budget)
- Modify: `cmd/noda/main.go:859-868` (`parseShutdownDeadline`)
- Test: `internal/registry/bootstrap_test.go` (append; follow that file's existing setup style)

**Interfaces:**
- Consumes: `config.ServerInt`, `config.ServerDuration` (Task 1).
- Produces: no new API. Policy (from the plan header): `Bootstrap` propagates errors (startup path); openapi/routes/health/shutdown keep their defaults on error because crossrefs validation already gated the load path.

- [ ] **Step 1: Write the failing bootstrap test**

Append to `internal/registry/bootstrap_test.go` (mirror how existing tests in that file construct `rc` and call `Bootstrap`; if no existing test exercises `expression_memory_budget`, model the setup on the nearest `Bootstrap` test):

```go
func TestBootstrap_ExpressionMemoryBudgetFromEnvString(t *testing.T) {
	// arrange rc as the other Bootstrap tests do, then:
	rc.Root["server"] = map[string]any{"expression_memory_budget": "4096"}
	// act: Bootstrap must succeed and NOT report an error
	// (before the fix the string is silently ignored by toUint)
	result, errs := Bootstrap(context.Background(), rc, plugins)
	require.Empty(t, errs)
	require.NotNil(t, result)
}

func TestBootstrap_GarbageExpressionMemoryBudgetErrors(t *testing.T) {
	rc.Root["server"] = map[string]any{"expression_memory_budget": "lots"}
	_, errs := Bootstrap(context.Background(), rc, plugins)
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "expression_memory_budget")
}
```

Note: the env-string acceptance test can only observe "no error" from outside (the budget is internal to the compiler); the garbage test carries the real assertion weight.

- [ ] **Step 2: Run to verify the garbage test fails**

Run: `go test ./internal/registry/ -run TestBootstrap_.*ExpressionMemoryBudget -v`
Expected: garbage-case FAIL (no error reported today — `toUint` just returns `false`).

- [ ] **Step 3: Convert the five readers**

`internal/registry/bootstrap.go` — replace the budget read (keep `expression_strict_mode` as-is):

```go
	if v, ok, err := config.ServerInt(rc.Root, "expression_memory_budget"); err != nil {
		errs = append(errs, err) // match the surrounding []error accumulation style; if the
		// compiler-options section instead returns early on error, do that
	} else if ok && v >= 0 {
		compilerOpts = append(compilerOpts, expr.WithMemoryBudget(uint(v)))
	}
```

Then `grep -n "toUint" internal/registry/` — if the budget read was its only caller, delete `toUint` (bootstrap.go:16-31) and its `encoding/json` import if now unused.

`internal/server/openapi.go`:

```go
	// Add server if configured (errors already surfaced by config validation)
	if port, ok, err := config.ServerInt(rc.Root, "port"); err == nil && ok {
		doc.Servers = openapi3.Servers{
			{URL: fmt.Sprintf("http://localhost:%d", port)},
		}
	}
```

`internal/server/routes.go` (inside the responseTimeout resolution):

```go
		responseTimeout := defaultResponseTimeout
		if routeTimeout > 0 {
			responseTimeout = routeTimeout
		} else if d, ok, err := config.ServerDuration(s.config.Root, "response_timeout"); err == nil && ok {
			responseTimeout = d
		}
```

`internal/server/health.go`:

```go
// healthTimeout returns the configured health check timeout or the default.
func (s *Server) healthTimeout() time.Duration {
	if d, ok, err := config.ServerDuration(s.config.Root, "health_timeout"); err == nil && ok {
		return d
	}
	return defaultHealthTimeout
}
```

`cmd/noda/main.go`:

```go
// parseShutdownDeadline reads the shutdown_deadline from server config, falling back to defaultVal.
func parseShutdownDeadline(rc *config.ResolvedConfig, defaultVal time.Duration) time.Duration {
	if d, ok, err := config.ServerDuration(rc.Root, "shutdown_deadline"); err == nil && ok {
		return d
	}
	return defaultVal
}
```

Add the `config` import where a file doesn't have it yet (health.go likely doesn't).

- [ ] **Step 4: Run the affected packages**

Run: `go test ./internal/server/ ./internal/registry/ ./cmd/noda/`
Expected: PASS.

- [ ] **Step 5: Gate and commit**

```bash
gofmt -l . && go vet ./...
git add internal/server/openapi.go internal/server/routes.go internal/server/health.go internal/registry/bootstrap.go internal/registry/bootstrap_test.go cmd/noda/main.go
git commit -m "refactor: route all server.* scalar readers through config setting helpers (#301)"
```

---

### Task 5: Docs + CHANGELOG

**Files:**
- Modify: `docs/02-config/noda-json.md` (server settings table ~line 23, example ~line 33; add trust_proxy subsection)
- Modify: `CHANGELOG.md` ([Unreleased] section)

**Interfaces:** none (docs only).

- [ ] **Step 1: Update `docs/02-config/noda-json.md`**

1. In the server settings table: note on `port`, `body_limit`, `expression_memory_budget` rows that the value may be an integer **or** a string containing `{{ $env('NAME') }}`; add a `health_timeout` row (duration string, default `5s`) if the table lacks it; add a `trust_proxy` row pointing at the new subsection.
2. Below the server example, add:

```markdown
### Trusted proxies (`server.trust_proxy`)

When noda runs behind a reverse proxy (Caddy, nginx, a cloud load balancer),
the client IP seen by rate limiting and session tracking is the proxy's IP
unless you tell noda which peers to trust:

​```json
{
  "server": {
    "trust_proxy": {
      "enabled": true,
      "proxies": ["10.0.0.0/8"],
      "private": true,
      "header": "X-Forwarded-For"
    }
  }
}
​```

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | boolean | `false` | Master switch. Off = header never trusted. |
| `proxies` | string[] | `[]` | Trusted proxy IPs or CIDR ranges. |
| `loopback` | boolean | `false` | Trust all loopback addresses (127.0.0.0/8, ::1). |
| `link_local` | boolean | `false` | Trust link-local ranges (169.254.0.0/16, fe80::/10). |
| `private` | boolean | `false` | Trust private ranges (10/8, 172.16/12, 192.168/16, fc00::/7) — handy for Docker networks where the proxy IP is dynamic. |
| `header` | string | `"X-Forwarded-For"` | Header the proxy writes the client IP to. |

Only requests arriving **from** a trusted address have the header honored;
direct clients spoofing `X-Forwarded-For` keep their socket IP. Enabling
`trust_proxy` without any trusted set (no `proxies`, no class flag) is a
config error. Only enable this when every hop that can reach noda's port is
your own proxy.

> **Memory note:** the server buffers each request body in memory up to
> `body_limit` *before* auth runs, so a large limit is an unauthenticated
> memory-pressure vector. Keep the edge proxy's own body limit at or below
> noda's, and don't raise `body_limit` beyond what uploads actually need.
```

(Strip the zero-width escapes around the inner code fence.)

- [ ] **Step 2: Update `CHANGELOG.md` [Unreleased]**

- **Added:** `server.trust_proxy` — trusted-proxy support so `c.IP()` (rate limiting, session IPs) sees the real client behind a reverse proxy (#300).
- **Added:** numeric `server.*` settings (`port`, `body_limit`, `expression_memory_budget`) accept `{{ $env('NAME') }}` strings (#301).
- **Changed:** invalid `server.*` scalar values (bad numbers, malformed durations, invalid trust_proxy entries) now fail config validation/startup instead of silently falling back to defaults.

Match the existing entry style in the file; if a `### Security`/`### Added` subsection already exists under [Unreleased], fold into it rather than duplicating headers.

- [ ] **Step 3: Commit**

```bash
git add docs/02-config/noda-json.md CHANGELOG.md
git commit -m "docs: trust_proxy + env-settable server scalars (noda-json reference, CHANGELOG)"
```

---

### Task 6: Homebase flip

**Files:**
- Modify: `projects/homebase/noda.json:2-4` (body_limit)
- Create: `projects/homebase/noda.production.json`
- Modify: `projects/homebase/docker-compose.yml` (noda + migrate service env)
- Modify: `projects/homebase/.env.example`
- Modify: `projects/homebase/README.md:28` (proxy caveat bullet)

**Interfaces:**
- Consumes: the runtime features from Tasks 1-3, and noda's existing `noda.{env}.json` overlay discovery (`internal/config/discovery.go:47`, selected by `NODA_ENV`, default `development`).

- [ ] **Step 1: `noda.json` — body_limit from env**

```json
  "server": {
    "body_limit": "{{ $env('BODY_LIMIT') }}"
  },
```

- [ ] **Step 2: Create `projects/homebase/noda.production.json`**

```json
{
  "server": {
    "trust_proxy": {
      "enabled": true,
      "private": true,
      "header": "X-Forwarded-For"
    }
  }
}
```

Rationale: overlay-scoped so plain dev compose (clients hitting noda directly on 127.0.0.1:3000) never trusts a spoofable header; `private: true` because the Caddy container's compose-network IP is dynamic but always in a Docker private range. The bind mount `.:/app/config` ships the overlay automatically.

- [ ] **Step 3: `docker-compose.yml` env**

In the `noda` service `environment` block add:

```yaml
      BODY_LIMIT: ${BODY_LIMIT:-1073741824}
      NODA_ENV: ${NODA_ENV:-development}
```

In the `migrate` service `environment` block add (it loads the same config, so `$env('BODY_LIMIT')` must resolve there too):

```yaml
      BODY_LIMIT: ${BODY_LIMIT:-1073741824}
```

- [ ] **Step 4: `.env.example`**

Under `# --- optional ---` add:

```bash
# Max request body size in bytes (default 1 GiB)
BODY_LIMIT=1073741824
# Set on the production host so noda loads noda.production.json
# (enables trust_proxy so rate limiting sees real client IPs behind Caddy)
NODA_ENV=production
```

And under `# --- only needed when running noda outside docker ---` add `BODY_LIMIT=1073741824` (outside compose there is no `${BODY_LIMIT:-...}` default and a missing env var is a hard config error).

- [ ] **Step 5: README caveat swap**

Replace the bullet at `projects/homebase/README.md:28` ("Behind the Caddy proxy, per-IP rate limiting ... tracked upstream.") with:

```markdown
- With `NODA_ENV=production` set (see `.env.example`), the `noda.production.json` overlay enables `server.trust_proxy` so per-IP rate limiting and the session device-IP list see real client IPs behind Caddy. Requires a noda image newer than 0.0.4 — bump `NODA_VERSION` when the next release is published.
```

- [ ] **Step 6: Validate both env flavors against the real config**

```bash
cd /Users/marten/GolandProjects/noda/.worktrees/server-edge-correctness
export DATABASE_URL=postgres://noda:noda@localhost:5432/noda?sslmode=disable \
  FILES_PATH=/tmp/hb-files SETUP_TOKEN=x PUBLIC_BASE_URL=http://localhost:3000 \
  LIVEKIT_URL=wss://x.livekit.cloud LIVEKIT_API_KEY=x LIVEKIT_API_SECRET=x \
  BODY_LIMIT=1073741824
go run ./cmd/noda validate --config projects/homebase
NODA_ENV=production go run ./cmd/noda validate --config projects/homebase
```

Expected: both runs report the config valid (the production run additionally loads the overlay; a trust_proxy mistake would surface as a `/server/trust_proxy` validation error). Then negative check — `BODY_LIMIT=10GB go run ./cmd/noda validate --config projects/homebase` must FAIL with a `/server/body_limit` error.

- [ ] **Step 7: Commit**

```bash
git add projects/homebase/noda.json projects/homebase/noda.production.json projects/homebase/docker-compose.yml projects/homebase/.env.example projects/homebase/README.md
git commit -m "feat(homebase): BODY_LIMIT from env + trust_proxy via production overlay (#300, #301)"
```

---

### Task 7: Whole-branch verification

- [ ] **Step 1: Full test suite + lint**

```bash
go build ./... && go vet ./... && gofmt -l .
go test ./...
golangci-lint run 2>/dev/null || true   # run if installed locally; CI runs it regardless
```

Expected: build clean, `gofmt -l` prints nothing, all tests pass.

- [ ] **Step 2: Whole-branch code review** (per convention: requesting-code-review skill / reviewer subagent over the full branch diff)

- [ ] **Step 3: PR**

```bash
git push -u origin feat/server-edge-correctness
gh pr create --title "feat(server): trusted-proxy support + env-settable server scalars" \
  --body "$(cat <<'EOF'
Implements the server-edge-correctness tranche (spec + plan on branch under docs/superpowers/).

- server.trust_proxy: Fiber TrustProxy/TrustProxyConfig/ProxyHeader exposed via config, off by default, fail-fast validation (empty trust set, bad CIDR/IP). Behind a proxy, rate limiting and session IPs now see real client IPs.
- Numeric server.* settings (port, body_limit, expression_memory_budget) accept {{ $env('NAME') }}; ALL server scalar readers routed through shared coercion helpers.
- Behavior change: invalid server.* values fail validation/startup loudly instead of silently using defaults.
- Homebase: BODY_LIMIT from env, trust_proxy via noda.production.json overlay (NODA_ENV=production), README caveat resolved.

Closes #300
Closes #301

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Wait for the 4 required functional CI checks before merging.
