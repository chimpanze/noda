# Auth Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** First-party authentication for Noda — `plugins/auth` (8 primitive nodes + service), `auth.session` middleware, and a `noda auth init` scaffold that generates project-owned auth flows.

**Architecture:** Security-critical primitives (argon2id hashing, token minting/consumption, session validation) live in `plugins/auth` behind the standard `api.Plugin` contract; a new `auth.session` middleware in `internal/server` validates opaque session tokens via a `pkg/api.SessionAuthenticator` interface; `noda auth init` scaffolds migrations, routes, workflows, and tests into the user's project. Spec: `docs/superpowers/specs/2026-07-04-auth-plugin-design.md`.

**Tech Stack:** Go, gofiber/fiber/v3, gorm (map-based, no structs for user data), `golang.org/x/crypto/argon2` + `bcrypt`, `github.com/google/uuid`, spf13/cobra, embedded `text/template` with `[[ ]]` delimiters.

## Global Constraints

- Work in a worktree: `git worktree add .worktrees/auth-plugin -b feat/auth-plugin main` (project convention).
- Every commit must pass `gofmt -l ./... | grep -v '^$'` (empty output) and `go vet ./...` — CI runs golangci-lint and fails on formatting alone.
- Commit messages: conventional commits (`feat(auth): …`, `test(auth): …`, `docs: …`), ending with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Package test coverage target ≥75% for `plugins/auth`.
- Node domain failures use dedicated outputs (`exists`, `invalid`, `not_found`) — `error` output is for infrastructure failures only.
- Raw tokens and passwords must never be logged or stored; only SHA-256 hashes of tokens persist. (Existing trace redaction already masks keys containing `password`/`token` — do not weaken it.)
- All timestamps `time.Now().UTC()`.
- Test against real config files in `testdata/`, not hard-coded structures, for integration tests.
- At the end (Task 9), `git add -f docs/superpowers/specs/2026-07-04-auth-plugin-design.md docs/superpowers/plans/2026-07-04-auth-plugin.md` onto the branch as point-in-time records (dir is gitignored; this is the established convention).

---

### Task 1: Plugin skeleton, service config, crypto core

**Files:**
- Create: `plugins/auth/plugin.go`
- Create: `plugins/auth/service.go`
- Create: `plugins/auth/crypto.go`
- Test: `plugins/auth/service_test.go`, `plugins/auth/crypto_test.go`
- Modify: `cmd/noda/main.go:784` (`registerCorePlugins` — add auth plugin; find the function and append alongside the other plugin registrations)

**Interfaces:**
- Consumes: `api.Plugin`, `api.NodeRegistration` from `pkg/api`.
- Produces (later tasks rely on these exact names):
  - `type Service struct { DatabaseName string; SessionTTL time.Duration; Cookie CookieConfig; Argon ArgonParams; TokenTTLs map[string]time.Duration }`
  - `type CookieConfig struct { Name, Path, Domain, SameSite string; Secure, HTTPOnly bool }`
  - `type ArgonParams struct { MemoryKiB, Iterations, SaltLen, KeyLen uint32; Parallelism uint8 }`
  - `func (s *Service) HashPassword(pw string) (string, error)`
  - `func VerifyPassword(pw, encoded string) (ok bool, needsRehash bool, err error)`
  - `func MintToken() (raw string, hash string, err error)`
  - `func HashToken(raw string) string`
  - `var DummyVerify func(pw string)` — no; instead: `func VerifyDummy(pw string)` (burns argon2 time for unknown users)
  - `func (s *Service) TokenTTL(purpose string) time.Duration`
  - `const PurposeVerifyEmail = "verify_email"`, `const PurposeResetPassword = "reset_password"`
  - Plugin registered under `Name() = "auth"`, `Prefix() = "auth"`.

- [ ] **Step 1: Write failing tests for crypto + service config**

`plugins/auth/crypto_test.go`:

```go
package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	s := &Service{Argon: DefaultArgonParams()}
	hash, err := s.HashPassword("hunter2!")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("expected PHC argon2id hash, got %q", hash)
	}
	ok, rehash, err := VerifyPassword("hunter2!", hash)
	if err != nil || !ok || rehash {
		t.Fatalf("want ok=true rehash=false err=nil, got ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	ok, _, err = VerifyPassword("wrong", hash)
	if err != nil || ok {
		t.Fatalf("wrong password must not verify")
	}
}

func TestVerifyPasswordBcryptCompat(t *testing.T) {
	// bcrypt hash of "hunter2!" (cost 10) — generated with golang.org/x/crypto/bcrypt
	ok, rehash, err := VerifyPassword("hunter2!", mustBcrypt(t, "hunter2!"))
	if err != nil || !ok {
		t.Fatalf("bcrypt hash must verify: ok=%v err=%v", ok, err)
	}
	if !rehash {
		t.Fatal("bcrypt hashes must be flagged for rehash")
	}
}

func TestVerifyPasswordMalformedHash(t *testing.T) {
	if _, _, err := VerifyPassword("x", "not-a-hash"); err == nil {
		t.Fatal("malformed hash must error")
	}
}

func TestMintToken(t *testing.T) {
	raw, hash, err := MintToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 40 { // 32 bytes base64url ≈ 43 chars
		t.Fatalf("token too short: %d", len(raw))
	}
	if HashToken(raw) != hash {
		t.Fatal("HashToken(raw) must equal minted hash")
	}
	raw2, _, _ := MintToken()
	if raw == raw2 {
		t.Fatal("tokens must be unique")
	}
}
```

Add helper in the test file:

```go
import "golang.org/x/crypto/bcrypt"

func mustBcrypt(t *testing.T, pw string) string {
	t.Helper()
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
```

`plugins/auth/service_test.go`:

```go
package auth

import (
	"testing"
	"time"
)

func TestNewServiceDefaults(t *testing.T) {
	svc, err := newService(map[string]any{"database": "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	if svc.DatabaseName != "postgres" {
		t.Fatalf("DatabaseName = %q", svc.DatabaseName)
	}
	if svc.SessionTTL != 720*time.Hour {
		t.Fatalf("SessionTTL = %v", svc.SessionTTL)
	}
	if svc.Cookie.Name != "noda_session" || !svc.Cookie.Secure || !svc.Cookie.HTTPOnly || svc.Cookie.SameSite != "Lax" || svc.Cookie.Path != "/" {
		t.Fatalf("cookie defaults wrong: %+v", svc.Cookie)
	}
	if svc.TokenTTL(PurposeVerifyEmail) != 24*time.Hour || svc.TokenTTL(PurposeResetPassword) != time.Hour {
		t.Fatal("token TTL defaults wrong")
	}
	if svc.Argon.MemoryKiB != 65536 || svc.Argon.Iterations != 3 || svc.Argon.Parallelism != 2 {
		t.Fatalf("argon defaults wrong: %+v", svc.Argon)
	}
}

func TestNewServiceValidation(t *testing.T) {
	if _, err := newService(map[string]any{}); err == nil {
		t.Fatal("missing database must error")
	}
	if _, err := newService(map[string]any{"database": "db", "session": map[string]any{"ttl": "nope"}}); err == nil {
		t.Fatal("bad ttl must error")
	}
}

func TestPluginContract(t *testing.T) {
	p := &Plugin{}
	if p.Name() != "auth" || p.Prefix() != "auth" || !p.HasServices() {
		t.Fatal("plugin identity wrong")
	}
	svc, err := p.CreateService(map[string]any{"database": "db"})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.HealthCheck(svc); err != nil {
		t.Fatal(err)
	}
	if err := p.Shutdown(svc); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run tests, verify failure**

Run: `go test ./plugins/auth/ -run 'TestHash|TestVerify|TestMint|TestNewService|TestPluginContract' -v`
Expected: compile FAIL (package doesn't exist yet).

- [ ] **Step 3: Implement**

`plugins/auth/crypto.go`:

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// ArgonParams configures argon2id hashing. Defaults follow OWASP recommendations.
type ArgonParams struct {
	MemoryKiB   uint32
	Iterations  uint32
	SaltLen     uint32
	KeyLen      uint32
	Parallelism uint8
}

func DefaultArgonParams() ArgonParams {
	return ArgonParams{MemoryKiB: 65536, Iterations: 3, Parallelism: 2, SaltLen: 16, KeyLen: 32}
}

// HashPassword returns a PHC-encoded argon2id hash: $argon2id$v=19$m=...,t=...,p=...$salt$key
func (s *Service) HashPassword(pw string) (string, error) {
	p := s.Argon
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, p.Iterations, p.MemoryKiB, p.Parallelism, p.KeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.MemoryKiB, p.Iterations, p.Parallelism, b64(salt), b64(key)), nil
}

// VerifyPassword checks pw against an argon2id (PHC) or bcrypt hash.
// needsRehash is true when the stored hash is bcrypt (opportunistic upgrade).
func VerifyPassword(pw, encoded string) (ok bool, needsRehash bool, err error) {
	switch {
	case strings.HasPrefix(encoded, "$argon2id$"):
		ok, err = verifyArgon2id(pw, encoded)
		return ok, false, err
	case strings.HasPrefix(encoded, "$2a$"), strings.HasPrefix(encoded, "$2b$"), strings.HasPrefix(encoded, "$2y$"):
		err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(pw))
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, false, nil
		}
		if err != nil {
			return false, false, fmt.Errorf("auth: bcrypt verify: %w", err)
		}
		return true, true, nil
	default:
		return false, false, fmt.Errorf("auth: unrecognized password hash format")
	}
}

func verifyArgon2id(pw, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, key]
	if len(parts) != 6 {
		return false, fmt.Errorf("auth: malformed argon2id hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("auth: malformed argon2id version: %w", err)
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, fmt.Errorf("auth: malformed argon2id params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("auth: malformed argon2id salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("auth: malformed argon2id key: %w", err)
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash is verified against when no user matches, so response timing does
// not reveal account existence. Fixed params (defaults), fixed password.
var dummyHash = func() string {
	s := &Service{Argon: DefaultArgonParams()}
	h, err := s.HashPassword("noda-dummy-password-for-timing")
	if err != nil {
		panic(err)
	}
	return h
}()

// VerifyDummy burns the same time as a real argon2id verification.
func VerifyDummy(pw string) {
	_, _, _ = VerifyPassword(pw, dummyHash)
}

// MintToken returns a 256-bit random token (base64url raw) and its SHA-256 hex hash.
func MintToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: generate token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashToken(raw), nil
}

// HashToken returns the SHA-256 hex digest of a raw token.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
```

`plugins/auth/service.go`:

```go
package auth

import (
	"fmt"
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
}

func (s *Service) TokenTTL(purpose string) time.Duration {
	if ttl, ok := s.TokenTTLs[purpose]; ok {
		return ttl
	}
	return time.Hour
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
```

`plugins/auth/plugin.go` (node registrations grow in later tasks):

```go
package auth

import (
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

// Plugin implements first-party authentication primitives.
type Plugin struct{}

func (p *Plugin) Name() string      { return "auth" }
func (p *Plugin) Prefix() string    { return "auth" }
func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{}
}

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	return newService(config)
}

func (p *Plugin) HealthCheck(service any) error {
	if _, ok := service.(*Service); !ok {
		return fmt.Errorf("auth: invalid service type")
	}
	return nil
}

func (p *Plugin) Shutdown(any) error { return nil }
```

In `cmd/noda/main.go` `registerCorePlugins` (line ~784), register `&auth.Plugin{}` the same way the other plugins are registered (add the `plugins/auth` import; match the exact registration call pattern used for `&db.Plugin{}` in that function).

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./plugins/auth/ -v && go build ./cmd/noda`
Expected: all PASS; build OK. Also run `go run ./cmd/noda plugin list` and confirm `auth` appears with 0 nodes.

- [ ] **Step 5: Commit**

```bash
gofmt -l ./plugins/auth ./cmd/noda && go vet ./plugins/auth/... 
git add plugins/auth cmd/noda/main.go
git commit -m "feat(auth): plugin skeleton, service config, argon2id/token crypto core"
```

---

### Task 2: Test DB harness + `auth.create_user` + `auth.get_user`

**Files:**
- Create: `plugins/auth/schema_test.go` (shared test harness)
- Create: `plugins/auth/helpers.go` (shared node helpers)
- Create: `plugins/auth/create_user.go`, `plugins/auth/get_user.go`
- Test: `plugins/auth/create_user_test.go`, `plugins/auth/get_user_test.go`
- Modify: `plugins/auth/plugin.go` (register the two nodes)

**Interfaces:**
- Consumes: Task 1 crypto/service; `internal/plugin.ResolveString/ResolveOptionalString/ResolveOptionalArray/ResolveOptionalMap/GetService`; `api.ExecutionContext`.
- Produces:
  - Test harness: `func newTestDB(t *testing.T) *gorm.DB` (in-memory SQLite with all three auth tables), `func testServices(db *gorm.DB) map[string]any` (returns `{"auth": testService(), "database": db}`), `func testService() *Service` (fast argon params for tests), `type fakeCtx struct{}` implementing `api.ExecutionContext` with pass-through `Resolve` (returns the expression string unchanged; node configs in unit tests use literal values).
  - Helpers: `func normalizeEmail(s string) string`, `func isUniqueViolation(err error) bool`, `func parseRoles(v any) []string`, `func parseJSONMap(v any) map[string]any`, `func userView(row map[string]any) map[string]any` (strips `password_hash`, decodes `roles`/`metadata`).
  - Nodes `auth.create_user` (outputs `success`, `exists`, `error`) and `auth.get_user` (outputs `success`, `not_found`, `error`); `success` data is the user map with keys `id, email, email_verified_at, status, roles ([]string), metadata (map), created_at, updated_at`.

- [ ] **Step 1: Write the shared harness**

`plugins/auth/schema_test.go`:

```go
package auth

import (
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testSchema = `
CREATE TABLE auth_users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  email_verified_at TIMESTAMP,
  status TEXT NOT NULL DEFAULT 'active',
  roles TEXT NOT NULL DEFAULT '["user"]',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);
CREATE TABLE auth_sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TIMESTAMP NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  last_used_at TIMESTAMP,
  ip TEXT,
  user_agent TEXT,
  revoked_at TIMESTAMP
);
CREATE TABLE auth_tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
  purpose TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMP NOT NULL,
  consumed_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL
);
`

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_pragma=foreign_keys(1)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.Exec(testSchema).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func testService() *Service {
	svc, err := newService(map[string]any{"database": "test-db"})
	if err != nil {
		panic(err)
	}
	// fast argon params for tests
	svc.Argon = ArgonParams{MemoryKiB: 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}
	return svc
}

func testServices(db *gorm.DB) map[string]any {
	return map[string]any{"auth": testService(), "database": db}
}

// fakeCtx implements api.ExecutionContext for unit tests. Resolve is identity:
// tests pass literal config values, not expressions.
type fakeCtx struct{}

func (fakeCtx) Input() any             { return nil }
func (fakeCtx) Auth() *api.AuthData    { return nil }
func (fakeCtx) Trigger() api.TriggerData {
	return api.TriggerData{}
}
func (fakeCtx) Resolve(expr string) (any, error) { return expr, nil }
func (fakeCtx) ResolveWithVars(expr string, _ map[string]any) (any, error) {
	return expr, nil
}
func (fakeCtx) Log(string, string, map[string]any) {}
```

Note: check `pkg/api/context.go` — if `ExecutionContext` gains methods this doesn't compile against, mirror what other plugin tests (e.g. `plugins/db/helpers_test.go`) use for a fake context; if a shared fake already exists there, copy its pattern rather than inventing a new one.

- [ ] **Step 2: Write failing node tests**

`plugins/auth/create_user_test.go`:

```go
package auth

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
)

func TestCreateUser(t *testing.T) {
	db := newTestDB(t)
	exec := newCreateUserExecutor(nil)
	out, data, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "  Alice@Example.COM ", "password": "password123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	user := data.(map[string]any)
	if user["email"] != "alice@example.com" {
		t.Fatalf("email not normalized: %v", user["email"])
	}
	if _, exists := user["password_hash"]; exists {
		t.Fatal("password_hash must be stripped from output")
	}
	roles, _ := user["roles"].([]string)
	if len(roles) != 1 || roles[0] != "user" {
		t.Fatalf("default roles wrong: %v", user["roles"])
	}

	// duplicate → exists
	out, _, err = exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "password123",
	}, testServices(db))
	if err != nil || out != "exists" {
		t.Fatalf("duplicate: out=%q err=%v", out, err)
	}
}

func TestCreateUserPasswordRules(t *testing.T) {
	db := newTestDB(t)
	exec := newCreateUserExecutor(nil)
	_, _, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "bob@example.com", "password": "short",
	}, testServices(db))
	if err == nil {
		t.Fatal("password < 8 chars must error")
	}
}
```

`plugins/auth/get_user_test.go`:

```go
package auth

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
)

func TestGetUserByEmailAndID(t *testing.T) {
	db := newTestDB(t)
	create := newCreateUserExecutor(nil)
	_, data, err := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "password123",
	}, testServices(db))
	if err != nil {
		t.Fatal(err)
	}
	id := data.(map[string]any)["id"].(string)

	get := newGetUserExecutor(nil)
	out, got, err := get.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": id}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("by id: out=%q err=%v", out, err)
	}
	if got.(map[string]any)["email"] != "alice@example.com" {
		t.Fatal("wrong user")
	}
	if _, exists := got.(map[string]any)["password_hash"]; exists {
		t.Fatal("password_hash must be stripped")
	}

	out, _, err = get.Execute(context.Background(), fakeCtx{}, map[string]any{"email": "ALICE@example.com"}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("by email: out=%q err=%v", out, err)
	}

	out, _, err = get.Execute(context.Background(), fakeCtx{}, map[string]any{"email": "nobody@example.com"}, testServices(db))
	if err != nil || out != "not_found" {
		t.Fatalf("missing: out=%q err=%v", out, err)
	}

	if _, _, err := get.Execute(context.Background(), fakeCtx{}, map[string]any{}, testServices(db)); err == nil {
		t.Fatal("neither user_id nor email must error")
	}
}
```

- [ ] **Step 3: Run tests, verify failure**

Run: `go test ./plugins/auth/ -run 'TestCreateUser|TestGetUser' -v`
Expected: compile FAIL (`newCreateUserExecutor` undefined).

- [ ] **Step 4: Implement helpers + nodes**

`plugins/auth/helpers.go`:

```go
package auth

import (
	"encoding/json"
	"strings"
)

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// isUniqueViolation matches unique-constraint errors across sqlite and postgres.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || // sqlite
		strings.Contains(msg, "duplicate key value violates unique constraint") // postgres
}

func parseRoles(v any) []string {
	var raw string
	switch t := v.(type) {
	case string:
		raw = t
	case []byte:
		raw = string(t)
	default:
		return []string{}
	}
	var roles []string
	if err := json.Unmarshal([]byte(raw), &roles); err != nil {
		return []string{}
	}
	return roles
}

func parseJSONMap(v any) map[string]any {
	var raw string
	switch t := v.(type) {
	case string:
		raw = t
	case []byte:
		raw = string(t)
	default:
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// userView returns a copy of a raw auth_users row safe for workflow output:
// password_hash removed, roles/metadata decoded.
func userView(row map[string]any) map[string]any {
	out := make(map[string]any, len(row))
	for k, v := range row {
		if k == "password_hash" {
			continue
		}
		out[k] = v
	}
	out["roles"] = parseRoles(row["roles"])
	out["metadata"] = parseJSONMap(row["metadata"])
	return out
}

func validatePassword(pw string) error {
	if len(pw) < 8 {
		return errPasswordTooShort
	}
	if len(pw) > 512 {
		return errPasswordTooLong
	}
	return nil
}
```

Add the two sentinel errors at the top of `helpers.go`:

```go
import "errors"

var (
	errPasswordTooShort = errors.New("auth: password must be at least 8 characters")
	errPasswordTooLong  = errors.New("auth: password must be at most 512 characters")
)
```

`plugins/auth/create_user.go`:

```go
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
```

`plugins/auth/get_user.go`:

```go
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

func (d *getUserDescriptor) Name() string        { return "get_user" }
func (d *getUserDescriptor) Description() string { return "Fetches a user by id or email (password hash stripped)" }
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
```

Register both in `plugin.go`:

```go
func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &createUserDescriptor{}, Factory: newCreateUserExecutor},
		{Descriptor: &getUserDescriptor{}, Factory: newGetUserExecutor},
	}
}
```

- [ ] **Step 5: Run tests, verify pass**

Run: `go test ./plugins/auth/ -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -l ./plugins/auth && go vet ./plugins/auth/...
git add plugins/auth
git commit -m "feat(auth): create_user and get_user nodes with sqlite test harness"
```

---

### Task 3: `auth.verify_credentials`

**Files:**
- Create: `plugins/auth/verify_credentials.go`
- Test: `plugins/auth/verify_credentials_test.go`
- Modify: `plugins/auth/plugin.go` (register node)

**Interfaces:**
- Consumes: Task 1 `VerifyPassword`/`VerifyDummy`/`HashPassword`, Task 2 harness/helpers.
- Produces: node `auth.verify_credentials`, outputs `success` (user view map), `invalid`, `error`. On successful bcrypt verify, rewrites `password_hash` to argon2id.

- [ ] **Step 1: Write failing tests**

`plugins/auth/verify_credentials_test.go`:

```go
package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedUser(t *testing.T, db *gorm.DB, email, passwordHash, status string) string {
	t.Helper()
	id := uuid.NewString()
	now := time.Now().UTC()
	err := db.Table("auth_users").Create(map[string]any{
		"id": id, "email": email, "password_hash": passwordHash,
		"status": status, "roles": `["user"]`, "metadata": `{}`,
		"created_at": now, "updated_at": now,
	}).Error
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestVerifyCredentials(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	seedUser(t, db, "alice@example.com", hash, "active")
	exec := newVerifyCredentialsExecutor(nil)

	out, data, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "ALICE@example.com", "password": "password123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if _, exists := data.(map[string]any)["password_hash"]; exists {
		t.Fatal("password_hash must be stripped")
	}

	for name, cfg := range map[string]map[string]any{
		"wrong password": {"email": "alice@example.com", "password": "nope-nope"},
		"unknown user":   {"email": "ghost@example.com", "password": "password123"},
	} {
		out, _, err := exec.Execute(context.Background(), fakeCtx{}, cfg, testServices(db))
		if err != nil || out != "invalid" {
			t.Fatalf("%s: out=%q err=%v", name, out, err)
		}
	}
}

func TestVerifyCredentialsDisabledUser(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	seedUser(t, db, "off@example.com", hash, "disabled")
	exec := newVerifyCredentialsExecutor(nil)
	out, _, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "off@example.com", "password": "password123",
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("disabled user must be invalid: out=%q err=%v", out, err)
	}
}

func TestVerifyCredentialsBcryptUpgrade(t *testing.T) {
	db := newTestDB(t)
	id := seedUser(t, db, "old@example.com", mustBcrypt(t, "password123"), "active")
	exec := newVerifyCredentialsExecutor(nil)
	out, _, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "old@example.com", "password": "password123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	var newHash string
	db.Table("auth_users").Where("id = ?", id).Pluck("password_hash", &newHash)
	if !strings.HasPrefix(newHash, "$argon2id$") {
		t.Fatalf("hash must be upgraded to argon2id, got %q", newHash[:10])
	}
}
```

- [ ] **Step 2: Run tests, verify failure**

Run: `go test ./plugins/auth/ -run TestVerifyCredentials -v`
Expected: compile FAIL (`newVerifyCredentialsExecutor` undefined).

- [ ] **Step 3: Implement**

`plugins/auth/verify_credentials.go`:

```go
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

func (e *verifyCredentialsExecutor) Outputs() []string { return []string{"success", "invalid", "error"} }

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
		return "", nil, fmt.Errorf("auth.verify_credentials: %w", err)
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
```

Register in `plugin.go`: append `{Descriptor: &verifyCredentialsDescriptor{}, Factory: newVerifyCredentialsExecutor},`.

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./plugins/auth/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l ./plugins/auth && go vet ./plugins/auth/...
git add plugins/auth
git commit -m "feat(auth): verify_credentials node with dummy-hash timing safety and bcrypt upgrade"
```

---

### Task 4: `auth.create_session` + `auth.revoke_session`

**Files:**
- Create: `plugins/auth/create_session.go`, `plugins/auth/revoke_session.go`
- Test: `plugins/auth/session_nodes_test.go`
- Modify: `plugins/auth/plugin.go` (register nodes), `plugins/auth/service.go` (add cookie-object builders)

**Interfaces:**
- Consumes: Tasks 1–2.
- Produces:
  - `func (s *Service) SessionCookieObject(rawToken string, ttl time.Duration) map[string]any` and `func (s *Service) ClearCookieObject() map[string]any` — keys exactly matching `response.json` cookie parsing (`plugins/core/response/json.go:144-166`): `name, value, path, domain, max_age, secure, http_only, same_site` (numbers as `float64`).
  - Node `auth.create_session`: outputs `success` (`{token, session_id, expires_at, cookie}`), `error`.
  - Node `auth.revoke_session`: outputs `success` (`{revoked_count, clear_cookie}`), `error`. Config: exactly one of `token`, `session_id`, `user_id`.

- [ ] **Step 1: Write failing tests**

`plugins/auth/session_nodes_test.go`:

```go
package auth

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
)

func TestCreateSession(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")

	exec := newCreateSessionExecutor(nil)
	out, data, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	res := data.(map[string]any)
	token, _ := res["token"].(string)
	if token == "" {
		t.Fatal("missing token")
	}
	cookie, _ := res["cookie"].(map[string]any)
	if cookie["name"] != "noda_session" || cookie["value"] != token || cookie["http_only"] != true {
		t.Fatalf("cookie object wrong: %v", cookie)
	}
	if _, ok := cookie["max_age"].(float64); !ok {
		t.Fatalf("max_age must be float64 for response.json, got %T", cookie["max_age"])
	}

	// raw token must not be stored
	var count int64
	db.Table("auth_sessions").Where("token_hash = ?", token).Count(&count)
	if count != 0 {
		t.Fatal("raw token stored in token_hash column")
	}
	db.Table("auth_sessions").Where("token_hash = ?", HashToken(token)).Count(&count)
	if count != 1 {
		t.Fatal("hashed token not stored")
	}
}

func TestRevokeSession(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")

	create := newCreateSessionExecutor(nil)
	_, d1, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	_, d2, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	token1 := d1.(map[string]any)["token"].(string)
	_ = d2

	revoke := newRevokeSessionExecutor(nil)
	out, data, err := revoke.Execute(context.Background(), fakeCtx{}, map[string]any{"token": token1}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if data.(map[string]any)["revoked_count"].(int64) != 1 {
		t.Fatalf("revoked_count = %v", data.(map[string]any)["revoked_count"])
	}
	cc := data.(map[string]any)["clear_cookie"].(map[string]any)
	if cc["value"] != "" || cc["max_age"].(float64) != -1 {
		t.Fatalf("clear_cookie wrong: %v", cc)
	}

	// idempotent
	out, data, err = revoke.Execute(context.Background(), fakeCtx{}, map[string]any{"token": token1}, testServices(db))
	if err != nil || out != api.OutputSuccess || data.(map[string]any)["revoked_count"].(int64) != 0 {
		t.Fatalf("re-revoke must be idempotent success: out=%q err=%v", out, err)
	}

	// revoke all for user (one remains active)
	out, data, err = revoke.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	if err != nil || out != api.OutputSuccess || data.(map[string]any)["revoked_count"].(int64) != 1 {
		t.Fatalf("revoke-all: out=%q data=%v err=%v", out, data, err)
	}

	// exactly-one-selector validation
	if _, _, err := revoke.Execute(context.Background(), fakeCtx{}, map[string]any{}, testServices(db)); err == nil {
		t.Fatal("no selector must error")
	}
	if _, _, err := revoke.Execute(context.Background(), fakeCtx{}, map[string]any{"token": "x", "user_id": "y"}, testServices(db)); err == nil {
		t.Fatal("two selectors must error")
	}
}
```

- [ ] **Step 2: Run tests, verify failure**

Run: `go test ./plugins/auth/ -run 'TestCreateSession|TestRevokeSession' -v`
Expected: compile FAIL.

- [ ] **Step 3: Implement**

Add to `plugins/auth/service.go`:

```go
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
```

`plugins/auth/create_session.go`:

```go
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
	if err := db.WithContext(ctx).Table("auth_sessions").Create(row).Error; err != nil {
		return "", nil, fmt.Errorf("auth.create_session: %w", err)
	}
	return api.OutputSuccess, map[string]any{
		"token":      raw,
		"session_id": sessionID,
		"expires_at": expiresAt,
		"cookie":     svc.SessionCookieObject(raw, ttl),
	}, nil
}
```

`plugins/auth/revoke_session.go`:

```go
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type revokeSessionDescriptor struct{}

func (d *revokeSessionDescriptor) Name() string { return "revoke_session" }
func (d *revokeSessionDescriptor) Description() string {
	return "Revokes one session (by token or id) or all sessions for a user"
}
func (d *revokeSessionDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"auth":     {Prefix: "auth", Required: true},
		"database": {Prefix: "db", Required: true},
	}
}
func (d *revokeSessionDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"token":      map[string]any{"type": "string", "description": "Raw session token to revoke (expression)"},
			"session_id": map[string]any{"type": "string", "description": "Session id to revoke (expression)"},
			"user_id":    map[string]any{"type": "string", "description": "Revoke ALL sessions for this user (expression)"},
		},
	}
}
func (d *revokeSessionDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "{revoked_count, clear_cookie} — pass clear_cookie to response.json cookies",
		"error":   "Infrastructure error",
	}
}

type revokeSessionExecutor struct{}

func newRevokeSessionExecutor(_ map[string]any) api.NodeExecutor { return &revokeSessionExecutor{} }

func (e *revokeSessionExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *revokeSessionExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "auth")
	if err != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", err)
	}
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", err)
	}
	token, _, err := plugin.ResolveOptionalString(nCtx, config, "token")
	if err != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", err)
	}
	sessionID, _, err := plugin.ResolveOptionalString(nCtx, config, "session_id")
	if err != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", err)
	}
	userID, _, err := plugin.ResolveOptionalString(nCtx, config, "user_id")
	if err != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", err)
	}

	set := 0
	for _, v := range []string{token, sessionID, userID} {
		if v != "" {
			set++
		}
	}
	if set != 1 {
		return "", nil, fmt.Errorf("auth.revoke_session: exactly one of 'token', 'session_id', 'user_id' is required")
	}

	q := db.WithContext(ctx).Table("auth_sessions").Where("revoked_at IS NULL")
	switch {
	case token != "":
		q = q.Where("token_hash = ?", HashToken(token))
	case sessionID != "":
		q = q.Where("id = ?", sessionID)
	default:
		q = q.Where("user_id = ?", userID)
	}
	res := q.Update("revoked_at", time.Now().UTC())
	if res.Error != nil {
		return "", nil, fmt.Errorf("auth.revoke_session: %w", res.Error)
	}
	return api.OutputSuccess, map[string]any{
		"revoked_count": res.RowsAffected,
		"clear_cookie":  svc.ClearCookieObject(),
	}, nil
}
```

Register both in `plugin.go`.

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./plugins/auth/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l ./plugins/auth && go vet ./plugins/auth/...
git add plugins/auth
git commit -m "feat(auth): create_session and revoke_session nodes with response.json cookie objects"
```

---

### Task 5: `auth.create_token` + `auth.consume_token` + `auth.set_password`

**Files:**
- Create: `plugins/auth/one_time_tokens.go` (both token nodes), `plugins/auth/set_password.go`
- Test: `plugins/auth/one_time_tokens_test.go`, `plugins/auth/set_password_test.go`
- Modify: `plugins/auth/plugin.go` (register nodes)

**Interfaces:**
- Consumes: Tasks 1–4.
- Produces:
  - `auth.create_token`: config `user_id`, `purpose` (`verify_email`|`reset_password`), optional `ttl`; outputs `success` (`{token, expires_at}`), `error`. Creating a token invalidates prior unconsumed tokens for the same user+purpose.
  - `auth.consume_token`: config `token`, `purpose`; outputs `success` (`{user_id}`), `invalid`, `error`. Atomic single-use. `verify_email` consumption sets `auth_users.email_verified_at`.
  - `auth.set_password`: config `user_id`, `password`, optional `revoke_sessions` (default true); outputs `success` (`{revoked_sessions}`), `error`.

- [ ] **Step 1: Write failing tests**

`plugins/auth/one_time_tokens_test.go`:

```go
package auth

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
)

func TestCreateAndConsumeToken(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")

	create := newCreateTokenExecutor(nil)
	out, data, err := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": PurposeVerifyEmail,
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("create: out=%q err=%v", out, err)
	}
	token := data.(map[string]any)["token"].(string)

	consume := newConsumeTokenExecutor(nil)

	// wrong purpose → invalid
	out, _, err = consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "purpose": PurposeResetPassword,
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("wrong purpose: out=%q err=%v", out, err)
	}

	// correct → success with user_id, and email_verified_at set
	out, data, err = consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "purpose": PurposeVerifyEmail,
	}, testServices(db))
	if err != nil || out != api.OutputSuccess || data.(map[string]any)["user_id"] != userID {
		t.Fatalf("consume: out=%q data=%v err=%v", out, data, err)
	}
	var verified *time.Time
	db.Table("auth_users").Where("id = ?", userID).Pluck("email_verified_at", &verified)
	if verified == nil {
		t.Fatal("email_verified_at not set")
	}

	// second consume → invalid
	out, _, err = consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "purpose": PurposeVerifyEmail,
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("reuse: out=%q err=%v", out, err)
	}
}

func TestCreateTokenInvalidatesPrior(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")
	create := newCreateTokenExecutor(nil)
	consume := newConsumeTokenExecutor(nil)

	_, d1, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID, "purpose": PurposeResetPassword}, testServices(db))
	_, _, _ = create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID, "purpose": PurposeResetPassword}, testServices(db))

	out, _, err := consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": d1.(map[string]any)["token"], "purpose": PurposeResetPassword,
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("older token must be invalidated: out=%q err=%v", out, err)
	}
}

func TestConsumeExpiredToken(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")
	create := newCreateTokenExecutor(nil)
	_, data, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": PurposeVerifyEmail, "ttl": "1ns",
	}, testServices(db))
	time.Sleep(10 * time.Millisecond)
	consume := newConsumeTokenExecutor(nil)
	out, _, err := consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": data.(map[string]any)["token"], "purpose": PurposeVerifyEmail,
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("expired: out=%q err=%v", out, err)
	}
}

// Polarity check: this test MUST fail if the consumed_at guard is removed from
// the UPDATE's WHERE clause (i.e. if consumption stops being atomic).
func TestConsumeTokenConcurrentSingleUse(t *testing.T) {
	db := newTestDB(t) // MaxOpenConns(1) serializes; the guard does the correctness work
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")
	create := newCreateTokenExecutor(nil)
	_, data, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": PurposeVerifyEmail,
	}, testServices(db))
	token := data.(map[string]any)["token"].(string)

	consume := newConsumeTokenExecutor(nil)
	const n = 8
	var wg sync.WaitGroup
	successes := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, _, err := consume.Execute(context.Background(), fakeCtx{}, map[string]any{
				"token": token, "purpose": PurposeVerifyEmail,
			}, testServices(db))
			if err == nil && out == api.OutputSuccess {
				successes <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(successes)
	count := 0
	for range successes {
		count++
	}
	if count != 1 {
		t.Fatalf("token consumed %d times; must be exactly 1", count)
	}
}
```

`plugins/auth/set_password_test.go`:

```go
package auth

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
)

func TestSetPassword(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	oldHash, _ := svc.HashPassword("oldpassword")
	userID := seedUser(t, db, "alice@example.com", oldHash, "active")

	create := newCreateSessionExecutor(nil)
	_, _, _ = create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))

	set := newSetPasswordExecutor(nil)
	out, data, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "password": "newpassword123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if data.(map[string]any)["revoked_sessions"].(int64) != 1 {
		t.Fatal("existing sessions must be revoked by default")
	}

	verify := newVerifyCredentialsExecutor(nil)
	out, _, _ = verify.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "newpassword123",
	}, testServices(db))
	if out != api.OutputSuccess {
		t.Fatal("new password must verify")
	}
	out, _, _ = verify.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "oldpassword",
	}, testServices(db))
	if out != "invalid" {
		t.Fatal("old password must no longer verify")
	}
}

func TestSetPasswordUnknownUser(t *testing.T) {
	db := newTestDB(t)
	set := newSetPasswordExecutor(nil)
	if _, _, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": "nope", "password": "newpassword123",
	}, testServices(db)); err == nil {
		t.Fatal("unknown user must error")
	}
}
```

- [ ] **Step 2: Run tests, verify failure**

Run: `go test ./plugins/auth/ -run 'TestCreateAndConsume|TestCreateTokenInvalidates|TestConsume|TestSetPassword' -v`
Expected: compile FAIL.

- [ ] **Step 3: Implement**

`plugins/auth/one_time_tokens.go`:

```go
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
	// Atomic single-use: the WHERE guard on consumed_at makes concurrent
	// consumption impossible — exactly one UPDATE can match.
	res := db.WithContext(ctx).Table("auth_tokens").
		Where("token_hash = ? AND purpose = ? AND consumed_at IS NULL AND expires_at > ?", hash, purpose, now).
		Update("consumed_at", now)
	if res.Error != nil {
		return "", nil, fmt.Errorf("auth.consume_token: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return "invalid", map[string]any{}, nil
	}

	var userID string
	if err := db.WithContext(ctx).Table("auth_tokens").
		Where("token_hash = ?", hash).Pluck("user_id", &userID).Error; err != nil {
		return "", nil, fmt.Errorf("auth.consume_token: %w", err)
	}
	if userID == "" {
		return "", nil, fmt.Errorf("auth.consume_token: consumed token row disappeared")
	}

	if purpose == PurposeVerifyEmail {
		if err := db.WithContext(ctx).Table("auth_users").Where("id = ?", userID).
			Updates(map[string]any{"email_verified_at": now, "updated_at": now}).Error; err != nil {
			return "", nil, fmt.Errorf("auth.consume_token: mark verified: %w", err)
		}
	}
	return api.OutputSuccess, map[string]any{"user_id": userID}, nil
}
```

`plugins/auth/set_password.go`:

```go
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
```

Register all three in `plugin.go` — final registration list (8 nodes):

```go
func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &createUserDescriptor{}, Factory: newCreateUserExecutor},
		{Descriptor: &getUserDescriptor{}, Factory: newGetUserExecutor},
		{Descriptor: &verifyCredentialsDescriptor{}, Factory: newVerifyCredentialsExecutor},
		{Descriptor: &createSessionDescriptor{}, Factory: newCreateSessionExecutor},
		{Descriptor: &revokeSessionDescriptor{}, Factory: newRevokeSessionExecutor},
		{Descriptor: &createTokenDescriptor{}, Factory: newCreateTokenExecutor},
		{Descriptor: &consumeTokenDescriptor{}, Factory: newConsumeTokenExecutor},
		{Descriptor: &setPasswordDescriptor{}, Factory: newSetPasswordExecutor},
	}
}
```

- [ ] **Step 4: Run tests (with race detector), verify pass**

Run: `go test ./plugins/auth/ -race -v && go test ./plugins/auth/ -cover`
Expected: all PASS; coverage ≥75%.

- [ ] **Step 5: Verify polarity of the concurrency test**

Temporarily remove `AND consumed_at IS NULL` from the consume UPDATE, run `go test ./plugins/auth/ -race -run TestConsumeTokenConcurrentSingleUse -count=5`; expected: FAIL. Restore the guard, re-run; expected: PASS. (Project convention: race-style tests need polarity-flipped assertions.)

- [ ] **Step 6: Commit**

```bash
gofmt -l ./plugins/auth && go vet ./plugins/auth/...
git add plugins/auth
git commit -m "feat(auth): one-time tokens (atomic single-use) and set_password nodes"
```

---

### Task 6: `auth.session` middleware

**Files:**
- Create: `pkg/api/session.go` (interface)
- Create: `plugins/auth/session_auth.go` (`Service.AuthenticateSession`)
- Create: `internal/server/session_middleware.go`
- Test: `plugins/auth/session_auth_test.go`, `internal/server/session_middleware_test.go`
- Modify: `internal/server/server.go` (add `serverMiddleware` map to `Server`, init in `NewServer`)
- Modify: `internal/server/presets.go:57`, `internal/server/routes.go:30`, `internal/server/connections.go:191` (route through new `s.buildMiddleware`)
- Modify: `internal/server/presets.go:221` (`middlewareOrderRules`)
- Modify: `internal/server/middleware.go:101` (`middlewareConfigPaths`: add `"auth.session": {"security", "session"}`)

**Interfaces:**
- Consumes: `registry.ServiceRegistry.Get(name)`, Task 1 `Service`, `api.AuthData`, locals constants `api.LocalJWTClaims/LocalJWTUserID/LocalJWTRoles` (`pkg/api/constants.go:7-9`).
- Produces:
  - `pkg/api/session.go`:

```go
package api

import "context"

// SessionAuthenticator is implemented by auth services that validate opaque
// session tokens. db is the GORM handle of the service named by
// DatabaseServiceName (typed any to keep pkg/api free of gorm).
// AuthenticateSession returns (nil, nil) when the token is invalid.
type SessionAuthenticator interface {
	AuthenticateSession(ctx context.Context, db any, rawToken string) (*AuthData, error)
	DatabaseServiceName() string
	SessionCookieName() string
}
```

  - `Service` methods: `AuthenticateSession` (claims: `sub`, `email`, `email_verified` (bool), `session_id`, `roles`), `DatabaseServiceName() string`, `SessionCookieName() string`.
  - `func (s *Server) buildMiddleware(name string) (fiber.Handler, error)` — server-scoped factories first, fallback to package `BuildMiddleware`.

- [ ] **Step 1: Write failing test for `AuthenticateSession`**

`plugins/auth/session_auth_test.go`:

```go
package auth

import (
	"context"
	"testing"
	"time"
)

func TestAuthenticateSession(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")

	create := newCreateSessionExecutor(nil)
	_, data, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	token := data.(map[string]any)["token"].(string)

	ad, err := svc.AuthenticateSession(context.Background(), db, token)
	if err != nil || ad == nil {
		t.Fatalf("ad=%v err=%v", ad, err)
	}
	if ad.UserID != userID || ad.Claims["email"] != "alice@example.com" || ad.Claims["email_verified"] != false {
		t.Fatalf("auth data wrong: %+v", ad)
	}
	if len(ad.Roles) != 1 || ad.Roles[0] != "user" {
		t.Fatalf("roles wrong: %v", ad.Roles)
	}

	// invalid token → (nil, nil)
	if ad, err := svc.AuthenticateSession(context.Background(), db, "garbage"); ad != nil || err != nil {
		t.Fatalf("garbage token: ad=%v err=%v", ad, err)
	}

	// revoked → nil
	revoke := newRevokeSessionExecutor(nil)
	revoke.Execute(context.Background(), fakeCtx{}, map[string]any{"token": token}, testServices(db))
	if ad, _ := svc.AuthenticateSession(context.Background(), db, token); ad != nil {
		t.Fatal("revoked session must not authenticate")
	}
}

func TestAuthenticateSessionExpiredAndDisabled(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")

	// expired session
	u1 := seedUser(t, db, "a@example.com", hash, "active")
	create := newCreateSessionExecutor(nil)
	_, d, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": u1, "ttl": "1ns"}, testServices(db))
	time.Sleep(10 * time.Millisecond)
	if ad, _ := svc.AuthenticateSession(context.Background(), db, d.(map[string]any)["token"].(string)); ad != nil {
		t.Fatal("expired session must not authenticate")
	}

	// disabled user
	u2 := seedUser(t, db, "b@example.com", hash, "disabled")
	_, d2, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": u2}, testServices(db))
	if ad, _ := svc.AuthenticateSession(context.Background(), db, d2.(map[string]any)["token"].(string)); ad != nil {
		t.Fatal("disabled user must not authenticate")
	}
}

func TestAuthenticateSessionTouchesLastUsed(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")
	create := newCreateSessionExecutor(nil)
	_, d, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	token := d.(map[string]any)["token"].(string)

	svc.AuthenticateSession(context.Background(), db, token)
	var first *time.Time
	db.Table("auth_sessions").Where("token_hash = ?", HashToken(token)).Pluck("last_used_at", &first)
	if first == nil {
		t.Fatal("last_used_at not set on first use")
	}
	svc.AuthenticateSession(context.Background(), db, token)
	var second *time.Time
	db.Table("auth_sessions").Where("token_hash = ?", HashToken(token)).Pluck("last_used_at", &second)
	if !second.Equal(*first) {
		t.Fatal("last_used_at must be throttled (unchanged within a minute)")
	}
}
```

- [ ] **Step 2: Run, verify failure**

Run: `go test ./plugins/auth/ -run TestAuthenticateSession -v`
Expected: compile FAIL.

- [ ] **Step 3: Implement interface + `AuthenticateSession`**

Create `pkg/api/session.go` with the interface exactly as in the Interfaces block above.

`plugins/auth/session_auth.go`:

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

// lastUsedThrottle limits last_used_at writes to once per interval per session.
const lastUsedThrottle = time.Minute

func (s *Service) DatabaseServiceName() string { return s.DatabaseName }
func (s *Service) SessionCookieName() string   { return s.Cookie.Name }

// AuthenticateSession implements api.SessionAuthenticator.
func (s *Service) AuthenticateSession(ctx context.Context, dbAny any, rawToken string) (*api.AuthData, error) {
	db, ok := dbAny.(*gorm.DB)
	if !ok {
		return nil, fmt.Errorf("auth: AuthenticateSession: expected *gorm.DB, got %T", dbAny)
	}
	if rawToken == "" {
		return nil, nil
	}
	hash := HashToken(rawToken)
	now := time.Now().UTC()

	var row struct {
		SessionID       string
		UserID          string
		Email           string
		EmailVerifiedAt *time.Time
		Roles           string
		LastUsedAt      *time.Time
	}
	err := db.WithContext(ctx).Table("auth_sessions").
		Select("auth_sessions.id AS session_id, auth_sessions.last_used_at AS last_used_at, "+
			"auth_users.id AS user_id, auth_users.email AS email, "+
			"auth_users.email_verified_at AS email_verified_at, auth_users.roles AS roles").
		Joins("JOIN auth_users ON auth_users.id = auth_sessions.user_id").
		Where("auth_sessions.token_hash = ? AND auth_sessions.revoked_at IS NULL AND auth_sessions.expires_at > ? AND auth_users.status = ?",
			hash, now, "active").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: AuthenticateSession: %w", err)
	}

	if row.LastUsedAt == nil || now.Sub(*row.LastUsedAt) > lastUsedThrottle {
		// best-effort; a failed touch must not fail authentication
		db.WithContext(ctx).Table("auth_sessions").
			Where("id = ?", row.SessionID).Update("last_used_at", now)
	}

	roles := parseRoles(row.Roles)
	return &api.AuthData{
		UserID: row.UserID,
		Roles:  roles,
		Claims: map[string]any{
			"sub":            row.UserID,
			"email":          row.Email,
			"email_verified": row.EmailVerifiedAt != nil,
			"session_id":     row.SessionID,
			"roles":          roles,
		},
	}, nil
}
```

Run: `go test ./plugins/auth/ -run TestAuthenticateSession -v` — expected PASS.

- [ ] **Step 4: Write failing middleware test**

`internal/server/session_middleware_test.go` — follow the existing pattern in `internal/server/middleware_test.go` for constructing a test `Server` (reuse its helpers for building a server with a `ServiceRegistry`; read that file first and mirror it). Test body:

```go
package server

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
)

// fakeSessionAuth implements api.SessionAuthenticator without a real DB.
type fakeSessionAuth struct {
	validToken string
}

func (f *fakeSessionAuth) AuthenticateSession(_ context.Context, _ any, tok string) (*api.AuthData, error) {
	if tok == f.validToken {
		return &api.AuthData{
			UserID: "user-1",
			Roles:  []string{"user"},
			Claims: map[string]any{"sub": "user-1", "email": "a@b.c", "session_id": "sess-1", "roles": []string{"user"}},
		}, nil
	}
	return nil, nil
}
func (f *fakeSessionAuth) DatabaseServiceName() string { return "db" }
func (f *fakeSessionAuth) SessionCookieName() string   { return "noda_session" }

func TestSessionMiddleware(t *testing.T) {
	// Build a Server whose ServiceRegistry contains "auth" → &fakeSessionAuth{validToken: "tok123"}
	// and "db" → struct{}{} (never dereferenced by the fake), using the same
	// construction helpers as the other middleware tests in this package.
	s := newTestServerWithServices(t, map[string]any{
		"auth": &fakeSessionAuth{validToken: "tok123"},
		"db":   struct{}{},
	})

	h, err := s.buildMiddleware("auth.session")
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Use(h)
	app.Get("/x", func(c fiber.Ctx) error {
		if c.Locals(api.LocalJWTUserID) != "user-1" {
			t.Error("user id local not set")
		}
		claims, _ := c.Locals(api.LocalJWTClaims).(map[string]any)
		if claims["session_id"] != "sess-1" {
			t.Error("claims local not set")
		}
		return c.SendString("ok")
	})

	// bearer
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer tok123")
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("bearer: status %d", resp.StatusCode)
	}

	// cookie
	req = httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(&http.Cookie{Name: "noda_session", Value: "tok123"})
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("cookie: status %d", resp.StatusCode)
	}

	// invalid / missing → 401
	for _, setup := range []func(*http.Request){
		func(r *http.Request) {},
		func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") },
		func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "noda_session", Value: "wrong"}) },
	} {
		req := httptest.NewRequest("GET", "/x", nil)
		setup(req)
		resp, _ := app.Test(req)
		if resp.StatusCode != 401 {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	}
}

func TestSessionMiddlewareOrdering(t *testing.T) {
	if err := ValidateMiddlewareOrder([]string{"auth.session", "casbin.enforce"}); err != nil {
		t.Fatalf("auth.session before casbin must be valid: %v", err)
	}
	if err := ValidateMiddlewareOrder([]string{"casbin.enforce", "auth.session"}); err == nil {
		t.Fatal("casbin before auth.session must be rejected")
	}
}
```

(Add `"net/http"` import. If no `newTestServerWithServices` helper exists yet, create it in this test file modeled on how `NewServer` is called in `middleware_test.go` / `routes_test.go`.)

Note on ordering semantics: `middlewareOrderRules` prerequisites are OR-style presence checks (each listed prerequisite that *is present* must precede). Adding `auth.session` to casbin's list must not make chains like `["auth.jwt", "casbin.enforce"]` invalid — confirm `TestSessionMiddlewareOrdering` plus the existing ordering tests in `internal/server` still pass unmodified. If any existing test asserts the exact rule list, update it deliberately.

- [ ] **Step 5: Run, verify failure**

Run: `go test ./internal/server/ -run TestSessionMiddleware -v`
Expected: compile FAIL (`buildMiddleware` undefined).

- [ ] **Step 6: Implement middleware + wiring**

`internal/server/session_middleware.go`:

```go
package server

import (
	"log/slog"
	"strings"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
)

// newSessionMiddleware validates opaque session tokens issued by the auth
// plugin. Server-scoped because it needs the ServiceRegistry at request time.
func (s *Server) newSessionMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	serviceName := "auth"
	if v, ok := cfg["service"].(string); ok && v != "" {
		serviceName = v
	}
	return func(c fiber.Ctx) error {
		svcAny, ok := s.services.Get(serviceName)
		if !ok {
			slog.Error("auth.session: service not found", "service", serviceName)
			return fiber.NewError(fiber.StatusInternalServerError, "auth misconfigured")
		}
		authn, ok := svcAny.(api.SessionAuthenticator)
		if !ok {
			slog.Error("auth.session: service does not implement SessionAuthenticator", "service", serviceName)
			return fiber.NewError(fiber.StatusInternalServerError, "auth misconfigured")
		}
		db, ok := s.services.Get(authn.DatabaseServiceName())
		if !ok {
			slog.Error("auth.session: database service not found", "service", authn.DatabaseServiceName())
			return fiber.NewError(fiber.StatusInternalServerError, "auth misconfigured")
		}

		token := c.Cookies(authn.SessionCookieName())
		if token == "" {
			header := c.Get("Authorization")
			if t := strings.TrimPrefix(header, "Bearer "); t != header {
				token = t
			}
		}
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token")
		}

		ad, err := authn.AuthenticateSession(c.Context(), db, token)
		if err != nil {
			slog.Error("auth.session: validation error", "error", err)
			return fiber.NewError(fiber.StatusInternalServerError, "internal error")
		}
		if ad == nil {
			slog.Debug("auth.session: invalid session token")
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token")
		}

		c.Locals(api.LocalJWTClaims, ad.Claims)
		c.Locals(api.LocalJWTUserID, ad.UserID)
		c.Locals(api.LocalJWTRoles, ad.Roles)
		return c.Next()
	}, nil
}
```

In `internal/server/server.go`, add a field to `Server` and populate it in `NewServer` after `s.services` is set:

```go
serverMiddleware map[string]MiddlewareFactory
```

```go
s.serverMiddleware = map[string]MiddlewareFactory{
	"auth.session": s.newSessionMiddleware,
}
```

Add to `internal/server/session_middleware.go` (or `middleware.go`) the dispatch method — same config resolution as `BuildMiddleware` (`internal/server/middleware.go:77-96`):

```go
// buildMiddleware resolves server-scoped middleware first, then falls back to
// the package-level registry.
func (s *Server) buildMiddleware(name string) (fiber.Handler, error) {
	baseType, instance := ParseMiddlewareName(name)
	factory, ok := s.serverMiddleware[baseType]
	if !ok {
		return BuildMiddleware(name, s.config.Root)
	}
	var mwConfig map[string]any
	if instance != "" {
		mwConfig = extractInstanceConfig(name, s.config.Root)
		if mwConfig == nil {
			return nil, fmt.Errorf("middleware instance %q not found in middleware_instances", name)
		}
	} else {
		mwConfig = extractMiddlewareConfig(name, s.config.Root)
	}
	return factory(mwConfig, s.config.Root)
}
```

Replace the three call sites (`presets.go:57`, `routes.go:30`, `connections.go:191`): `BuildMiddleware(name, s.config.Root)` → `s.buildMiddleware(name)`.

Add to `middlewareConfigPaths` (`middleware.go:101`): `"auth.session": {"security", "session"},`.
Change `middlewareOrderRules` (`presets.go:221`): `"casbin.enforce": {"auth.jwt", "auth.oidc", "auth.session"},`.

Also check `internal/server/editor_nodes.go` — if middleware are surfaced to the editor there (search for `auth.jwt` in that file), add an `auth.session` entry with config keys `service`.

- [ ] **Step 7: Run all server + auth tests, verify pass**

Run: `go test ./internal/server/ ./plugins/auth/ ./pkg/api/...`
Expected: all PASS (including pre-existing middleware/ordering tests).

- [ ] **Step 8: Commit**

```bash
gofmt -l ./internal/server ./plugins/auth ./pkg/api && go vet ./internal/server/... ./plugins/auth/... ./pkg/api/...
git add pkg/api plugins/auth internal/server
git commit -m "feat(server): auth.session middleware validating opaque session tokens"
```

---

### Task 7: `noda auth init` scaffold

**Files:**
- Create: `cmd/noda/auth_init.go`
- Create: `cmd/noda/auth_templates/` — see file list in Step 3
- Test: `cmd/noda/auth_init_test.go`
- Modify: `cmd/noda/main.go` (register `newAuthCmd()` alongside the other subcommands — find where `newPluginCmd()` is added)

**Interfaces:**
- Consumes: `noda init` template embedding pattern (`cmd/noda/init.go:15-60`), config loading from `internal/config` for validation in tests.
- Produces: `noda auth init [--dir DIR]` command that writes migrations + 7 workflows + 7 routes + 7 tests and patches `noda.json`. Workflow/route templates use Go `text/template` with delimiters `[[ ]]` (JSON bodies contain Noda `{{ }}` expressions) and data `struct{ DBService, EmailService string }`.

- [ ] **Step 1: Write failing test**

`cmd/noda/auth_init_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeMinimalProject(t *testing.T, dir string, withEmail bool) {
	t.Helper()
	services := map[string]any{
		"main-db": map[string]any{"plugin": "db", "config": map[string]any{"driver": "sqlite", "path": "data/app.db"}},
	}
	if withEmail {
		services["mailer"] = map[string]any{"plugin": "email", "config": map[string]any{"host": "localhost", "port": 1025}}
	}
	root := map[string]any{"services": services}
	b, _ := json.MarshalIndent(root, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "noda.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestAuthInitScaffold(t *testing.T) {
	dir := t.TempDir()
	writeMinimalProject(t, dir, true)

	if err := runAuthInit(dir); err != nil {
		t.Fatal(err)
	}

	// files exist
	for _, glob := range []string{
		"migrations/*_auth_tables.up.sql",
		"migrations/*_auth_tables.down.sql",
		"workflows/auth.register.json",
		"workflows/auth.login.json",
		"workflows/auth.logout.json",
		"workflows/auth.me.json",
		"workflows/auth.verify-email.json",
		"workflows/auth.request-password-reset.json",
		"workflows/auth.reset-password.json",
		"routes/auth.register.json",
		"routes/auth.login.json",
		"routes/auth.logout.json",
		"routes/auth.me.json",
		"routes/auth.verify-email.json",
		"routes/auth.request-password-reset.json",
		"routes/auth.reset-password.json",
		"tests/test-auth-register.json",
		"tests/test-auth-login.json",
	} {
		m, _ := filepath.Glob(filepath.Join(dir, glob))
		if len(m) != 1 {
			t.Fatalf("missing scaffolded file: %s", glob)
		}
	}

	// db service name substituted into workflows
	b, _ := os.ReadFile(filepath.Join(dir, "workflows", "auth.login.json"))
	var wf map[string]any
	if err := json.Unmarshal(b, &wf); err != nil {
		t.Fatalf("scaffolded workflow is not valid JSON: %v", err)
	}
	if !containsString(string(b), `"main-db"`) {
		t.Fatal("db service name not substituted")
	}

	// noda.json patched
	rb, _ := os.ReadFile(filepath.Join(dir, "noda.json"))
	var root map[string]any
	json.Unmarshal(rb, &root)
	services := root["services"].(map[string]any)
	authSvc, ok := services["auth"].(map[string]any)
	if !ok || authSvc["plugin"] != "auth" {
		t.Fatal("auth service not added to noda.json")
	}
	if authSvc["config"].(map[string]any)["database"] != "main-db" {
		t.Fatal("auth service database not set")
	}

	// sqlite dialect migration (project driver is sqlite)
	m, _ := filepath.Glob(filepath.Join(dir, "migrations", "*_auth_tables.up.sql"))
	sql, _ := os.ReadFile(m[0])
	if containsString(string(sql), "JSONB") || containsString(string(sql), "TIMESTAMPTZ") {
		t.Fatal("sqlite project must get sqlite-dialect migration")
	}
}

func TestAuthInitCollisionAborts(t *testing.T) {
	dir := t.TempDir()
	writeMinimalProject(t, dir, true)
	os.MkdirAll(filepath.Join(dir, "workflows"), 0755)
	os.WriteFile(filepath.Join(dir, "workflows", "auth.login.json"), []byte("{}"), 0644)

	if err := runAuthInit(dir); err == nil {
		t.Fatal("collision must abort")
	}
	// all-or-nothing: nothing else written
	if _, err := os.Stat(filepath.Join(dir, "workflows", "auth.register.json")); err == nil {
		t.Fatal("must not write any file when aborting")
	}
}

func TestAuthInitWithoutEmailServiceWarnsButSucceeds(t *testing.T) {
	dir := t.TempDir()
	writeMinimalProject(t, dir, false)
	if err := runAuthInit(dir); err != nil {
		t.Fatalf("must succeed without email service: %v", err)
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && strings.Contains(haystack, needle)
}
```

(Add `"strings"` import; use `strings.Contains` directly if preferred.)

- [ ] **Step 2: Run, verify failure**

Run: `go test ./cmd/noda/ -run TestAuthInit -v`
Expected: compile FAIL (`runAuthInit` undefined).

- [ ] **Step 3: Create templates**

Create `cmd/noda/auth_templates/` with these files. Migrations (four files):

`migrations/postgres.up.sql`:

```sql
CREATE TABLE auth_users (
  id                TEXT PRIMARY KEY,
  email             TEXT NOT NULL UNIQUE,
  password_hash     TEXT NOT NULL,
  email_verified_at TIMESTAMPTZ,
  status            TEXT NOT NULL DEFAULT 'active',
  roles             JSONB NOT NULL DEFAULT '["user"]',
  metadata          JSONB NOT NULL DEFAULT '{}',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE auth_sessions (
  id           TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
  token_hash   TEXT NOT NULL UNIQUE,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at   TIMESTAMPTZ NOT NULL,
  last_used_at TIMESTAMPTZ,
  ip           TEXT,
  user_agent   TEXT,
  revoked_at   TIMESTAMPTZ
);
CREATE INDEX idx_auth_sessions_user ON auth_sessions(user_id);

CREATE TABLE auth_tokens (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
  purpose     TEXT NOT NULL,
  token_hash  TEXT NOT NULL UNIQUE,
  expires_at  TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_tokens_user_purpose ON auth_tokens(user_id, purpose);
```

`migrations/postgres.down.sql`:

```sql
DROP TABLE IF EXISTS auth_tokens;
DROP TABLE IF EXISTS auth_sessions;
DROP TABLE IF EXISTS auth_users;
```

`migrations/sqlite.up.sql`: same tables with `TIMESTAMP` instead of `TIMESTAMPTZ`, `TEXT` instead of `JSONB`, and `CURRENT_TIMESTAMP` instead of `now()`; `migrations/sqlite.down.sql` identical to the postgres down file.

Workflows (`.tmpl` where substitution is needed). `workflows/auth.register.json.tmpl`:

```json
{
  "id": "auth-register",
  "name": "Auth: Register",
  "nodes": {
    "create_user": {
      "type": "auth.create_user",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "email": "{{ input.email }}", "password": "{{ input.password }}" }
    },
    "respond_exists": {
      "type": "response.json",
      "config": { "status": 400, "body": { "error": "registration failed" } }
    },
    "verify_token": {
      "type": "auth.create_token",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "user_id": "{{ nodes.create_user.id }}", "purpose": "verify_email" }
    },
    "send_verify_email": {
      "type": "email.send",
      "services": { "email": "[[.EmailService]]" },
      "config": {
        "to": "{{ nodes.create_user.email }}",
        "subject": "Verify your email",
        "body": "<p>Welcome! Verify your email with this token: <strong>{{ nodes.verify_token.token }}</strong></p>"
      }
    },
    "session": {
      "type": "auth.create_session",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "user_id": "{{ nodes.create_user.id }}" }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": { "user": "{{ nodes.create_user }}", "token": "{{ nodes.session.token }}" },
        "cookies": "{{ [nodes.session.cookie] }}"
      }
    }
  },
  "edges": [
    { "from": "create_user", "to": "verify_token" },
    { "from": "create_user", "output": "exists", "to": "respond_exists" },
    { "from": "verify_token", "to": "send_verify_email" },
    { "from": "send_verify_email", "to": "session" },
    { "from": "session", "to": "respond" }
  ]
}
```

**Check the edge format**: look at an existing multi-output workflow (e.g. search `examples/` for `"output":` in edges, or `docs/02-config/workflows.md`) and use the exact key the engine expects for selecting a non-success output port; adjust all templates accordingly.

`workflows/auth.login.json.tmpl`:

```json
{
  "id": "auth-login",
  "name": "Auth: Login",
  "nodes": {
    "verify": {
      "type": "auth.verify_credentials",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "email": "{{ input.email }}", "password": "{{ input.password }}" }
    },
    "respond_invalid": {
      "type": "response.json",
      "config": { "status": 401, "body": { "error": "invalid credentials" } }
    },
    "session": {
      "type": "auth.create_session",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "user_id": "{{ nodes.verify.id }}" }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "user": "{{ nodes.verify }}", "token": "{{ nodes.session.token }}" },
        "cookies": "{{ [nodes.session.cookie] }}"
      }
    }
  },
  "edges": [
    { "from": "verify", "to": "session" },
    { "from": "verify", "output": "invalid", "to": "respond_invalid" },
    { "from": "session", "to": "respond" }
  ]
}
```

`workflows/auth.logout.json.tmpl`:

```json
{
  "id": "auth-logout",
  "name": "Auth: Logout",
  "nodes": {
    "revoke": {
      "type": "auth.revoke_session",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "session_id": "{{ input.session_id }}" }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 204, "cookies": "{{ [nodes.revoke.clear_cookie] }}" }
    }
  },
  "edges": [ { "from": "revoke", "to": "respond" } ]
}
```

`workflows/auth.me.json.tmpl`:

```json
{
  "id": "auth-me",
  "name": "Auth: Current User",
  "nodes": {
    "get": {
      "type": "auth.get_user",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "user_id": "{{ input.user_id }}" }
    },
    "respond_missing": {
      "type": "response.json",
      "config": { "status": 401, "body": { "error": "invalid token" } }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 200, "body": "{{ nodes.get }}" }
    }
  },
  "edges": [
    { "from": "get", "to": "respond" },
    { "from": "get", "output": "not_found", "to": "respond_missing" }
  ]
}
```

`workflows/auth.verify-email.json.tmpl`:

```json
{
  "id": "auth-verify-email",
  "name": "Auth: Verify Email",
  "nodes": {
    "consume": {
      "type": "auth.consume_token",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "token": "{{ input.token }}", "purpose": "verify_email" }
    },
    "respond_invalid": {
      "type": "response.json",
      "config": { "status": 400, "body": { "error": "invalid or expired token" } }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 200, "body": { "verified": true } }
    }
  },
  "edges": [
    { "from": "consume", "to": "respond" },
    { "from": "consume", "output": "invalid", "to": "respond_invalid" }
  ]
}
```

`workflows/auth.request-password-reset.json.tmpl` (both branches return identical 200 — no user enumeration; the branch is visible in the editor):

```json
{
  "id": "auth-request-password-reset",
  "name": "Auth: Request Password Reset",
  "nodes": {
    "find_user": {
      "type": "auth.get_user",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "email": "{{ input.email }}" }
    },
    "reset_token": {
      "type": "auth.create_token",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "user_id": "{{ nodes.find_user.id }}", "purpose": "reset_password" }
    },
    "send_reset_email": {
      "type": "email.send",
      "services": { "email": "[[.EmailService]]" },
      "config": {
        "to": "{{ nodes.find_user.email }}",
        "subject": "Reset your password",
        "body": "<p>Reset your password with this token: <strong>{{ nodes.reset_token.token }}</strong></p><p>If you did not request this, ignore this email.</p>"
      }
    },
    "respond_sent": {
      "type": "response.json",
      "config": { "status": 200, "body": { "message": "If that account exists, an email was sent" } }
    },
    "respond_unknown": {
      "type": "response.json",
      "config": { "status": 200, "body": { "message": "If that account exists, an email was sent" } }
    }
  },
  "edges": [
    { "from": "find_user", "to": "reset_token" },
    { "from": "find_user", "output": "not_found", "to": "respond_unknown" },
    { "from": "reset_token", "to": "send_reset_email" },
    { "from": "send_reset_email", "to": "respond_sent" }
  ]
}
```

`workflows/auth.reset-password.json.tmpl`:

```json
{
  "id": "auth-reset-password",
  "name": "Auth: Reset Password",
  "nodes": {
    "consume": {
      "type": "auth.consume_token",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "token": "{{ input.token }}", "purpose": "reset_password" }
    },
    "respond_invalid": {
      "type": "response.json",
      "config": { "status": 400, "body": { "error": "invalid or expired token" } }
    },
    "set_password": {
      "type": "auth.set_password",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "user_id": "{{ nodes.consume.user_id }}", "password": "{{ input.password }}" }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 200, "body": { "message": "password updated" } }
    }
  },
  "edges": [
    { "from": "consume", "to": "set_password" },
    { "from": "consume", "output": "invalid", "to": "respond_invalid" },
    { "from": "set_password", "to": "respond" }
  ]
}
```

Routes (static JSON, no substitution — copy the shape of `examples/rest-api/routes/create-task.json`). `routes/auth.register.json`:

```json
{
  "id": "auth-register",
  "method": "POST",
  "path": "/auth/register",
  "summary": "Register a new account",
  "tags": ["auth"],
  "middleware": ["limiter"],
  "body": {
    "schema": {
      "type": "object",
      "required": ["email", "password"],
      "properties": {
        "email": { "type": "string" },
        "password": { "type": "string" }
      }
    }
  },
  "trigger": {
    "workflow": "auth-register",
    "input": { "email": "{{ body.email }}", "password": "{{ body.password }}" }
  }
}
```

The other six routes follow the same shape:

| File | Method/Path | Middleware | Workflow | Trigger input |
|---|---|---|---|---|
| `routes/auth.login.json` | `POST /auth/login` | `["limiter"]` | `auth-login` | `email`, `password` from body |
| `routes/auth.logout.json` | `POST /auth/logout` | `["auth.session"]` | `auth-logout` | `session_id: "{{ auth.claims.session_id }}"` |
| `routes/auth.me.json` | `GET /auth/me` | `["auth.session"]` | `auth-me` | `user_id: "{{ auth.user_id }}"` |
| `routes/auth.verify-email.json` | `POST /auth/verify-email` | `[]` | `auth-verify-email` | `token` from body |
| `routes/auth.request-password-reset.json` | `POST /auth/request-password-reset` | `["limiter"]` | `auth-request-password-reset` | `email` from body |
| `routes/auth.reset-password.json` | `POST /auth/reset-password` | `["limiter"]` | `auth-reset-password` | `token`, `password` from body |

Write each of the six as a complete JSON file with a body schema for its inputs (same pattern as the register route; `GET /auth/me` and logout have no body block). **Check the trigger-input expression names** (`auth.claims`, `auth.user_id`, `body.*`) against `docs/01-getting-started/expressions.md` and `examples/rest-api/routes/create-task.json` (which uses `auth.sub`) — use whichever key the trigger mapping actually exposes for the authenticated user id, and keep it consistent across the six files.

Tests (static, mock-based, following `examples/rest-api/tests/test-create-task.json`). `tests/test-auth-login.json`:

```json
{
  "id": "test-auth-login",
  "workflow": "auth-login",
  "tests": [
    {
      "name": "valid credentials get a session",
      "input": { "email": "alice@example.com", "password": "password123" },
      "mocks": {
        "verify": { "output": { "id": "user-1", "email": "alice@example.com", "roles": ["user"] } },
        "session": { "output": { "token": "tok", "session_id": "s1", "cookie": { "name": "noda_session", "value": "tok" } } },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    },
    {
      "name": "invalid credentials get 401",
      "input": { "email": "alice@example.com", "password": "wrong" },
      "mocks": {
        "verify": { "output_name": "invalid", "output": {} },
        "respond_invalid": { "output": { "status": 401 } }
      },
      "expect": { "status": "success", "output": { "respond_invalid.status": 401 } }
    }
  ]
}
```

**Check the mock syntax for selecting a named output** (`output_name` above is a guess): read `docs/02-config/tests.md` and `internal/testing/` for the actual key, and fix all test templates to match. Write the remaining six test files with the same two-case pattern (happy path + main failure path) for their workflows.

- [ ] **Step 4: Implement the command**

`cmd/noda/auth_init.go`:

```go
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

//go:embed auth_templates
var authTemplateFS embed.FS

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication scaffolding",
	}
	var dir string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold auth flows (routes, workflows, migrations, tests) into this project",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runAuthInit(dir)
		},
	}
	initCmd.Flags().StringVar(&dir, "dir", ".", "project directory (must contain noda.json)")
	cmd.AddCommand(initCmd)
	return cmd
}

type authScaffoldData struct {
	DBService    string
	EmailService string
}

func runAuthInit(dir string) error {
	rootPath := filepath.Join(dir, "noda.json")
	rootBytes, err := os.ReadFile(rootPath)
	if err != nil {
		return fmt.Errorf("auth init: read noda.json: %w", err)
	}
	var root map[string]any
	if err := json.Unmarshal(rootBytes, &root); err != nil {
		return fmt.Errorf("auth init: parse noda.json: %w", err)
	}

	services, _ := root["services"].(map[string]any)
	dbName, driver := findServiceByPlugin(services, "db")
	if dbName == "" {
		return fmt.Errorf("auth init: no database service (plugin \"db\") found in noda.json — add one first")
	}
	if driver == "" {
		driver = "postgres"
	}
	emailName, _ := findServiceByPlugin(services, "email")
	if emailName == "" {
		emailName = "email"
		fmt.Fprintln(os.Stderr, "warning: no email service configured — verify-email and password-reset flows need a service named \"email\" (or edit the generated workflows)")
	}
	if _, exists := services["auth"]; exists {
		return fmt.Errorf("auth init: services.auth already exists in noda.json")
	}

	data := authScaffoldData{DBService: dbName, EmailService: emailName}
	timestamp := time.Now().UTC().Format("20060102150405")

	// Build the full output set in memory first (all-or-nothing).
	outputs := map[string][]byte{}

	upSQL, err := authTemplateFS.ReadFile("auth_templates/migrations/" + driver + ".up.sql")
	if err != nil {
		return fmt.Errorf("auth init: unsupported db driver %q", driver)
	}
	downSQL, _ := authTemplateFS.ReadFile("auth_templates/migrations/" + driver + ".down.sql")
	outputs[filepath.Join("migrations", timestamp+"_auth_tables.up.sql")] = upSQL
	outputs[filepath.Join("migrations", timestamp+"_auth_tables.down.sql")] = downSQL

	err = fs.WalkDir(authTemplateFS, "auth_templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(path, "auth_templates/migrations/") {
			return err
		}
		rel := strings.TrimPrefix(path, "auth_templates/")
		content, err := authTemplateFS.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(rel, ".tmpl") {
			tpl, err := template.New(rel).Delims("[[", "]]").Parse(string(content))
			if err != nil {
				return fmt.Errorf("template %s: %w", rel, err)
			}
			var buf strings.Builder
			if err := tpl.Execute(&buf, data); err != nil {
				return fmt.Errorf("template %s: %w", rel, err)
			}
			content = []byte(buf.String())
			rel = strings.TrimSuffix(rel, ".tmpl")
		}
		outputs[rel] = content
		return nil
	})
	if err != nil {
		return fmt.Errorf("auth init: %w", err)
	}

	// Collision check before writing anything.
	var collisions []string
	for rel := range outputs {
		if _, err := os.Stat(filepath.Join(dir, rel)); err == nil {
			collisions = append(collisions, rel)
		}
	}
	if len(collisions) > 0 {
		return fmt.Errorf("auth init: refusing to overwrite existing files:\n  %s", strings.Join(collisions, "\n  "))
	}

	for rel, content := range outputs {
		target := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("auth init: %w", err)
		}
		if err := os.WriteFile(target, content, 0644); err != nil {
			return fmt.Errorf("auth init: %w", err)
		}
	}

	// Patch noda.json: services.auth + middleware preset.
	services["auth"] = map[string]any{
		"plugin": "auth",
		"config": map[string]any{"database": dbName},
	}
	root["services"] = services
	presets, _ := root["middleware_presets"].(map[string]any)
	if presets == nil {
		presets = map[string]any{}
	}
	if _, exists := presets["authenticated_session"]; !exists {
		presets["authenticated_session"] = []any{"auth.session"}
	}
	root["middleware_presets"] = presets
	patched, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("auth init: %w", err)
	}
	if err := os.WriteFile(rootPath, append(patched, '\n'), 0644); err != nil {
		return fmt.Errorf("auth init: %w", err)
	}

	fmt.Printf("Scaffolded auth: %d files + noda.json updated.\nNext steps:\n  1. noda migrate up\n  2. Open the auth-* workflows in the editor and customize\n  3. noda test\n", len(outputs))
	return nil
}

func findServiceByPlugin(services map[string]any, pluginName string) (name, driver string) {
	for n, v := range services {
		svc, ok := v.(map[string]any)
		if !ok || svc["plugin"] != pluginName {
			continue
		}
		cfg, _ := svc["config"].(map[string]any)
		d, _ := cfg["driver"].(string)
		return n, d
	}
	return "", ""
}
```

Register `newAuthCmd()` in `cmd/noda/main.go` where the other subcommands are added. Note `json.MarshalIndent` alphabetizes `noda.json` keys — mention in the command output? No: document in the guide (Task 9).

- [ ] **Step 5: Run tests, verify pass**

Run: `go test ./cmd/noda/ -run TestAuthInit -v`
Expected: PASS.

- [ ] **Step 6: Validate scaffolded output against the real config loader**

Add to `auth_init_test.go`:

```go
func TestAuthInitOutputValidates(t *testing.T) {
	dir := t.TempDir()
	writeMinimalProject(t, dir, true)
	if err := runAuthInit(dir); err != nil {
		t.Fatal(err)
	}
	// Load through the real config pipeline — catches schema violations,
	// unknown node types, bad edges, and unknown middleware.
	// Use the same entry point the `noda validate` / runtime path uses
	// (see cmd/noda/runtime.go for how config is loaded from a dir).
	rc, err := loadResolvedConfigForTest(dir)
	if err != nil {
		t.Fatalf("scaffolded project fails config load: %v", err)
	}
	if len(rc.Workflows) < 7 {
		t.Fatalf("expected ≥7 workflows, got %d", len(rc.Workflows))
	}
}
```

Implement `loadResolvedConfigForTest` by extracting/calling the config-loading used in `cmd/noda/runtime.go` (find the `config.Load...` call there and reuse it). If workflow compilation is separate (`engine.NewWorkflowCache`), also compile the workflows to catch bad node types/edges. Run and fix any template mistakes this uncovers (this is the step that catches wrong edge/mock/expression syntax guessed in Step 3).

- [ ] **Step 7: Commit**

```bash
gofmt -l ./cmd/noda && go vet ./cmd/noda/...
git add cmd/noda
git commit -m "feat(cli): noda auth init scaffolds project-owned auth flows"
```

---

### Task 8: Engine-level integration test + `examples/auth-demo`

**Files:**
- Create: `plugins/auth/engine_e2e_integration_test.go`
- Create: `testdata/auth/` (config used by the e2e test — generated by the scaffold, committed)
- Create: `examples/auth-demo/` (scaffolded example project + README.md + docker-compose.yml)

**Interfaces:**
- Consumes: everything from Tasks 1–7; existing engine e2e pattern in `plugins/db/engine_e2e_integration_test.go` (read it first and mirror its bootstrap: registry + workflow cache + engine dispatch).

- [ ] **Step 1: Generate the test fixture with the scaffold**

```bash
mkdir -p testdata/auth && cd testdata/auth
cat > noda.json <<'EOF'
{
  "services": {
    "main-db": { "plugin": "db", "config": { "driver": "sqlite", "path": "{{ $env('AUTH_TEST_DB') }}" } },
    "email":   { "plugin": "email", "config": { "host": "localhost", "port": 1025 } }
  }
}
EOF
go run ./cmd/noda auth init --dir testdata/auth
```

(Adjust the db config keys to whatever `plugins/db` sqlite expects — `driver`/`path` per `plugins/db/plugin.go:52-58,108-119`. If `$env` in a path isn't supported for sqlite services, use a fixed relative path and copy the fixture to a temp dir in the test.)

- [ ] **Step 2: Write the e2e test**

`plugins/auth/engine_e2e_integration_test.go` — mirror `plugins/db/engine_e2e_integration_test.go`'s setup (build tag / short-mode skip conventions included). Flow to assert, using the *scaffolded* workflows from `testdata/auth` with the register/reset email nodes **mocked or the email service replaced by a no-op** (the workflow test runner's mock mechanism, or override the `email` service via the engine's service-override support — whichever the db e2e precedent uses; do not require Mailpit here):

1. Apply `testdata/auth/migrations/*.up.sql` to a temp sqlite DB via `internal/migrate.Up`.
2. Run `auth-register` with `{email: "alice@example.com", password: "password123"}` → assert 201 response node output, a user row exists, a session row exists, and the response body contains a token.
3. Run `auth-login` with correct credentials → 200 + token; with wrong password → the workflow ends at `respond_invalid` (401 output).
4. Run `auth-request-password-reset` for both an existing and a non-existing email → identical 200 response body on both paths (enumeration check).
5. Read the reset token from the `auth_tokens` table (hash only — mint path can't be read back, so instead capture the token from the mocked email node's input, or run `auth.create_token` directly), run `auth-reset-password`, then assert old password fails login and new password succeeds.
6. Run `auth-logout` with the login session, then `svc.AuthenticateSession` with the logged-out token → nil (revoked).

Write it as one test function with subtests (`t.Run("register", ...)` etc.) sharing the temp DB, in that order.

- [ ] **Step 3: Run, iterate until pass**

Run: `go test ./plugins/auth/ -run TestEngineE2E -v` (plus whatever build tag the db e2e uses, e.g. `-tags integration`).
Expected: PASS. This is the step that shakes out real edge-format/expression issues in the templates; fix templates in `cmd/noda/auth_templates/` and regenerate `testdata/auth` when it does.

- [ ] **Step 4: Create `examples/auth-demo`**

```bash
mkdir -p examples/auth-demo && cd examples/auth-demo
# noda.json with postgres db + mailpit email service, matching other examples' docker-compose style
go run ./cmd/noda auth init --dir examples/auth-demo
```

`examples/auth-demo/noda.json` (pre-scaffold seed):

```json
{
  "services": {
    "main-db": { "plugin": "db", "config": { "url": "{{ $env('DATABASE_URL') }}" } },
    "email":   { "plugin": "email", "config": { "host": "{{ $env('SMTP_HOST') }}", "port": 1025, "from": "noreply@example.com" } }
  },
  "security": {}
}
```

(Match the email service config keys to `plugins/email/plugin.go`.) Add `docker-compose.yml` (postgres + mailpit + noda, modeled on `examples/saas-backend/docker-compose.yml`) and a `README.md` explaining: what got scaffolded, the register→verify→login→me→logout curl sequence, how to open the flows in the editor, and the customization points (invite codes, extra fields via `metadata`, welcome emails).

- [ ] **Step 5: Verify the example boots**

Run: `docker compose -f examples/auth-demo/docker-compose.yml up -d && sleep 5` then the curl sequence from the README (register, login, me with cookie, logout). Expected: 201/200/200/204. Tear down with `docker compose ... down -v`. (Mailpit e2e flake note: if the email container races, re-run — known issue.)

- [ ] **Step 6: Commit**

```bash
git add plugins/auth testdata/auth examples/auth-demo
git commit -m "test(auth): engine e2e over scaffolded flows + auth-demo example"
```

---

### Task 9: Documentation + branch finalization

**Files:**
- Create: `docs/03-nodes/auth.create_user.md`, `auth.get_user.md`, `auth.verify_credentials.md`, `auth.create_session.md`, `auth.revoke_session.md`, `auth.create_token.md`, `auth.consume_token.md`, `auth.set_password.md`
- Create: `docs/04-guides/authentication.md`
- Modify: `docs/03-nodes/_index.md` (add the 8 nodes), `docs/02-config/middleware.md` (auth.session section + ordering note), `docs/02-config/noda-json.md` (auth service section)

- [ ] **Step 1: Write the 8 node docs**

Follow the exact structure of `docs/03-nodes/util.jwt_sign.md` (title, one-liner, Config table, Outputs, Behavior, Example, Service Dependencies). Content source: each node's `ConfigSchema`/`OutputDescriptions` from Tasks 2–5 — the tables must list every config field with type/required/description, outputs must name the domain outputs (`exists`/`invalid`/`not_found`), and each example should be the corresponding node config from the scaffolded workflows (Task 7 templates). In `auth.create_session.md` and `auth.revoke_session.md`, document the `cookie`/`clear_cookie` output objects and show the `response.json` `"cookies": "{{ [nodes.session.cookie] }}"` pattern. In `auth.verify_credentials.md`, document the timing-safe behavior and bcrypt auto-upgrade. In `auth.consume_token.md`, document atomic single-use and that `verify_email` consumption sets `email_verified_at`.

- [ ] **Step 2: Middleware + service config docs**

`docs/02-config/middleware.md`: add an `### auth.session` section after `### auth.oidc` documenting: token sources (cookie then bearer), config (`security.session` path, `service` key), locals populated (same as auth.jwt: `auth.user_id`, `auth.roles`, `auth.claims` with `sub/email/email_verified/session_id/roles`), 401 behavior, and update the ordering constraints line to include `auth.session` as a valid casbin prerequisite.

`docs/02-config/noda-json.md`: add an `### Auth Service (plugin: "auth")` section with the full config table (database, session.ttl, session.cookie.*, argon2.*, tokens.*) and defaults, plus the example JSON block from the spec §2.

- [ ] **Step 3: Write the guide**

`docs/04-guides/authentication.md` covering: the shadcn model (what the plugin owns vs what your project owns), `noda auth init` walkthrough, the seven flows and how to customize them in the editor (invite-code example), sessions vs `auth.jwt`/`auth.oidc` and when to use which, security defaults and their reasoning (argon2id, opaque tokens, enumeration-safe reset, reset revokes sessions), CSRF guidance for cross-site frontends (`security.csrf`), the residual reset-request timing signal and the `event.emit` hardening pattern, bcrypt import path for migrating existing user tables, and the note that `noda auth init` rewrites `noda.json` with alphabetized keys.

- [ ] **Step 4: Editor spot-check**

Run `go run ./cmd/noda dev` (or however the editor is served — check `cmd/noda` for the dev command) against `examples/auth-demo`, open the editor, and confirm the 8 auth nodes appear in the palette with their config fields, and the `auth-register` workflow renders. Fix descriptor schema issues if fields don't render.

- [ ] **Step 5: Full verification + finalize branch**

```bash
go build ./... && go test ./... && gofmt -l . && go vet ./...
git add docs/03-nodes docs/02-config docs/04-guides
git commit -m "docs: auth plugin node reference, middleware config, authentication guide"
git add -f docs/superpowers/specs/2026-07-04-auth-plugin-design.md docs/superpowers/plans/2026-07-04-auth-plugin.md
git commit -m "docs: auth plugin spec and implementation plan (point-in-time records)"
```

Expected: full test suite green (`TestEmailSend_Engine` flake: re-run once if it fails without touching `plugins/email`). Then follow superpowers:finishing-a-development-branch (PR to `main`).

---

## Self-Review Notes (already applied)

- Spec §5/§7 cookie handling was aligned to the real response architecture (`response.json` sets cookies; nodes emit cookie objects) before this plan was written.
- Steps that depend on formats I could not fully verify from static reading are flagged inline with **Check …** instructions (edge `output` key, test-mock output selection key, trigger-input auth expression names, sqlite service config keys, e2e build tags). Task 7 Step 6 and Task 8 Step 3 are the designed safety nets that force those to be validated against the real loader/engine.
- Type consistency verified: `Service`/`ArgonParams`/`CookieConfig`, `MintToken/HashToken/VerifyPassword/VerifyDummy`, node factory names (`new<X>Executor`), output names (`success/exists/invalid/not_found/error`), claim keys (`sub/email/email_verified/session_id/roles`), and cookie object keys (`name/value/path/domain/max_age/secure/http_only/same_site`) are used identically across Tasks 1–9.
