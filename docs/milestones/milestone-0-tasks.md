# Milestone 0: Project Skeleton — Task Breakdown

**Depends on:** Nothing (first milestone)
**Result:** Go module compiles, CI pipeline green, Docker builds, all public interfaces defined in `pkg/api/`.

---

## Task 0.1: Initialize Go Module and Dependencies

**Description:** Create the Go module and declare all dependencies from the technology stack.

**Subtasks:**

- [ ] Initialize Go module: `go mod init github.com/<org>/noda`
- [ ] Add all direct dependencies:
  - `gofiber/fiber/v3` — HTTP framework
  - `spf13/cobra` — CLI
  - `gorm.io/gorm` + `gorm.io/driver/postgres` — ORM
  - `redis/go-redis/v9` — Redis client
  - `gofiber/contrib/jwt` + `golang-jwt/jwt/v5` — JWT auth
  - `casbin/casbin/v2` — Authorization
  - `expr-lang/expr` — Expression engine
  - `santhosh-tekuri/jsonschema/v6` — JSON Schema validation
  - `spf13/afero` — Storage abstraction
  - `h2non/bimg` — Image processing (libvips)
  - `robfig/cron/v3` — Cron scheduler
  - `google/uuid` — UUID generation
  - `go.opentelemetry.io/otel` + tracing/export packages — Observability
  - `getkin/kin-openapi` — OpenAPI generation
  - `fsnotify/fsnotify` — File watching
  - `extism/go-sdk` — Wasm runtime
  - `vmihailenco/msgpack/v5` — MessagePack encoding
- [ ] Add test dependencies:
  - `stretchr/testify` — Test assertions
- [ ] Run `go mod tidy` — verify all dependencies resolve
- [ ] Create a minimal `main.go` that compiles (just a `main()` printing version)

**Acceptance criteria:** `go build ./...` succeeds. `go mod tidy` produces no changes.

---

## Task 0.2: Directory Structure

**Description:** Create the project directory structure matching the architecture plan.

**Subtasks:**

- [ ] Create source directories:
  ```
  cmd/noda/           — CLI entry point (main.go)
  pkg/api/            — public interfaces (the plugin author contract)
  internal/config/    — config loading, merging, validation
  internal/engine/    — workflow engine (graph compiler, executor)
  internal/expr/      — expression parser, compiler, evaluator
  internal/registry/  — plugin registry, service registry, node registry
  internal/server/    — Fiber HTTP server, route registration
  internal/worker/    — worker runtime (stream consumer)
  internal/scheduler/ — scheduler runtime (cron)
  internal/connmgr/   — connection manager (WebSocket/SSE)
  internal/wasm/      — Wasm runtime (Extism host)
  internal/trace/     — tracing, dev mode trace WebSocket
  plugins/db/         — PostgreSQL plugin
  plugins/cache/      — Redis cache plugin
  plugins/stream/     — Redis Streams plugin
  plugins/pubsub/     — Redis PubSub plugin
  plugins/storage/    — Afero storage plugin
  plugins/image/      — bimg image plugin
  plugins/http/       — outbound HTTP plugin
  plugins/email/      — email plugin
  plugins/core/       — core node plugins (control, transform, response, util, event, upload, ws, sse, wasm)
  ```
- [ ] Create test and fixture directories:
  ```
  testdata/           — test fixtures (sample projects, Wasm modules)
  ```
- [ ] Create documentation and config directories:
  ```
  docs/               — architecture docs, planning docs
  ```
- [ ] Add `.gitkeep` files in empty directories so Git tracks them
- [ ] Create `.gitignore` with Go defaults, IDE files, `dist/`, `tmp/`, `.env`

**Acceptance criteria:** Directory structure exists. `git status` shows all directories tracked.

---

## Task 0.3: Dockerfile

**Description:** Create a multi-stage Dockerfile that builds Noda and produces a minimal runtime image with libvips.

**Subtasks:**

- [ ] Create `Dockerfile` with two stages:
  - **Builder stage:** Go build image, copy source, `go build -o /noda ./cmd/noda`
  - **Runtime stage:** Minimal image (Debian slim or Alpine) with `libvips` installed, copy binary from builder
- [ ] Builder stage should cache Go modules (copy `go.mod`/`go.sum` before source for layer caching)
- [ ] Runtime stage installs: `libvips-dev` (or `vips-dev` on Alpine), `ca-certificates`, `tzdata`
- [ ] Set `ENTRYPOINT ["/noda"]`
- [ ] Add `.dockerignore` to exclude `.git`, `testdata`, `docs`, IDE files
- [ ] Verify: `docker build -t noda:dev .` succeeds

**Acceptance criteria:** `docker build` succeeds. `docker run noda:dev version` prints the version string.

---

## Task 0.4: Docker Compose

**Description:** Create `docker-compose.yml` for local development with PostgreSQL and Redis.

**Subtasks:**

- [ ] Create `docker-compose.yml` with three services:
  - **noda** — builds from Dockerfile, mounts project directory, exposes ports (HTTP + editor), depends on postgres and redis
  - **postgres** — `postgres:16-alpine`, configured with default database `noda_dev`, credentials via environment
  - **redis** — `redis:7-alpine`, default port 6379
- [ ] Add volume mounts for Noda service:
  - Project config directory mounted for hot reload
  - Named volume for PostgreSQL data persistence
- [ ] Add health checks for postgres (`pg_isready`) and redis (`redis-cli ping`)
- [ ] Create `.env.example` with default development values:
  ```
  DATABASE_URL=postgres://noda:noda@postgres:5432/noda_dev?sslmode=disable
  REDIS_URL=redis://redis:6379/0
  JWT_SECRET=dev-secret-change-in-production
  NODA_ENV=development
  ```
- [ ] Verify: `docker compose up --build` starts all three services

**Acceptance criteria:** `docker compose up` starts Noda, PostgreSQL, and Redis. PostgreSQL is reachable. Redis is reachable.

---

## Task 0.5: Makefile

**Description:** Create a Makefile with standard development targets.

**Subtasks:**

- [ ] Create `Makefile` with targets:
  - `build` — `go build -o dist/noda ./cmd/noda`
  - `test` — `go test ./... -race -count=1`
  - `test-coverage` — `go test ./... -race -coverprofile=coverage.out && go tool cover -html=coverage.out`
  - `lint` — `golangci-lint run ./...`
  - `fmt` — `gofmt -w .`
  - `dev` — `docker compose up --build`
  - `clean` — remove `dist/`, coverage files
  - `migrate-up` — placeholder (prints "not yet implemented")
  - `migrate-down` — placeholder
- [ ] Add `.PHONY` declarations for all targets
- [ ] Verify: `make build` produces the binary, `make test` runs (even if no tests yet — should pass with empty test suite)

**Acceptance criteria:** All make targets execute without errors.

---

## Task 0.6: CI Pipeline

**Description:** Create a GitHub Actions CI pipeline that runs on every push and pull request.

**Subtasks:**

- [ ] Create `.github/workflows/ci.yml` with:
  - **Trigger:** push to `main`, pull requests to `main`
  - **Go version:** latest stable (1.22+)
  - **Steps:**
    1. Checkout code
    2. Set up Go
    3. Cache Go modules
    4. Install golangci-lint
    5. Run `make lint`
    6. Run `make test`
    7. Run `make build`
- [ ] Configure golangci-lint: create `.golangci.yml` with:
  - Enable: `errcheck`, `gosimple`, `govet`, `ineffassign`, `staticcheck`, `unused`, `misspell`, `gofmt`
  - Timeout: 5 minutes
  - Exclude test files from certain linters where appropriate
- [ ] Verify: pipeline definition is valid YAML (can test with `act` locally or push to verify)

**Acceptance criteria:** CI pipeline configuration exists. When pushed to GitHub, it runs lint, test, and build steps.

---

## Task 0.7: Public Interfaces — Plugin and Node Registration

**Description:** Define the Plugin interface, NodeDescriptor, NodeRegistration, and ServiceDep types in `pkg/api/`.

**Subtasks:**

- [ ] Create `pkg/api/plugin.go`:
  ```go
  // Plugin is the top-level interface for all Noda plugins.
  type Plugin interface {
      Name() string
      Prefix() string
      Nodes() []NodeRegistration
      HasServices() bool
      CreateService(config map[string]any) (any, error)
      HealthCheck(service any) error
      Shutdown(service any) error
  }
  ```
- [ ] Create `pkg/api/node.go` with NodeDescriptor:
  ```go
  type ServiceDep struct {
      Prefix   string
      Required bool
  }

  type NodeDescriptor interface {
      Name() string
      ServiceDeps() map[string]ServiceDep
      ConfigSchema() map[string]any // JSON Schema as a Go map
  }
  ```
- [ ] Add NodeRegistration in `pkg/api/node.go`:
  ```go
  type NodeRegistration struct {
      Descriptor NodeDescriptor
      Factory    func(config map[string]any) NodeExecutor
  }
  ```
- [ ] Add package doc comment to `pkg/api/doc.go` explaining this is the stable public API for plugin authors
- [ ] Write tests: verify interfaces compile, verify types are usable (create mock implementations)

**Acceptance criteria:** `go build ./pkg/api/...` compiles. Mock implementations satisfy the interfaces.

---

## Task 0.8: Public Interfaces — Node Executor

**Description:** Define the NodeExecutor interface and ExecutionContext in `pkg/api/`.

**Subtasks:**

- [ ] Add NodeExecutor to `pkg/api/node.go`:
  ```go
  type NodeExecutor interface {
      Outputs() []string
      Execute(ctx context.Context, nCtx ExecutionContext, config map[string]any, services map[string]any) (outputName string, data any, err error)
  }
  ```
- [ ] Create `pkg/api/context.go` with ExecutionContext:
  ```go
  type AuthData struct {
      UserID string
      Roles  []string
      Claims map[string]any
  }

  type TriggerData struct {
      Type      string    // "http", "event", "schedule", "websocket", "wasm"
      Timestamp time.Time
      TraceID   string
  }

  type ExecutionContext interface {
      Input() any
      Auth() *AuthData  // nil if no auth
      Trigger() TriggerData
      Resolve(expression string) (any, error)
      Log(level string, message string, fields map[string]any)
  }
  ```
- [ ] Write tests: create a mock ExecutionContext, verify Resolve and Log can be called

**Acceptance criteria:** Interfaces compile. Mock executor can be created and its Execute method called.

---

## Task 0.9: Public Interfaces — Response Types

**Description:** Define HTTPResponse and Cookie types in `pkg/api/`.

**Subtasks:**

- [ ] Create `pkg/api/response.go`:
  ```go
  type Cookie struct {
      Name     string
      Value    string
      Path     string
      Domain   string
      MaxAge   int
      Secure   bool
      HTTPOnly bool
      SameSite string // "Strict", "Lax", "None"
  }

  type HTTPResponse struct {
      Status  int
      Headers map[string]string
      Cookies []Cookie
      Body    any
  }
  ```
- [ ] Write tests: create an HTTPResponse, verify all fields are accessible

**Acceptance criteria:** Types compile and are usable.

---

## Task 0.10: Public Interfaces — Common Service Interfaces

**Description:** Define StorageService, CacheService, and ConnectionService interfaces in `pkg/api/`.

**Subtasks:**

- [ ] Create `pkg/api/services.go`:
  ```go
  type StorageService interface {
      Read(ctx context.Context, path string) ([]byte, error)
      Write(ctx context.Context, path string, data []byte) error
      Delete(ctx context.Context, path string) error
      List(ctx context.Context, prefix string) ([]string, error)
  }

  type CacheService interface {
      Get(ctx context.Context, key string) (any, error)
      Set(ctx context.Context, key string, value any, ttl int) error
      Del(ctx context.Context, key string) error
      Exists(ctx context.Context, key string) (bool, error)
  }

  type ConnectionService interface {
      Send(ctx context.Context, channel string, data any) error
      SendSSE(ctx context.Context, channel string, event string, data any, id string) error
  }
  ```
- [ ] Write tests: verify each interface can be implemented by a mock

**Acceptance criteria:** Interfaces compile. Mock implementations satisfy them.

---

## Task 0.11: Public Interfaces — Standard Errors

**Description:** Define all standard error types in `pkg/api/`.

**Subtasks:**

- [ ] Create `pkg/api/errors.go`:
  ```go
  type ServiceUnavailableError struct {
      Service string
      Cause   error
  }

  type ValidationError struct {
      Field   string
      Message string
      Value   any
  }

  type TimeoutError struct {
      Duration  time.Duration
      Operation string
  }

  type NotFoundError struct {
      Resource string
      ID       string
  }

  type ConflictError struct {
      Resource string
      Reason   string
  }
  ```
- [ ] Implement `Error() string` on each type (satisfying Go's `error` interface)
- [ ] Create `pkg/api/error_data.go` for the standardized error output shape:
  ```go
  type ErrorData struct {
      Code     string
      Message  string
      NodeID   string
      NodeType string
      Details  any
  }
  ```
- [ ] Write tests: verify each error type satisfies `error` interface, verify `Error()` returns meaningful messages, verify type assertions work (`errors.As`, `errors.Is` patterns)

**Acceptance criteria:** All error types compile, implement `error`, and are distinguishable via type assertion.

---

## Task 0.12: CLI Entry Point

**Description:** Create the minimal CLI entry point with Cobra.

**Subtasks:**

- [ ] Create `cmd/noda/main.go`:
  - Initialize Cobra root command with `Use: "noda"`, version string, short/long description
  - Add global `--env` flag (string, default: `"development"`)
  - Add global `--config` flag (string, default: `"."` for current directory)
- [ ] Add `version` subcommand that prints Noda version
- [ ] Add placeholder subcommands (each prints "not yet implemented" and exits):
  - `validate`, `dev`, `start`, `test`, `generate`, `migrate`, `init`, `plugin`
- [ ] Verify: `go run ./cmd/noda version` prints version, `go run ./cmd/noda validate` prints placeholder message

**Acceptance criteria:** CLI compiles and runs. `noda version` works. All placeholder commands exist.

---

## Task 0.13: README and Project Documentation

**Description:** Create the project README and copy planning documents into the repository.

**Subtasks:**

- [ ] Create `README.md` with:
  - Project name and tagline
  - What Noda is (one paragraph)
  - Prerequisites (Go, Docker, Docker Compose)
  - Quick start (`docker compose up`, `make dev`)
  - Project structure overview
  - Development setup instructions
  - Link to architecture docs
- [ ] Copy all planning documents into `docs/`:
  - `docs/architecture-plan.md`
  - `docs/interfaces.md`
  - `docs/wasm-host-api.md`
  - `docs/config-conventions.md`
  - `docs/core-nodes.md`
  - `docs/visual-editor.md`
  - `docs/implementation-plan.md`
  - `docs/use-cases/` (all five use case files)
  - `docs/future-client-generation.md`
- [ ] Create `LICENSE` file (choose license — MIT recommended for open core)

**Acceptance criteria:** README exists with accurate setup instructions. All planning docs are in `docs/`.

---

## Task 0.14: Verification

**Description:** Final verification that everything works together.

**Subtasks:**

- [ ] Run `make build` — binary compiles
- [ ] Run `make test` — all tests pass (pkg/api/ interface tests)
- [ ] Run `make lint` — no lint errors
- [ ] Run `docker build -t noda:dev .` — Docker image builds
- [ ] Run `docker compose up` — all three services start
- [ ] Run `go vet ./...` — no issues
- [ ] Verify `dist/noda version` prints version string
- [ ] Verify `dist/noda validate` prints "not yet implemented"
- [ ] Verify all files in `pkg/api/` have doc comments
- [ ] Verify the CI pipeline YAML is valid

**Acceptance criteria:** All checks pass. The project is ready for Milestone 1.
