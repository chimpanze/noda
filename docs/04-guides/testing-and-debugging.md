# Testing & Debugging

This guide covers writing and running workflow tests, debugging with dev mode, and resolving common errors.

## Writing Workflow Tests

Noda includes a built-in test runner that executes workflows in isolation. Core nodes (transform, control, response, util, workflow) run with real executors, while plugin nodes (db, cache, http, etc.) are replaced with mocks you define. This lets you verify workflow logic without running databases, caches, or external services.

### Test File Structure

Test files live in `tests/` at the root of your config directory. Each file is a JSON file that defines a test suite for one workflow. Name files `test-<workflow-id>.json` by convention.

```
my-project/
  workflows/
    create-user.json
    get-user.json
  tests/
    test-create-user.json
    test-get-user.json
```

A test file has this structure:

```json
{
  "id": "test-create-user",
  "workflow": "create-user",
  "tests": [
    {
      "name": "creates user with valid input",
      "input": { ... },
      "auth": { ... },
      "mocks": { ... },
      "expect": { ... }
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique identifier for the test suite |
| `workflow` | string | yes | ID of the workflow under test |
| `tests` | array | yes | Array of test cases |

Each test case has:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Descriptive test name |
| `input` | object | no | Workflow input data (available as `input.*` in expressions) |
| `auth` | object | no | Simulated auth context (available as `auth.*` in expressions) |
| `mocks` | object | no | Mock outputs for plugin nodes, keyed by node ID |
| `expect` | object | yes | Expected results |

### Providing Test Input

The `input` field sets data available through `input.*` expressions in the workflow. Pass the same shape your route or worker would receive:

```json
{
  "name": "creates user with valid input",
  "input": {
    "email": "alice@example.com",
    "name": "Alice",
    "role": "editor"
  }
}
```

Inside the workflow, expressions like `{{ input.email }}` resolve to `"alice@example.com"`.

### Providing Auth Context

The `auth` field simulates an authenticated request. It maps to `auth.*` expressions in the workflow:

```json
{
  "name": "admin can create user",
  "auth": {
    "user_id": "admin-1",
    "roles": ["admin"],
    "claims": {
      "org_id": "org-42"
    }
  },
  "input": { "email": "bob@example.com" }
}
```

| Auth Field | Type | Expression |
|------------|------|------------|
| `user_id` | string | `auth.sub` |
| `roles` | string[] | `auth.roles` |
| `claims` | object | `auth.claims.*` |

### Mocking Service Nodes

Plugin nodes (db, cache, http, email, etc.) need infrastructure that is not available during tests. The test runner replaces them with mocks. Any plugin node without a mock fails with a clear error message.

Core nodes (transform.set, control.if, response.json, util.log, etc.) run with their real executors and do not need mocks.

**Mocking a successful result:**

```json
"mocks": {
  "insert_user": {
    "output": {
      "id": "uuid-123",
      "email": "alice@example.com",
      "created_at": "2025-01-15T10:00:00Z"
    }
  }
}
```

The mock returns the `output` data on the `"success"` output by default. To fire a different named output, use `output_name`:

```json
"mocks": {
  "check_permission": {
    "output": { "allowed": true },
    "output_name": "granted"
  }
}
```

**Mocking an error:**

```json
"mocks": {
  "insert_user": {
    "error": { "message": "duplicate key: email already exists" }
  }
}
```

When `error` is set, the mock node fails. If the workflow has an error edge from that node, execution follows the error path. If there is no error edge, the workflow fails with status `"error"`.

### Asserting Outputs

The `expect` block defines what the test checks after execution.

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `"success"` or `"error"` |
| `error_node` | string | Which node caused the error (when status is `"error"`) |
| `output` | object | Dot-path assertions: `"node_id.field"` mapped to expected value |
| `outputs` | object | Partial deep match against full node output objects |

**Dot-path assertions** check individual fields across any node's output:

```json
"expect": {
  "status": "success",
  "output": {
    "respond.status": 201,
    "respond.body.email": "alice@example.com",
    "format.full_name": "Alice Smith"
  }
}
```

Dot paths navigate into nested output: `"respond.body.email"` resolves to `outputs["respond"]["body"]["email"]`.

**Partial deep matching** checks that an entire node's output contains expected fields (extra fields are ignored):

```json
"expect": {
  "status": "success",
  "outputs": {
    "respond": {
      "status": 201,
      "body": {
        "email": "alice@example.com"
      }
    }
  }
}
```

Both `output` and `outputs` can be used together. Numbers are compared with type coercion (int and float64 are treated as equal).

### Complete Example

Below is a full test file for a `create-user` workflow with three test cases covering the happy path, a validation error, and a duplicate email.

```json
{
  "id": "test-create-user",
  "workflow": "create-user",
  "tests": [
    {
      "name": "creates user successfully",
      "input": {
        "email": "alice@example.com",
        "name": "Alice"
      },
      "auth": {
        "user_id": "admin-1",
        "roles": ["admin"]
      },
      "mocks": {
        "insert": {
          "output": {
            "id": "uuid-123",
            "email": "alice@example.com",
            "name": "Alice"
          }
        },
        "respond": {
          "output": {
            "status": 201,
            "body": {
              "id": "uuid-123",
              "email": "alice@example.com"
            }
          }
        }
      },
      "expect": {
        "status": "success",
        "output": {
          "respond.status": 201,
          "respond.body.id": "uuid-123"
        }
      }
    },
    {
      "name": "validation fails with missing email",
      "input": {
        "name": "Alice"
      },
      "auth": {
        "user_id": "admin-1",
        "roles": ["admin"]
      },
      "mocks": {
        "insert": {
          "output": {}
        },
        "respond": {
          "output": {}
        }
      },
      "expect": {
        "status": "error"
      }
    },
    {
      "name": "duplicate email returns error",
      "input": {
        "email": "alice@example.com",
        "name": "Alice"
      },
      "auth": {
        "user_id": "admin-1",
        "roles": ["admin"]
      },
      "mocks": {
        "insert": {
          "error": { "message": "duplicate key: email already exists" }
        },
        "respond": {
          "output": {}
        }
      },
      "expect": {
        "status": "error",
        "error_node": "insert"
      }
    }
  ]
}
```

## Testing Strategies

### Happy Path First

Start every test suite with a test that exercises the primary success path. Provide valid input, mock all plugin nodes to return expected data, and assert the final output. This confirms the workflow structure and node wiring are correct before testing edge cases.

### Error Paths

After the happy path, add tests for each failure mode:

- **Validation failure:** Provide input that fails a `transform.validate` node. Expect `status: "error"`. No mocks needed for nodes that run after validation since execution stops at the failing node.
- **Not found:** Mock a database query to return `null` or an empty result. Verify the workflow follows the correct branch (e.g., returns a 404 response).
- **Unauthorized:** Omit the `auth` field or provide a user without required roles. Test that authorization checks reject the request.
- **Service failure:** Mock a plugin node with an `error` to simulate database downtime, network failures, or external API errors. Verify error handling paths work.

### Edge Cases

- **Empty input:** Test with `"input": {}` to verify required-field checks.
- **Null values:** Mock a node output with `null` fields to test null handling in downstream expressions.
- **Large payloads:** Test with arrays or deeply nested objects if your workflow processes collections.
- **Branch coverage:** For workflows with `control.if` or `control.switch`, write one test per branch to ensure each path executes correctly.

## Running Tests

### CLI Command

Run all tests:

```bash
noda test --config ./my-project
```

Run tests for a specific workflow:

```bash
noda test --config ./my-project --workflow create-user
```

Run with verbose output to see the execution trace for every test case:

```bash
noda test --config ./my-project --verbose
```

Flags:

| Flag | Description |
|------|-------------|
| `--config` | Path to the config directory (default: current directory) |
| `--env` | Environment to use for config resolution |
| `--workflow` | Run tests only for the specified workflow ID |
| `--verbose` | Show execution traces for all test cases |

### Reading Test Output

Standard output shows pass/fail status for each test:

```
  Workflow: create-user
    ✓ creates user successfully (1.2ms)
    ✓ validation fails with missing email (0.8ms)
    ✗ duplicate email returns error (0.9ms)
      expected error at node "insert", got ""

  2 passed, 1 failed, 3 total
```

With `--verbose`, each test also shows the execution trace listing every node that ran, its type, output, and duration:

```
    ✓ creates user successfully (1.2ms)
      Trace:
        validate (transform.validate) → success [0.1ms]
        insert (db.insert) → success [0.3ms]
        respond (response.json) → success [0.2ms]
```

The trace helps identify which nodes executed and in what order, which is useful for debugging branching logic.

## Debugging with Dev Mode

Dev mode provides live feedback while you build and modify workflows.

### Starting Dev Mode

```bash
noda dev --config ./my-project
```

Dev mode starts all configured runtimes (server, workers, scheduler, wasm) and enables:

- **Hot reload:** Config files are watched for changes. When a JSON file is modified, created, or renamed, Noda re-validates the full config and swaps it in without restarting. If validation fails, the previous config is kept and errors are logged.
- **Trace WebSocket:** A WebSocket endpoint at `/ws/trace` streams execution events in real time.
- **Visual editor:** The built-in editor is served at `/editor/` for visual workflow editing.

### Live Trace Output

Connect to the trace WebSocket at `ws://localhost:3000/ws/trace` to see live execution events. Each event includes the node ID, type, output, and timing. Trace events fire for every workflow execution, including those triggered by HTTP requests, workers, and schedulers.

The trace stream also reports config reload events:

- `config:reloaded` -- config was successfully reloaded with the count of files processed.
- `file:error` -- config reload failed with validation errors. The previous config remains active.

### Visual Editor Debugging

Open `http://localhost:3000/editor/` in your browser. The editor connects to the trace WebSocket automatically. You can:

- See the workflow graph rendered visually with all nodes and edges.
- Watch nodes light up as they execute in real time.
- Inspect node outputs and errors by clicking on nodes after execution.
- Edit workflow config and see changes applied immediately through hot reload.

### Using util.log

Add `util.log` nodes to your workflow to output values during execution. These appear in the server logs and in the trace stream:

```json
{
  "id": "debug_input",
  "type": "util.log",
  "config": {
    "message": "Received input",
    "data": "{{ input }}"
  }
}
```

Place `util.log` nodes between other nodes to inspect intermediate values. They pass through without affecting workflow execution. Remove or disable them before deploying to production.

## Common Errors and Fixes

| Error | Cause | Fix |
|-------|-------|-----|
| `workflow "X" not found` | Test suite references a workflow ID that does not exist in the config | Check that the `workflow` field in the test file matches the `id` in the workflow JSON file |
| `node "X" (type "Y") has no mock` | A plugin node was not mocked in the test case | Add a mock entry for the node ID in the `mocks` object |
| `expected status "success", got "error"` | The workflow failed but the test expected success | Check mock outputs -- a missing or misconfigured mock may cause a node to fail. Run with `--verbose` to see the trace |
| `field "X" not found at "Y"` | A dot-path assertion references a field that does not exist in the node output | Verify the node ID and field name in `expect.output`. Check that the mock returns the expected structure |
| `expected X to be Y, got Z` | A dot-path assertion value does not match | Check the expected value type. JSON numbers are float64, so use `201` not `"201"`. The runner handles int/float coercion but not string/number |
| `compile workflow: ...` | The workflow config has structural errors (missing edges, invalid node references) | Fix the workflow JSON. Run `noda validate` to check config separately |
| `expression error: ...` | An expression like `{{ nodes.X.field }}` failed to evaluate | Check that the referenced node ID exists and that its mock output contains the expected field |
| `config validation failed` | The config directory has validation errors preventing tests from loading | Run `noda validate --config ./my-project` and fix reported errors before running tests |
| `No test files found in tests/` | No JSON files exist in the `tests/` directory | Create test files in `tests/` following the naming convention `test-<workflow-id>.json` |
| `context deadline exceeded` | A workflow execution timed out, possibly due to an infinite loop | Check `control.loop` nodes for missing or incorrect exit conditions |
