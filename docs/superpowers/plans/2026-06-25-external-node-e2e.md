# External-Service Node E2E Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Verify every external-service Noda node (`db.*`, `cache.*`, `event.emit` stream+pubsub, `email.send`) works end-to-end through the real engine against real containerized backends, fixing any bugs found.

**Architecture:** Reuse the existing engine-e2e pattern (`plugins/core/upload/engine_e2e_test.go`): build a `ServiceRegistry` with a real service from `plugin.CreateService`, build a `NodeRegistry`, `engine.Compile` → `engine.ExecuteGraph`, then assert both the node output and the backend effect plus an error path. The only change vs. the in-process round is that each service is backed by a `testcontainers-go` container instead of an in-memory fake. A shared `internal/testing/containers/` package provides `StartPostgres`/`StartRedis`/`StartMailpit` helpers.

**Tech Stack:** Go, `testcontainers-go` (postgres + redis modules + generic container for Mailpit), gorm (postgres), `go-redis/v9`, `net/smtp`, Mailpit HTTP API for assertions.

## Global Constraints

- Every new file in this plan starts with `//go:build integration` on the first line, followed by a blank line, then `package …`. Default `go test ./...` (no tag) must remain container-free and unaffected.
- Run the suite with `go test -tags=integration ./...` (Docker daemon must be running; testcontainers auto-pulls images).
- Container images are pinned: `postgres:17-alpine`, `redis:7-alpine`, `axllent/mailpit:v1.20`.
- `testcontainers-go` is a test-only dependency: it is reached only from `//go:build integration` files, so it never enters the production `noda` build graph.
- Service dependency names (the key in a node's `Services` map) are fixed by the node descriptors: `db.*` → `database`, `cache.*` → `cache`, `event.emit` → `stream` and `pubsub`, `email.send` → `mailer`.
- Service config keys are fixed by the plugins: db → `{driver:"postgres", url}`, cache/stream/pubsub → `{url}` (parsed by `redis.ParseURL`), email → `{host, port, from}`.
- Cache/stream/pubsub services implement `plugin.RedisClientProvider` (`Client() *redis.Client`); the db service is a `*gorm.DB`. Use these for direct backend assertions.
- Bug-handling: fix in place when a node bug surfaces; append an entry to `docs/superpowers/specs/2026-06-25-external-node-e2e-findings.md` (node, symptom, root cause, fix, guarding test).
- Tests isolate themselves on shared per-package containers via unique table names (Postgres), key prefixes (Redis), and unique subjects (Mailpit) — not per-case containers.

---

### Task 1: Container helper package + dependency + Makefile target

**Files:**
- Modify: `go.mod`, `go.sum` (add testcontainers-go)
- Create: `internal/testing/containers/containers.go`
- Create: `internal/testing/containers/containers_test.go`
- Modify: `Makefile` (add `test-integration` target)

**Interfaces:**
- Produces:
  - `func StartPostgres(t testing.TB) (url string)` — returns a `postgres://…?sslmode=disable` URL accepted by gorm's postgres driver.
  - `func StartRedis(t testing.TB) (url string)` — returns a `redis://host:port` URL parseable by `redis.ParseURL`.
  - `func StartMailpit(t testing.TB) (smtpHost string, smtpPort int, apiBaseURL string)` — SMTP endpoint for the email service config; `apiBaseURL` is `http://host:port` for Mailpit's HTTP API.
  - Each helper registers `t.Cleanup` to terminate its container and `t.Skip`s if the Docker daemon is unavailable.

- [ ] **Step 1: Add the dependency**

Run:
```bash
cd /Users/marten/GolandProjects/noda
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
go get github.com/testcontainers/testcontainers-go/modules/redis@latest
```
Expected: `go.mod`/`go.sum` updated, no build errors.

- [ ] **Step 2: Write the helper package**

Create `internal/testing/containers/containers.go`:
```go
//go:build integration

// Package containers provides testcontainers-backed helpers for external-service
// end-to-end tests. Each helper starts one container, registers cleanup, and
// skips the test if Docker is unavailable.
package containers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

func skipIfNoDocker(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Skipf("skipping: cannot start container (is Docker running?): %v", err)
	}
}

// StartPostgres starts a postgres:17-alpine container and returns a gorm-ready URL.
func StartPostgres(t testing.TB) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("noda"),
		postgres.WithUsername("noda"),
		postgres.WithPassword("noda"),
		postgres.BasicWaitStrategies(),
	)
	skipIfNoDocker(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}
	return url
}

// StartRedis starts a redis:7-alpine container and returns a redis:// URL.
func StartRedis(t testing.TB) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcredis.Run(ctx, "redis:7-alpine")
	skipIfNoDocker(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis connection string: %v", err)
	}
	return url
}

// StartMailpit starts a Mailpit container and returns its SMTP host/port and HTTP API base URL.
func StartMailpit(t testing.TB) (string, int, string) {
	t.Helper()
	ctx := context.Background()
	ctr, err := testcontainers.Run(ctx, "axllent/mailpit:v1.20",
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("1025/tcp").WithStartupTimeout(30*time.Second),
		),
	)
	skipIfNoDocker(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatalf("mailpit host: %v", err)
	}
	smtpPort, err := ctr.MappedPort(ctx, "1025/tcp")
	if err != nil {
		t.Fatalf("mailpit smtp port: %v", err)
	}
	apiPort, err := ctr.MappedPort(ctx, "8025/tcp")
	if err != nil {
		t.Fatalf("mailpit api port: %v", err)
	}
	apiBase := fmt.Sprintf("http://%s:%s", host, apiPort.Port())
	return host, smtpPort.Int(), apiBase
}
```

- [ ] **Step 3: Write a smoke test for the helpers**

Create `internal/testing/containers/containers_test.go`:
```go
//go:build integration

package containers

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestStartPostgres(t *testing.T) {
	url := StartPostgres(t)
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{})
	require.NoError(t, err)
	var one int
	require.NoError(t, db.Raw("SELECT 1").Scan(&one).Error)
	require.Equal(t, 1, one)
}

func TestStartRedis(t *testing.T) {
	url := StartRedis(t)
	opts, err := redis.ParseURL(url)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	defer client.Close()
	require.NoError(t, client.Ping(t.Context()).Err())
}

func TestStartMailpit(t *testing.T) {
	host, port, apiBase := StartMailpit(t)
	require.NotEmpty(t, host)
	require.Positive(t, port)
	require.Contains(t, apiBase, "http://")
}
```

- [ ] **Step 4: Create the findings doc stub**

`docs/superpowers/` is gitignored, so the doc must be force-added (the prior round's findings doc is tracked the same way). Create `docs/superpowers/specs/2026-06-25-external-node-e2e-findings.md`:
```markdown
# External-Service Node E2E — Findings

Bugs found and fixed while implementing `2026-06-25-external-node-e2e.md`.
One entry per bug: node, symptom, root cause, fix, guarding test.

_(none yet)_
```
Append to this file as bugs are found in Tasks 2–5; it is force-added and committed in Task 6.

- [ ] **Step 5: Add the Makefile target**

In `Makefile`, add:
```makefile
.PHONY: test-integration
test-integration:
	go test -tags=integration ./...
```

- [ ] **Step 6: Run the helper tests**

Run: `go test -tags=integration ./internal/testing/containers/... -v`
Expected: PASS for all three (or SKIP with a clear message if Docker is down). Also confirm the default build is clean: `go build ./...` and `go vet ./...` succeed (testcontainers must not leak into the non-tagged build).

- [ ] **Step 7: Commit**

```bash
go mod tidy
git add go.mod go.sum Makefile internal/testing/containers/
git add -f docs/superpowers/specs/2026-06-25-external-node-e2e-findings.md
git commit -m "test(e2e): add testcontainers helpers for external-service tests"
```

---

### Task 2: `db.*` end-to-end against real Postgres

**Files:**
- Create: `plugins/db/engine_e2e_integration_test.go`
- Possibly modify: `plugins/db/*.go` (only if a bug is found)
- Possibly modify: `docs/superpowers/specs/2026-06-25-external-node-e2e-findings.md`

**Interfaces:**
- Consumes: `containers.StartPostgres(t) string`; `engine.Compile`, `engine.NewExecutionContext`, `engine.WithInput`, `engine.ExecuteGraph`, `engine.WorkflowConfig{ID, Nodes}`, `engine.NodeConfig{Type, Config, Services}`; `registry.NewServiceRegistry`, `registry.NewNodeRegistry`, `(*ServiceRegistry).Register(name, instance, plugin)`, `(*NodeRegistry).RegisterFromPlugin(plugin)`; `(*ExecutionContextImpl).GetOutput(nodeID) (any, bool)`.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the e2e test (CRUD round-trip + error path)**

Create `plugins/db/engine_e2e_integration_test.go`:
```go
//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupDB starts Postgres, builds the service + registries, and creates a fresh
// table unique to the test.
func setupDB(t *testing.T, table string) (*registry.ServiceRegistry, *registry.NodeRegistry, *gorm.DB) {
	t.Helper()
	url := containers.StartPostgres(t)

	svc, err := (&Plugin{}).CreateService(map[string]any{"driver": "postgres", "url": url})
	require.NoError(t, err)
	gdb := svc.(*gorm.DB)
	require.NoError(t, gdb.Exec(
		"CREATE TABLE "+table+" (id serial PRIMARY KEY, name text, email text UNIQUE)").Error)
	t.Cleanup(func() { gdb.Exec("DROP TABLE IF EXISTS " + table) })

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("db", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))
	return svcReg, nodeReg, gdb
}

func runNode(t *testing.T, svcReg *registry.ServiceRegistry, nodeReg *registry.NodeRegistry,
	wf engine.WorkflowConfig, input map[string]any) *engine.ExecutionContextImpl {
	t.Helper()
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(input))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
	return execCtx
}

func TestDBCreateAndFind_Engine(t *testing.T) {
	svcReg, nodeReg, gdb := setupDB(t, "users_create")

	createWF := engine.WorkflowConfig{
		ID: "db-create",
		Nodes: map[string]engine.NodeConfig{
			"c": {
				Type:     "db.create",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_create",
					"data":  map[string]any{"name": "Alice", "email": "alice@example.com"},
				},
			},
		},
	}
	execCtx := runNode(t, svcReg, nodeReg, createWF, nil)
	out, ok := execCtx.GetOutput("c")
	require.True(t, ok)
	row := out.(map[string]any)
	assert.Equal(t, "Alice", row["name"])
	assert.NotNil(t, row["id"])

	// Effect asserted directly against Postgres.
	var count int64
	require.NoError(t, gdb.Table("users_create").Where("email = ?", "alice@example.com").Count(&count).Error)
	assert.Equal(t, int64(1), count)

	findWF := engine.WorkflowConfig{
		ID: "db-find",
		Nodes: map[string]engine.NodeConfig{
			"f": {
				Type:     "db.find",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_create",
					"where": map[string]any{"email": "alice@example.com"},
				},
			},
		},
	}
	execCtx2 := runNode(t, svcReg, nodeReg, findWF, nil)
	fout, ok := execCtx2.GetOutput("f")
	require.True(t, ok)
	rows := fout.([]any)
	require.Len(t, rows, 1)
	assert.Equal(t, "Alice", rows[0].(map[string]any)["name"])
}

func TestDBUpdateCountDelete_Engine(t *testing.T) {
	svcReg, nodeReg, gdb := setupDB(t, "users_mut")
	require.NoError(t, gdb.Exec(
		"INSERT INTO users_mut (name, email) VALUES ('Bob','bob@example.com')").Error)

	updateWF := engine.WorkflowConfig{
		ID: "db-update",
		Nodes: map[string]engine.NodeConfig{
			"u": {
				Type:     "db.update",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_mut",
					"data":  map[string]any{"name": "Bobby"},
					"where": map[string]any{"email": "bob@example.com"},
				},
			},
		},
	}
	runNode(t, svcReg, nodeReg, updateWF, nil)
	var name string
	require.NoError(t, gdb.Table("users_mut").Select("name").Where("email = ?", "bob@example.com").Scan(&name).Error)
	assert.Equal(t, "Bobby", name)

	countWF := engine.WorkflowConfig{
		ID: "db-count",
		Nodes: map[string]engine.NodeConfig{
			"n": {
				Type:     "db.count",
				Services: map[string]string{"database": "db"},
				Config:   map[string]any{"table": "users_mut"},
			},
		},
	}
	cctx := runNode(t, svcReg, nodeReg, countWF, nil)
	cout, ok := cctx.GetOutput("n")
	require.True(t, ok)
	assert.EqualValues(t, 1, toInt(cout))

	deleteWF := engine.WorkflowConfig{
		ID: "db-delete",
		Nodes: map[string]engine.NodeConfig{
			"d": {
				Type:     "db.delete",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_mut",
					"where": map[string]any{"email": "bob@example.com"},
				},
			},
		},
	}
	runNode(t, svcReg, nodeReg, deleteWF, nil)
	var remaining int64
	require.NoError(t, gdb.Table("users_mut").Count(&remaining).Error)
	assert.Equal(t, int64(0), remaining)
}

func TestDBUpsertAndFindOne_Engine(t *testing.T) {
	svcReg, nodeReg, gdb := setupDB(t, "users_up")

	upsertWF := engine.WorkflowConfig{
		ID: "db-upsert",
		Nodes: map[string]engine.NodeConfig{
			"u": {
				Type:     "db.upsert",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table":    "users_up",
					"data":     map[string]any{"name": "Cara", "email": "cara@example.com"},
					"conflict": "email",
				},
			},
		},
	}
	runNode(t, svcReg, nodeReg, upsertWF, nil)
	// Second upsert with same conflict key updates rather than duplicating.
	upsertWF.Nodes["u"].Config["data"] = map[string]any{"name": "Cara2", "email": "cara@example.com"}
	runNode(t, svcReg, nodeReg, upsertWF, nil)

	var count int64
	require.NoError(t, gdb.Table("users_up").Count(&count).Error)
	assert.Equal(t, int64(1), count)

	findOneWF := engine.WorkflowConfig{
		ID: "db-findone",
		Nodes: map[string]engine.NodeConfig{
			"f": {
				Type:     "db.findOne",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_up",
					"where": map[string]any{"email": "cara@example.com"},
				},
			},
		},
	}
	fctx := runNode(t, svcReg, nodeReg, findOneWF, nil)
	fout, ok := fctx.GetOutput("f")
	require.True(t, ok)
	assert.Equal(t, "Cara2", fout.(map[string]any)["name"])
}

// toInt normalizes count output which may be int64/float64/int depending on the node.
func toInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case map[string]any:
		return toInt(n["count"])
	default:
		return -1
	}
}

func TestDBCreate_MissingTable_Errors(t *testing.T) {
	url := containers.StartPostgres(t)
	svc, err := (&Plugin{}).CreateService(map[string]any{"driver": "postgres", "url": url})
	require.NoError(t, err)
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("db", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "db-err",
		Nodes: map[string]engine.NodeConfig{
			"c": {
				Type:     "db.create",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "does_not_exist",
					"data":  map[string]any{"name": "X"},
				},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err) // unknown table → workflow fails, no panic
}
```

- [ ] **Step 2: Run the test**

Run: `go test -tags=integration ./plugins/db/ -run Engine -v`
Expected: PASS. If `db.count`/`db.find` output shape differs from the asserted form (`toInt` / `[]any`), inspect the actual output, adjust the assertion to the documented shape, and if the node's behavior is wrong rather than merely differently-shaped, fix the node and log a finding.

- [ ] **Step 3: Commit**

```bash
git add plugins/db/engine_e2e_integration_test.go
# If a bug was fixed, also stage the changed plugin file(s) and force-add the findings doc:
#   git add plugins/db/<fixed>.go && git add -f docs/superpowers/specs/2026-06-25-external-node-e2e-findings.md
git commit -m "test(e2e): drive db.* through the engine against real Postgres"
```

---

### Task 3: `cache.*` end-to-end against real Redis

**Files:**
- Create: `plugins/cache/engine_e2e_integration_test.go`
- Possibly modify: `plugins/cache/*.go` (only if a bug is found)

**Interfaces:**
- Consumes: `containers.StartRedis(t) string`; same engine/registry API as Task 2; `plugin.RedisClientProvider` (`Client() *redis.Client`) implemented by the cache service.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the e2e test (set/exists/del + TTL + error path)**

Create `plugins/cache/engine_e2e_integration_test.go`:
```go
//go:build integration

package cache

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCache(t *testing.T) (*registry.ServiceRegistry, *registry.NodeRegistry, any) {
	t.Helper()
	url := containers.StartRedis(t)
	svc, err := (&Plugin{}).CreateService(map[string]any{"url": url})
	require.NoError(t, err)
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("cache", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))
	return svcReg, nodeReg, svc
}

func run(t *testing.T, svcReg *registry.ServiceRegistry, nodeReg *registry.NodeRegistry,
	wf engine.WorkflowConfig) *engine.ExecutionContextImpl {
	t.Helper()
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
	return execCtx
}

func TestCacheSetExistsDel_Engine(t *testing.T) {
	svcReg, nodeReg, svc := setupCache(t)
	rc := svc.(plugin.RedisClientProvider).Client()
	ctx := context.Background()

	setWF := engine.WorkflowConfig{
		ID: "cache-set",
		Nodes: map[string]engine.NodeConfig{
			"s": {
				Type:     "cache.set",
				Services: map[string]string{"cache": "cache"},
				Config:   map[string]any{"key": "greeting", "value": "hello", "ttl": 60},
			},
		},
	}
	run(t, svcReg, nodeReg, setWF)

	// Effect + TTL asserted directly against Redis.
	got, err := rc.Get(ctx, "greeting").Result()
	require.NoError(t, err)
	assert.Equal(t, "hello", got)
	ttl, err := rc.TTL(ctx, "greeting").Result()
	require.NoError(t, err)
	assert.Positive(t, ttl)

	existsWF := engine.WorkflowConfig{
		ID: "cache-exists",
		Nodes: map[string]engine.NodeConfig{
			"e": {
				Type:     "cache.exists",
				Services: map[string]string{"cache": "cache"},
				Config:   map[string]any{"key": "greeting"},
			},
		},
	}
	ectx := run(t, svcReg, nodeReg, existsWF)
	eout, ok := ectx.GetOutput("e")
	require.True(t, ok)
	assert.Equal(t, true, normalizeBool(eout))

	delWF := engine.WorkflowConfig{
		ID: "cache-del",
		Nodes: map[string]engine.NodeConfig{
			"d": {
				Type:     "cache.del",
				Services: map[string]string{"cache": "cache"},
				Config:   map[string]any{"key": "greeting"},
			},
		},
	}
	run(t, svcReg, nodeReg, delWF)
	n, err := rc.Exists(ctx, "greeting").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

// normalizeBool unwraps cache.exists output which may be a bool or {"exists": bool}.
func normalizeBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case map[string]any:
		if x, ok := b["exists"].(bool); ok {
			return x
		}
	}
	return false
}

func TestCacheSet_MissingService_Errors(t *testing.T) {
	url := containers.StartRedis(t)
	_, err := (&Plugin{}).CreateService(map[string]any{"url": url})
	require.NoError(t, err)

	// Register no service so the required dep is unmet.
	svcReg := registry.NewServiceRegistry()
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "cache-noservice",
		Nodes: map[string]engine.NodeConfig{
			"s": {
				Type:     "cache.set",
				Services: map[string]string{"cache": "missing"},
				Config:   map[string]any{"key": "k", "value": "v"},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run the test**

Run: `go test -tags=integration ./plugins/cache/ -run Engine -v`
Expected: PASS. If `cache.exists` output is shaped differently, adjust `normalizeBool` to the actual documented shape; if the missing-service case panics instead of erroring, fix the node and log a finding.

- [ ] **Step 3: Commit**

```bash
git add plugins/cache/engine_e2e_integration_test.go
# If a bug was fixed, also stage the changed plugin file(s) and force-add the findings doc.
git commit -m "test(e2e): drive cache.* through the engine against real Redis"
```

---

### Task 4: `event.emit` (stream + pubsub) end-to-end against real Redis

**Files:**
- Create: `plugins/core/event/engine_e2e_integration_test.go`
- Possibly modify: `plugins/core/event/*.go`, `plugins/stream/*.go`, `plugins/pubsub/*.go` (only if a bug is found)

**Interfaces:**
- Consumes: `containers.StartRedis(t) string`; the `stream` and `pubsub` plugins' `CreateService(map[string]any{"url": url})`; `plugin.RedisClientProvider` on both services; same engine/registry API as Task 2.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the e2e test (stream mode + pubsub mode + error path)**

Create `plugins/core/event/engine_e2e_integration_test.go`:
```go
//go:build integration

package event

import (
	"context"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/testing/containers"
	pubsubplugin "github.com/chimpanze/noda/plugins/pubsub"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emitRegistries(t *testing.T, url string) (*registry.ServiceRegistry, *registry.NodeRegistry, *redis.Client) {
	t.Helper()
	streamSvc, err := (&streamplugin.Plugin{}).CreateService(map[string]any{"url": url})
	require.NoError(t, err)
	pubsubSvc, err := (&pubsubplugin.Plugin{}).CreateService(map[string]any{"url": url})
	require.NoError(t, err)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("stream", streamSvc, nil))
	require.NoError(t, svcReg.Register("pubsub", pubsubSvc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	rc := streamSvc.(plugin.RedisClientProvider).Client()
	return svcReg, nodeReg, rc
}

func emit(t *testing.T, svcReg *registry.ServiceRegistry, nodeReg *registry.NodeRegistry,
	wf engine.WorkflowConfig) {
	t.Helper()
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
}

func TestEventEmit_Stream_Engine(t *testing.T) {
	url := containers.StartRedis(t)
	svcReg, nodeReg, rc := emitRegistries(t, url)

	wf := engine.WorkflowConfig{
		ID: "emit-stream",
		Nodes: map[string]engine.NodeConfig{
			"e": {
				Type:     "event.emit",
				Services: map[string]string{"stream": "stream", "pubsub": "pubsub"},
				Config: map[string]any{
					"mode":    "stream",
					"topic":   "orders",
					"payload": map[string]any{"id": "42"},
				},
			},
		},
	}
	emit(t, svcReg, nodeReg, wf)

	// Effect asserted directly: the message is on the stream.
	msgs, err := rc.XRange(context.Background(), "orders", "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, msgs, 1)
}

func TestEventEmit_PubSub_Engine(t *testing.T) {
	url := containers.StartRedis(t)
	svcReg, nodeReg, rc := emitRegistries(t, url)

	sub := rc.Subscribe(context.Background(), "alerts")
	defer sub.Close()
	_, err := sub.Receive(context.Background()) // wait for subscription confirmation
	require.NoError(t, err)
	ch := sub.Channel()

	wf := engine.WorkflowConfig{
		ID: "emit-pubsub",
		Nodes: map[string]engine.NodeConfig{
			"e": {
				Type:     "event.emit",
				Services: map[string]string{"stream": "stream", "pubsub": "pubsub"},
				Config: map[string]any{
					"mode":    "pubsub",
					"topic":   "alerts",
					"payload": map[string]any{"level": "warn"},
				},
			},
		},
	}
	emit(t, svcReg, nodeReg, wf)

	select {
	case msg := <-ch:
		assert.Contains(t, msg.Payload, "warn")
	case <-time.After(5 * time.Second):
		t.Fatal("did not receive pubsub message")
	}
}

func TestEventEmit_BadMode_Errors(t *testing.T) {
	url := containers.StartRedis(t)
	svcReg, nodeReg, _ := emitRegistries(t, url)

	wf := engine.WorkflowConfig{
		ID: "emit-bad",
		Nodes: map[string]engine.NodeConfig{
			"e": {
				Type:     "event.emit",
				Services: map[string]string{"stream": "stream", "pubsub": "pubsub"},
				Config: map[string]any{
					"mode":    "carrier-pigeon",
					"topic":   "x",
					"payload": map[string]any{"a": 1},
				},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run the test**

Run: `go test -tags=integration ./plugins/core/event/ -run Engine -v`
Expected: PASS. If the pubsub test is flaky on timing, confirm the subscription is established (the `sub.Receive` call) before `emit`; if `event.emit` does not surface an error for an unknown mode, fix `emit.go`'s switch default and log a finding.

- [ ] **Step 3: Commit**

```bash
git add plugins/core/event/engine_e2e_integration_test.go
# If a bug was fixed, also stage the changed plugin file(s) and force-add the findings doc.
git commit -m "test(e2e): drive event.emit (stream+pubsub) against real Redis"
```

---

### Task 5: `email.send` end-to-end against Mailpit

**Files:**
- Create: `plugins/email/engine_e2e_integration_test.go`
- Possibly modify: `plugins/email/*.go` (only if a bug is found)

**Interfaces:**
- Consumes: `containers.StartMailpit(t) (host string, port int, apiBase string)`; the email plugin's `CreateService(map[string]any{"host", "port", "from"})`; same engine/registry API as Task 2; Mailpit HTTP API `GET {apiBase}/api/v1/messages`.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the e2e test (send + assert via Mailpit API + error path)**

Create `plugins/email/engine_e2e_integration_test.go`:
```go
//go:build integration

package email

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mailpitMessages struct {
	Total    int `json:"total"`
	Messages []struct {
		Subject string `json:"Subject"`
		To      []struct {
			Address string `json:"Address"`
		} `json:"To"`
	} `json:"messages"`
}

func fetchMessages(t *testing.T, apiBase string) mailpitMessages {
	t.Helper()
	var out mailpitMessages
	// Poll briefly; SMTP delivery is async relative to the HTTP API.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(apiBase + "/api/v1/messages")
		require.NoError(t, err)
		func() {
			defer resp.Body.Close()
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		}()
		if out.Total > 0 {
			return out
		}
		time.Sleep(100 * time.Millisecond)
	}
	return out
}

func TestEmailSend_Engine(t *testing.T) {
	host, port, apiBase := containers.StartMailpit(t)

	svc, err := (&Plugin{}).CreateService(map[string]any{
		"host": host,
		"port": port,
		"from": "noda@test.local",
	})
	require.NoError(t, err)
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("mailer", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "email-send",
		Nodes: map[string]engine.NodeConfig{
			"m": {
				Type:     "email.send",
				Services: map[string]string{"mailer": "mailer"},
				Config: map[string]any{
					"to":      "recipient@example.com",
					"subject": "Noda E2E",
					"body":    "hello from noda",
				},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))

	msgs := fetchMessages(t, apiBase)
	require.Equal(t, 1, msgs.Total)
	assert.Equal(t, "Noda E2E", msgs.Messages[0].Subject)
	require.NotEmpty(t, msgs.Messages[0].To)
	assert.Equal(t, "recipient@example.com", msgs.Messages[0].To[0].Address)
}

func TestEmailSend_UnreachableHost_Errors(t *testing.T) {
	svc, err := (&Plugin{}).CreateService(map[string]any{
		"host": "127.0.0.1",
		"port": 1, // nothing listening
		"from": "noda@test.local",
	})
	require.NoError(t, err)
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("mailer", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "email-err",
		Nodes: map[string]engine.NodeConfig{
			"m": {
				Type:     "email.send",
				Services: map[string]string{"mailer": "mailer"},
				Config: map[string]any{
					"to":      "x@example.com",
					"subject": "fail",
					"body":    "fail",
				},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run the test**

Run: `go test -tags=integration ./plugins/email/ -run Engine -v`
Expected: PASS. If `email.send` fails to connect to Mailpit because the service forces STARTTLS/TLS (see `plugins/email/service.go` dial logic), that is a real production limitation for plaintext SMTP servers — fix the service to support a non-TLS path for the configured port (or add a config flag), log a finding, and re-run. If the Mailpit JSON field names differ, adjust the `mailpitMessages` struct tags to match the actual API response.

- [ ] **Step 3: Commit**

```bash
git add plugins/email/engine_e2e_integration_test.go
# If a bug was fixed, also stage the changed plugin file(s) and force-add the findings doc.
git commit -m "test(e2e): drive email.send through the engine against Mailpit"
```

---

### Task 6: CI integration job + findings summary

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify/Create: `docs/superpowers/specs/2026-06-25-external-node-e2e-findings.md`

**Interfaces:**
- Consumes: `make test-integration` from Task 1.
- Produces: a CI job named `integration` and a complete findings document.

- [ ] **Step 1: Add the integration job**

In `.github/workflows/ci.yml`, add a job alongside the existing `go` job (match the existing checkout/setup-go steps; GitHub-hosted Ubuntu runners ship a running Docker daemon, so testcontainers needs no extra service):
```yaml
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Install libvips
        run: sudo apt-get update && sudo apt-get install -y libvips-dev
      - name: Run integration e2e tests
        run: make test-integration
```
(Drop the libvips step if the integration packages don't pull in the image plugin; keep it only if the build requires it.)

- [ ] **Step 2: Run the full tagged suite locally**

Run: `make test-integration`
Expected: All external-service e2e tests PASS (or SKIP cleanly without Docker). Confirm no regressions in the default suite: `go test ./...` (untagged) still passes.

- [ ] **Step 3: Finalize the findings document**

Ensure `docs/superpowers/specs/2026-06-25-external-node-e2e-findings.md` exists and contains, for each bug found during Tasks 2–5: node, symptom, root cause, fix, and the guarding test. If no bugs were found, state that explicitly with the list of nodes verified.

- [ ] **Step 4: Commit**

```bash
git add -f .github/workflows/ci.yml docs/superpowers/specs/2026-06-25-external-node-e2e-findings.md
git commit -m "ci: run external-service e2e integration suite; add findings summary"
```

---

## Definition of Done

- `db.create/find/findOne/count/update/upsert/delete`, `cache.set/del/exists`, `event.emit` (stream + pubsub), and `email.send` each have happy-path + error/edge coverage driven through `engine.ExecuteGraph` against a real container.
- `internal/testing/containers/` helpers exist and are reused by every plugin test.
- `make test-integration` passes with Docker running; default `go test ./...` is unaffected and pulls no new dependency into the production build path.
- All bugs found are fixed and recorded in `2026-06-25-external-node-e2e-findings.md`.
- New CI `integration` job runs the tagged suite.
