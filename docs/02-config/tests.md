# Tests

Files in `tests/*.json`. Each file defines a test suite for one workflow.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Test suite identifier |
| `workflow` | string | yes | Workflow under test |
| `tests` | array | yes | Test cases |

## Test Case

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Test name |
| `input` | object | no | Workflow input data |
| `mocks` | object | no | Node output mocks (node ID → mock definition) |
| `mocks.<node>.output` | any | no | The data the mocked node produces |
| `mocks.<node>.output_name` | string | no | Which output port the mock fires (default: `"success"`) |
| `mocks.<node>.error` | object | no | Make the mocked node fail: `{"message": "..."}` |
| `expect` | object | yes | Expectations |
| `expect.status` | string | no | `"success"` or `"error"` |
| `expect.output` | object | no | Expected output values (dot paths into node outputs) |
| `expect.outputs` | object | no | Partial deep match against node outputs (node ID → expected subtree) |
| `expect.error_node` | string | no | When `status` is `"error"`, assert which node produced the failure |

```json
{
  "id": "test-create-task",
  "workflow": "create-task",
  "tests": [
    {
      "name": "creates task with valid input",
      "input": { "title": "Test task" },
      "mocks": {
        "insert": {
          "output": { "id": 1, "title": "Test task", "completed": false }
        }
      },
      "expect": {
        "status": "success"
      }
    },
    {
      "name": "fails with empty title",
      "input": { "title": "" },
      "expect": {
        "status": "error"
      }
    }
  ]
}
```

## Matching node outputs

`expect.outputs` maps a node ID to a subtree that must appear in that node's
output. The match is partial — keys you omit are ignored.

Node outputs are compared as JSON, so use the **JSON field names**, which are
lowercase. A `response.json` node produces `status`, `headers`, `cookies` and
`body` — not the Go field names `Status` or `Body`:

```json
{
  "expect": {
    "status": "success",
    "outputs": {
      "respond": {
        "status": 200,
        "body": { "greeting": "Hello, World!" }
      }
    }
  }
}
```

## Error edges change `expect.status`

`expect.status` is the status of the *workflow*, not of a node. If the failing
node has an `error` edge, the workflow follows that edge and still finishes
successfully — so assert `"success"` and check the node the error edge leads
to:

```json
{
  "name": "task not found",
  "mocks": { "fetch": { "error": { "message": "record not found" } } },
  "expect": {
    "status": "success",
    "outputs": { "not_found": { "status": 404 } }
  }
}
```

Use `"status": "error"` only when nothing handles the failure. In that case
`expect.error_node` asserts which node failed.
