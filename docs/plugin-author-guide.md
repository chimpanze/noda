# Plugin Author Guide

This guide covers building custom Noda plugins that provide new node types and services.

## Overview

A Noda plugin is a Go package that implements the `api.Plugin` interface. Plugins provide:

- **Node types** — operations that execute within workflows
- **Services** — connections to external systems (databases, APIs, etc.)

## Plugin Interface

```go
package api

type Plugin interface {
    // Name returns the plugin's display name.
    Name() string

    // Prefix returns the node type prefix (e.g., "db" for "db.query").
    Prefix() string

    // Nodes returns the plugin's node registrations.
    Nodes() []NodeRegistration

    // HasServices returns true if the plugin manages services.
    HasServices() bool

    // CreateService initializes a service instance from config.
    CreateService(name string, config map[string]any) (any, error)

    // HealthCheck verifies a service instance is healthy.
    HealthCheck(service any) error

    // Shutdown cleans up a service instance.
    Shutdown(service any) error
}
```

## Minimal Plugin Example

Here's a complete plugin that provides a single node type:

```go
package myplugin

import (
    "context"
    "github.com/your-org/noda/pkg/api"
)

// Plugin implements api.Plugin.
type Plugin struct{}

func (p *Plugin) Name() string   { return "My Plugin" }
func (p *Plugin) Prefix() string { return "my" }

func (p *Plugin) HasServices() bool                                { return false }
func (p *Plugin) CreateService(name string, config map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(service any) error                    { return nil }
func (p *Plugin) Shutdown(service any) error                       { return nil }

func (p *Plugin) Nodes() []api.NodeRegistration {
    return []api.NodeRegistration{
        {Descriptor: &GreetDescriptor{}, Factory: greetFactory},
    }
}
```

## Node Descriptor

A `NodeDescriptor` describes a node type's metadata and config schema.

```go
type NodeDescriptor interface {
    // Name returns the node type name (without prefix).
    // Combined with plugin prefix: "my.greet"
    Name() string

    // ServiceDeps declares required/optional service slots.
    ServiceDeps() []ServiceDep

    // ConfigSchema returns a JSON Schema for the node's config.
    ConfigSchema() map[string]any
}
```

### Example Descriptor

```go
type GreetDescriptor struct{}

func (d *GreetDescriptor) Name() string { return "greet" }

func (d *GreetDescriptor) ServiceDeps() []ServiceDep {
    return nil // No services needed
}

func (d *GreetDescriptor) ConfigSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "name": map[string]any{
                "type":        "string",
                "description": "Name to greet",
            },
        },
        "required": []string{"name"},
    }
}
```

## Node Factory and Executor

The **factory** creates a node executor from the resolved config. It runs once at workflow load time — use it for validation and pre-computation.

The **executor** runs each time the node is invoked in a workflow.

```go
type NodeExecutor interface {
    // Outputs returns the node's possible output names.
    Outputs() []string

    // Execute runs the node logic.
    // Returns: (output name, result data, error)
    Execute(ctx context.Context, nCtx api.ExecutionContext,
        config map[string]any, services map[string]any) (string, any, error)
}
```

### Example Factory and Executor

```go
func greetFactory(config map[string]any) (api.NodeExecutor, error) {
    // Validate config at load time
    nameExpr, ok := config["name"].(string)
    if !ok {
        return nil, fmt.Errorf("name is required")
    }
    return &GreetExecutor{nameExpr: nameExpr}, nil
}

type GreetExecutor struct {
    nameExpr string
}

func (e *GreetExecutor) Outputs() []string {
    return api.DefaultOutputs() // ["success", "error"]
}

func (e *GreetExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext,
    config map[string]any, services map[string]any) (string, any, error) {

    // Resolve expression
    name, err := nCtx.Resolve(e.nameExpr)
    if err != nil {
        return api.OutputError, nil, err
    }

    greeting := fmt.Sprintf("Hello, %s!", name)
    return api.OutputSuccess, map[string]any{"greeting": greeting}, nil
}
```

## ExecutionContext

The `ExecutionContext` provides access to workflow data and expression resolution.

```go
type ExecutionContext interface {
    // Input returns the workflow input data.
    Input() map[string]any

    // Auth returns authentication data.
    Auth() *AuthData

    // Trigger returns trigger metadata.
    Trigger() *TriggerData

    // Resolve evaluates an expression string.
    Resolve(expr string) (any, error)

    // ResolveWithVars evaluates an expression with additional variables.
    ResolveWithVars(expr string, vars map[string]any) (any, error)

    // Log writes a structured log entry.
    Log(level, message string, fields map[string]any)
}
```

### AuthData

```go
type AuthData struct {
    UserID string
    Roles  []string
    Claims map[string]any
}
```

### TriggerData

```go
type TriggerData struct {
    Type      string    // "http", "event", "schedule", "websocket", "wasm"
    Timestamp time.Time
    TraceID   string
}
```

## Service Dependencies

Nodes can declare service dependencies through `ServiceDeps()`. Services are injected at runtime based on the workflow config.

```go
type ServiceDep struct {
    Prefix   string // Plugin prefix that provides the service (e.g., "db")
    Required bool   // Whether the service is mandatory
}
```

### Example: Node with Service Dependency

```go
type QueryDescriptor struct{}

func (d *QueryDescriptor) Name() string { return "query" }

func (d *QueryDescriptor) ServiceDeps() []ServiceDep {
    return []ServiceDep{
        {Prefix: "db", Required: true},
    }
}

func (d *QueryDescriptor) ConfigSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "sql": map[string]any{"type": "string"},
        },
        "required": []string{"sql"},
    }
}
```

The service is accessed in the executor via the `services` map:

```go
func (e *QueryExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext,
    config map[string]any, services map[string]any) (string, any, error) {

    db := services["database"] // Service slot name from workflow config
    // Use db...
}
```

In the workflow config, users bind service instances to slots:

```json
{
  "type": "my.query",
  "services": { "database": "postgres" },
  "config": { "sql": "SELECT 1" }
}
```

## Service Lifecycle

Plugins that manage services implement the full lifecycle:

```go
func (p *MyPlugin) HasServices() bool { return true }

func (p *MyPlugin) CreateService(name string, config map[string]any) (any, error) {
    // Initialize connection, validate config
    addr := config["addr"].(string)
    client, err := connectToService(addr)
    if err != nil {
        return nil, fmt.Errorf("failed to connect: %w", err)
    }
    return client, nil
}

func (p *MyPlugin) HealthCheck(service any) error {
    client := service.(*MyClient)
    return client.Ping()
}

func (p *MyPlugin) Shutdown(service any) error {
    client := service.(*MyClient)
    return client.Close()
}
```

Services are created at startup and shut down gracefully when the server stops.

## Custom Outputs

Nodes can define custom output names beyond the default `success`/`error`:

```go
func (e *MyExecutor) Outputs() []string {
    return []string{"found", "not_found", "error"}
}

func (e *MyExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext,
    config map[string]any, services map[string]any) (string, any, error) {

    result := lookupSomething()
    if result == nil {
        return "not_found", nil, nil
    }
    return "found", result, nil
}
```

In workflows, edges connect to these outputs:

```json
{
  "edges": [
    { "from": "lookup", "output": "found", "to": "process" },
    { "from": "lookup", "output": "not_found", "to": "handle-missing" }
  ]
}
```

## Error Types

Use Noda's built-in error types for proper HTTP status mapping:

```go
// 422 Unprocessable Entity
return api.OutputError, nil, &api.ValidationError{
    Field:   "email",
    Message: "invalid email format",
    Value:   email,
}

// 404 Not Found
return api.OutputError, nil, &api.NotFoundError{
    Resource: "user",
    ID:       userID,
}

// 409 Conflict
return api.OutputError, nil, &api.ConflictError{
    Resource: "user",
    Reason:   "email already exists",
}

// 503 Service Unavailable
return api.OutputError, nil, &api.ServiceUnavailableError{
    Service: "database",
    Cause:   err,
}

// 504 Timeout
return api.OutputError, nil, &api.TimeoutError{
    Duration:  5 * time.Second,
    Operation: "query",
}
```

## Service Interfaces

Plugins can implement standard service interfaces so other nodes can interact with them:

```go
// StorageService allows file operations
type StorageService interface {
    Read(ctx context.Context, path string) ([]byte, error)
    Write(ctx context.Context, path string, data []byte, contentType string) error
    Delete(ctx context.Context, path string) error
    List(ctx context.Context, prefix string) ([]string, error)
}

// CacheService allows key-value operations
type CacheService interface {
    Get(ctx context.Context, key string) (any, error)
    Set(ctx context.Context, key string, value any, ttl time.Duration) error
    Del(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
}

// StreamService allows event streaming
type StreamService interface {
    Publish(ctx context.Context, topic string, payload map[string]any) (string, error)
    Ack(ctx context.Context, topic, group, id string) error
}

// PubSubService allows pub/sub messaging
type PubSubService interface {
    Publish(ctx context.Context, topic string, payload map[string]any) error
}

// ConnectionService allows real-time communication
type ConnectionService interface {
    Send(ctx context.Context, channel string, data any) error
    SendSSE(ctx context.Context, channel string, event, id string, data any) error
}
```

## Config Schema

The `ConfigSchema()` method returns a JSON Schema that:

1. Validates node configuration at load time
2. Drives the visual editor's auto-generated config forms
3. Documents the node's configuration contract

```go
func (d *MyDescriptor) ConfigSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "url": map[string]any{
                "type":        "string",
                "description": "Target URL",
            },
            "method": map[string]any{
                "type":        "string",
                "enum":        []string{"GET", "POST", "PUT", "DELETE"},
                "description": "HTTP method",
            },
            "timeout": map[string]any{
                "type":        "string",
                "description": "Request timeout (e.g., '5s', '100ms')",
            },
        },
        "required": []string{"url", "method"},
    }
}
```

## Loading Custom Plugins

Register plugins before starting the server:

```go
package main

import (
    "github.com/your-org/noda/internal/registry"
    myplugin "github.com/example/noda-myplugin"
)

func init() {
    registry.RegisterPlugin(&myplugin.Plugin{})
}
```

## Best Practices

1. **Validate early** — check config in the factory, not the executor
2. **Use expressions** — let users pass expressions for dynamic values; resolve them with `nCtx.Resolve()`
3. **Return structured data** — return `map[string]any` so downstream nodes can access fields
4. **Use appropriate error types** — pick the right `api.*Error` for correct HTTP status mapping
5. **Respect context** — check `ctx.Done()` in long-running operations
6. **Clean up in Shutdown** — close connections, release resources
7. **Keep nodes focused** — one operation per node; compose in workflows
