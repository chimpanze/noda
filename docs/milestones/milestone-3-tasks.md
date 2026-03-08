# Milestone 3: Plugin System and Service Registry — Task Breakdown

**Depends on:** Milestone 0 (`pkg/api/` interfaces), Milestone 1 (config loading)
**Result:** Plugins register by prefix, service instances are created and tracked, startup validation catches missing plugins, missing services, and prefix mismatches.

---

## Task 3.1: Plugin Registry

**Description:** Central registry where plugins register themselves by prefix.

**Subtasks:**

- [ ] Create `internal/registry/plugins.go`
- [ ] Implement `PluginRegistry` struct with:
  - `Register(plugin api.Plugin) error` — registers a plugin, returns error on duplicate prefix
  - `Get(prefix string) (api.Plugin, bool)` — look up plugin by prefix
  - `All() []api.Plugin` — list all registered plugins
  - `Prefixes() []string` — list all registered prefixes
- [ ] Duplicate prefix detection: if two plugins register the same prefix, return a clear error with both plugin names
- [ ] Thread-safe: plugins are registered during init, read during execution

**Tests:**
- [ ] Register plugin, retrieve by prefix
- [ ] Register multiple plugins with different prefixes
- [ ] Duplicate prefix produces error naming both plugins
- [ ] Get non-existent prefix returns false
- [ ] All() returns all registered plugins

**Acceptance criteria:** Plugins register by prefix with duplicate detection.

---

## Task 3.2: Node Registry

**Description:** Registry for node type descriptors and executor factories, populated from plugins.

**Subtasks:**

- [ ] Create `internal/registry/nodes.go`
- [ ] Implement `NodeRegistry` struct with:
  - `RegisterFromPlugin(plugin api.Plugin)` — reads `plugin.Nodes()`, registers each under `prefix.name` (e.g., `db.query`)
  - `GetDescriptor(nodeType string) (api.NodeDescriptor, bool)` — look up by full type
  - `GetFactory(nodeType string) (func(map[string]any) api.NodeExecutor, bool)` — look up factory by full type
  - `AllTypes() []string` — list all registered node types
  - `TypesByPrefix(prefix string) []string` — list node types for a prefix
- [ ] Node type format is always `prefix.name` — validated on registration
- [ ] Duplicate node type is an error

**Tests:**
- [ ] Register nodes from a plugin, retrieve by full type
- [ ] GetDescriptor returns correct descriptor
- [ ] GetFactory returns working factory
- [ ] AllTypes lists all registered types
- [ ] TypesByPrefix filters correctly
- [ ] Duplicate type produces error

**Acceptance criteria:** Node types are registered and retrievable by their full `prefix.name` identifier.

---

## Task 3.3: Service Registry

**Description:** Central registry for all initialized service instances.

**Subtasks:**

- [ ] Create `internal/registry/services.go`
- [ ] Implement `ServiceRegistry` struct with:
  - `Register(name string, instance any, plugin api.Plugin) error` — stores instance with its owning plugin
  - `Get(name string) (any, bool)` — look up instance by name
  - `GetWithPlugin(name string) (any, api.Plugin, bool)` — look up instance and its owning plugin
  - `GetPrefix(name string) (string, bool)` — look up which prefix a service belongs to
  - `All() map[string]any` — all instances
  - `ByPrefix(prefix string) map[string]any` — instances for a specific prefix
- [ ] Duplicate instance name is an error
- [ ] Each instance tracks which plugin created it (for prefix validation)

**Tests:**
- [ ] Register instance, retrieve by name
- [ ] GetPrefix returns correct plugin prefix
- [ ] ByPrefix filters correctly
- [ ] Duplicate name produces error
- [ ] Get non-existent name returns false

**Acceptance criteria:** Service instances are tracked with their owning plugin for prefix validation.

---

## Task 3.4: Service Instance Lifecycle

**Description:** Create, health-check, and shut down service instances from config.

**Subtasks:**

- [ ] Create `internal/registry/lifecycle.go`
- [ ] Implement `InitializeServices(config map[string]any, plugins *PluginRegistry) (*ServiceRegistry, []error)`:
  - Read `services` map from root config
  - For each service: find the plugin by `plugin` field, call `CreateService(config)`, register in ServiceRegistry
  - If a service references an unknown plugin name → error
  - If `CreateService` fails → collect error, continue with remaining services
  - Return registry and any errors
- [ ] Implement `HealthCheckAll(registry *ServiceRegistry) []error`:
  - For each service: call owning plugin's `HealthCheck(instance)`
  - Collect all failures
- [ ] Implement `ShutdownAll(registry *ServiceRegistry) []error`:
  - For each service: call owning plugin's `Shutdown(instance)`
  - Shut down in reverse initialization order
  - Collect all failures but continue shutting down remaining

**Tests:**
- [ ] Initialize services from config with a test plugin
- [ ] Unknown plugin name produces error
- [ ] CreateService failure is collected, other services still initialize
- [ ] HealthCheck passes for healthy services
- [ ] HealthCheck fails for unhealthy services
- [ ] Shutdown calls Shutdown on all services

**Acceptance criteria:** Service instances are created from config and managed through their full lifecycle.

---

## Task 3.5: Internal Service Registration

**Description:** Auto-register internal services from connection endpoints and Wasm runtimes.

**Subtasks:**

- [ ] Create `internal/registry/internal.go`
- [ ] Implement `RegisterInternalServices(config *ResolvedConfig, registry *ServiceRegistry)`:
  - For each connection endpoint: register under `ws` or `sse` prefix based on `type` field
  - For each Wasm runtime: register under `wasm` prefix
  - These are placeholder registrations — the actual service implementations are created in their respective milestones. For now, register a marker that the name exists and its prefix, so validation passes.
- [ ] Internal services use the same registry as external services — no special cases

**Tests:**
- [ ] Connection endpoint registers under correct prefix
- [ ] Wasm runtime registers under `wasm` prefix
- [ ] Internal service names conflict with external service names → error
- [ ] Registry lookup finds internal services

**Acceptance criteria:** Internal services are registered and validateable alongside external services.

---

## Task 3.6: Startup Validator

**Description:** Full startup validation that checks all plugin/service/node references.

**Subtasks:**

- [ ] Create `internal/registry/validator.go`
- [ ] Implement `ValidateStartup(config *ResolvedConfig, plugins *PluginRegistry, services *ServiceRegistry, nodes *NodeRegistry) []error`
- [ ] Validation steps:
  1. Every node `type` in every workflow has a prefix that matches a registered plugin
  2. Every `services` slot in every node references an existing service instance
  3. Every service instance's prefix matches the slot's required prefix (from `ServiceDeps()`)
  4. Required slots are filled; optional slots that are filled are valid
  5. `event.emit` nodes: validate that the mode's matching slot is filled
- [ ] Collect all errors with workflow ID, node ID, and specific message
- [ ] This runs after config validation (M1) and after plugin/service initialization

**Tests:**
- [ ] Valid config passes
- [ ] Unknown node type prefix → error naming the type and workflow
- [ ] Missing service reference → error naming the slot, node, and workflow
- [ ] Wrong prefix on service slot → error (e.g., cache service in a database slot)
- [ ] Missing required slot → error
- [ ] Optional slot unfilled → no error
- [ ] Multiple errors across multiple workflows all collected

**Acceptance criteria:** Every plugin, service, and node reference is validated at startup.

---

## Task 3.7: Test Plugin

**Description:** A simple in-memory key-value plugin for testing the full lifecycle.

**Subtasks:**

- [ ] Create `internal/registry/testplugin_test.go` (test-only, not shipped)
- [ ] Implement a `testKVPlugin` that:
  - Name: `"test-kv"`, Prefix: `"kv"`
  - HasServices: true
  - CreateService: creates an in-memory map
  - HealthCheck: always passes
  - Shutdown: clears the map
  - Nodes: `kv.get` (reads from map), `kv.set` (writes to map)
- [ ] Implement `kvGetExecutor` and `kvSetExecutor` satisfying `api.NodeExecutor`
- [ ] Use this plugin in all registry integration tests

**Tests:**
- [ ] Full lifecycle: register plugin → create service → register nodes → validate → health check → shutdown
- [ ] Execute kv.get and kv.set nodes through the registry

**Acceptance criteria:** Test plugin exercises the complete plugin/service/node lifecycle.

---

## Task 3.8: Wire into `noda validate`

**Description:** Extend `noda validate` to include plugin and service validation.

**Subtasks:**

- [ ] Update `ValidateAll()` pipeline to include:
  1. (existing) Config file validation
  2. (new) Initialize plugin registry with all built-in plugins
  3. (new) Initialize services from config
  4. (new) Register internal services
  5. (new) Run startup validation
- [ ] Note: at this milestone, only the test plugin and core node plugins (which will be stubs for now) are registered. Real plugins come in later milestones. The validation framework is ready.
- [ ] `noda validate` now catches service reference errors in addition to config schema errors

**Tests:**
- [ ] `noda validate` on project with invalid service reference → error
- [ ] `noda validate` on project with unknown node type → error
- [ ] `noda validate` on valid project → passes (with stub plugins)

**Acceptance criteria:** `noda validate` runs the full startup validation pipeline.
