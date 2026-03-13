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
| `mocks` | object | no | Node output mocks (node ID → output) |
| `expect` | object | yes | Expectations |
| `expect.status` | string | no | `"success"` or `"error"` |
| `expect.output` | object | no | Expected output values |

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
